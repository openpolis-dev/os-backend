package proposal

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/db_agent"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var updateLock sync.RWMutex
var updatingProposals = make(map[uint]bool)

type proposalComponentActions struct {
	ProposalComponentRecordId int    `json:"proposal_component_record_id"`
	ComponentParams           string `json:"component_params"`
	ApproveActionName         string `json:"approve_action_name"`
	RejectActionName          string `json:"reject_action_name"`
}

type budgetComponentDataP1 struct {
	Amount     string `json:"amount"`
	Applicant  string `json:"applicant"`
	ProposalId string `json:"proposal_id"`
	AssetInfo  struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	} `json:"typeTest"`
}

func (r *budgetComponentDataP1) prepareBudgetRecords(proposalId uint) []*model.ProjectBudget {
	totalBudgetAmount := decimal.RequireFromString(r.Amount)
	advancedRatio := decimal.Zero
	totalAdvanceAmount := decimal.Zero

	return []*model.ProjectBudget{
		{
			ProposalID:          proposalId,
			ProjectID:           0,
			AssetName:           r.AssetInfo.Name,
			TotalAmount:         totalBudgetAmount,
			UsedAmount:          decimal.Zero,
			RemainAmount:        totalBudgetAmount,
			AdvanceRatio:        advancedRatio,
			TotalAdvanceAmount:  totalAdvanceAmount,
			UsedAdvanceAmount:   decimal.Zero,
			RemainAdvanceAmount: totalAdvanceAmount,
			CreateTs:            model.GetCurrentUtcEpochSecond(),
			UpdateTs:            model.GetCurrentUtcEpochSecond(),
		},
	}
}

type budgetComponentData struct {
	Applicant  string `json:"applicant"`
	BudgetList []struct {
		Amount      string `json:"amount"`
		Description string `json:"description"`
		Proportion  string `json:"proportion"`
		AssetInfo   struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		} `json:"typeTest"`
	} `json:"budgetList"`
	ProposalId string `json:"proposal_id"`
}

func (r *budgetComponentData) prepareBudgetRecords(proposalId uint) []*model.ProjectBudget {
	projectBudgetRcds := make([]*model.ProjectBudget, 0)
	for _, r := range r.BudgetList {
		totalBudgetAmount := decimal.RequireFromString(r.Amount)
		advancedRatio := decimal.RequireFromString(r.Proportion).Div(decimal.NewFromInt(100))
		totalAdvanceAmount := totalBudgetAmount.Mul(advancedRatio).Round(0)

		projectBudgetRcds = append(projectBudgetRcds, &model.ProjectBudget{
			ProposalID:          proposalId,
			ProjectID:           0,
			AssetName:           r.AssetInfo.Name,
			TotalAmount:         totalBudgetAmount,
			UsedAmount:          decimal.Zero,
			RemainAmount:        totalBudgetAmount,
			AdvanceRatio:        advancedRatio,
			TotalAdvanceAmount:  totalAdvanceAmount,
			UsedAdvanceAmount:   decimal.Zero,
			RemainAdvanceAmount: totalAdvanceAmount,
			CreateTs:            model.GetCurrentUtcEpochSecond(),
			UpdateTs:            model.GetCurrentUtcEpochSecond(),
		})
	}

	return projectBudgetRcds
}

type commonCreateProjectRelatedData struct {
	Desc string `json:"description"`
}

func TryAcquireUpdateProposalDbLockOrReturn(proposalId uint) bool {
	updateLock.Lock()
	defer updateLock.Unlock()
	if _, ok := updatingProposals[proposalId]; ok {
		return false
	} else {
		updatingProposals[proposalId] = true
		return true
	}
}

func ReleaseUpdateProposalDbLock(proposalId uint) {
	updateLock.Lock()
	defer updateLock.Unlock()
	delete(updatingProposals, proposalId)
}

func GetProposalFromStringId(db *gorm.DB, idStr string) (*model.Proposal, error) {
	proposalId, err := strconv.Atoi(idStr)
	if err != nil {
		log.Error().Msgf("parse proposal id %s error: %+v", idStr, err)
		return nil, err
	}
	var proposalRecord model.Proposal
	if err := db.Joins("ProposalCategory").First(&proposalRecord, proposalId).Error; err != nil {
		return nil, err
	}

	return &proposalRecord, nil
}

// ValidateProposalComponentParams validate proposal component params, currently it contains
// * For motivation components, if the proposal has associated project, verify the total amount is not greater than the (project budget - advance amount)
func ValidateProposalComponentParams(db *gorm.DB, reqData *CreateOrUpdateProposalData, userWallet string, proposalId uint, cfg *config.Config) error {
	log.Error().Msgf("TTT: validate proposal component params: %+v", reqData)
	for _, componentData := range reqData.Components {
		log.Error().Msgf("TTT: validate proposal component params: %+v", componentData)
		switch componentData.Name {
		case internal.ComponentNameMotivation:
			if reqData.CreateProjectProposalId == 0 {
				log.Debug().Msgf("proposal %d is not associated with any project", proposalId)
				continue
			}

			// Get project from create_project_proposal_id
			associatedProject, err := service.ProposalService.GetAssociatedProjectByProposalId(db, reqData.CreateProjectProposalId)
			if err != nil {
				log.Error().Msgf("get create project proposal %d error: %+v", reqData.CreateProjectProposalId, err)
				return err
			}

			if associatedProject == nil {
				err = fmt.Errorf("project %d is not associated with proposal %d", reqData.CreateProjectProposalId, proposalId)
				log.Error().Msg(err.Error())
				return err
			}

			// Get project_budget records to get total amount of the budgets
			projectBudgets, err := model.ProjectBudgetModel.ListByProjectId(db, associatedProject.ID)
			log.Error().Msgf("TTT: project budgts: %+v", projectBudgets)
			if err != nil {
				log.Error().Msgf("get create project proposal %d error: %+v", reqData.CreateProjectProposalId, err)
				return err
			}

			if len(projectBudgets) == 0 {
				err = fmt.Errorf("project %d has no budget", reqData.CreateProjectProposalId)
				log.Error().Msg(err.Error())
				return err
			}

			remainBudgetAmount := lo.SliceToMap(projectBudgets, func(budget *model.ProjectBudget) (string, decimal.Decimal) {
				return budget.AssetName, budget.RemainAmount
			})

			log.Error().Msgf("TTT: remain budget amount: %+v", remainBudgetAmount)

			log.Error().Msgf("TTT: Component data: %+v", componentData)

			var motivationComponentData *model.ComponentMotivationData
			componentDataBytes, err := json.Marshal(componentData.Data)
			if err != nil {
				log.Error().Msgf("marshal motivation component budget list error: %+v", err)
				return err
			}
			err = json.Unmarshal(componentDataBytes, &motivationComponentData)
			if err != nil {
				log.Error().Msgf("unmarshal motivation component budget list error: %+v", err)
				return err
			}

			componentRewardAmountData := lo.SliceToMap(motivationComponentData.RewardList, func(reward *model.ComponentMotivationRewardRecord) (string, decimal.Decimal) {
				return reward.AssetInfo.Name, decimal.RequireFromString(reward.Amount)
			})
			log.Error().Msgf("TTT: component reward amount: %+v", componentRewardAmountData)

			for assetName, remainBudgetAmount := range remainBudgetAmount {
				if componentBudgetAmount, found := componentRewardAmountData[assetName]; found {
					if componentBudgetAmount.GreaterThan(remainBudgetAmount) {
						return errors.New(fmt.Sprintf("motivation component %s budget amount %s is greater than project budget %s", assetName, componentBudgetAmount.String(), remainBudgetAmount.String()))
					}
				}
			}

			continue
		default:
			// Other components are not checking now
			continue
		}
	}
	return nil
}

func SaveProposalRecordToDB(db *gorm.DB, reqData *CreateOrUpdateProposalData, userWallet string, proposalId uint, cfg *config.Config) (*model.Proposal, error) {
	// If proposalIdStr is not 0, this request should be an update action, otherwise it is a creation action.
	// Create:
	//   1. Create proposal record
	//   2. Save associated content and component blocks
	// Update:
	//   1. Validate whether the state is in updatable list
	//   2. If proposal is in PendingSubmit state, update the proposal record in place, and upsert content/component blocks
	//   3. If proposal is in Withdrawn / Rejected state,
	//        * copy existing proposal record to new record,
	//        * inc version,
	//        * create blocks with new proposal.
	//   4. In this case, the proposal must be updated to metaforo without checking the Submit flag

	// Lock the proposal id for write
	if proposalId != 0 && TryAcquireUpdateProposalDbLockOrReturn(proposalId) == false {
		err := fmt.Errorf("proposal %d is updating", proposalId)
		log.Error().Msg(err.Error())
		return nil, err
	}
	defer ReleaseUpdateProposalDbLock(proposalId)

	// Validate component data before processing the proposal
	componentValidationError := ValidateProposalComponentParams(db, reqData, userWallet, proposalId, cfg)
	if componentValidationError != nil {
		err = fmt.Errorf("component data validation error: %+v", componentValidationError)
		log.Error().Msg(err.Error())
		return nil, err
	}

	log.Debug().Msgf("save proposal record to DB: %+v, proposalIdStr: %d, user wallet: %s", reqData, proposalId, userWallet)

	// Update title for testing
	if cfg.MetaforoData.ProposalPrefix != "" && !strings.HasPrefix(reqData.Title, cfg.MetaforoData.ProposalPrefix) {
		reqData.Title = cfg.MetaforoData.ProposalPrefix + reqData.Title
	}

	var voteTimeProps model.VoteTimeProperties

	var pTemplate model.ProposalTemplate
	err = db.Find(&pTemplate, reqData.TemplateId).Error
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return nil, err
	}

	voteTimeProps.PublicitySecond = pTemplate.PublicitySecond
	voteTimeProps.VoteDurationSecond = pTemplate.VoteDurationSecond
	voteTimeProps.PendingExecutionSecond = pTemplate.PendingExecutionSecond

	var pCategory model.ProposalCategory
	err := db.Find(&pCategory, pTemplate.ProposalCategoryID).Error
	if err != nil {
		log.Error().Msgf("get proposal category error: %+v", err)
		return nil, err
	}

	if proposalId != 0 {
		// Updating existing proposals
		var dbProposalRcd model.Proposal
		if err = db.Find(&dbProposalRcd, proposalId).Error; err != nil {
			log.Error().Msgf("get proposal error: %+v", err)
			return nil, err
		}

		// update logic:
		// * If original proposal not in ProposalStatePendingSubmit, then copy to new record and
		proposalRcd := dbProposalRcd
		// Bump proposal version if it is not in PendingSubmit state
		if dbProposalRcd.State != int(model.ProposalStatePendingSubmit) {
			log.Debug().Msgf("proposal %d has submitted to metaforo, create new record with bumped up version", dbProposalRcd.ID)
			// Proposal has already published on chain, create new record and bump up version
			newProposalRecord, err := dbProposalRcd.BumpUpVersion(db)
			if err != nil {
				log.Error().Msgf("create proposal with bumped version error: %+v, proposal data: %+v", err, dbProposalRcd)
				return nil, err
			}

			// Assign the bumped up version proposal back to proposalRcd
			proposalRcd = *newProposalRecord
		} else {
			log.Debug().Msgf("proposal %d is in PendingSubmit state, update it directly", dbProposalRcd.ID)
		}

		// Update fields from request data.
		// The update logic can be applied to both PendingSubmit and Draft state
		proposalRcd.Title = reqData.Title
		proposalRcd.ProposalCategoryID = pTemplate.ProposalCategoryID
		proposalRcd.VoteType = dbProposalRcd.VoteType
		proposalRcd.CanBeVetoed = dbProposalRcd.CanBeVetoed
		proposalRcd.IsBasedOnCustomTemplate = dbProposalRcd.IsBasedOnCustomTemplate
		proposalRcd.PublicitySecond = dbProposalRcd.PublicitySecond
		proposalRcd.PendingExecutionSecond = dbProposalRcd.PendingExecutionSecond
		proposalRcd.VoteDurationSecond = dbProposalRcd.VoteDurationSecond
		proposalRcd.VoteDurationSecond = dbProposalRcd.VoteDurationSecond
		proposalRcd.AssociateProposalId = reqData.CreateProjectProposalId

		// proposalRcd is a new record, use create function here. Also update other data related to proposal
		if err = db.Transaction(func(tx *gorm.DB) error {
			err = tx.Save(&proposalRcd).Error
			if err != nil {
				log.Error().Msgf("duplicate proposal error: %+v", err)
				return err
			}

			// Move vote record and vote option record from original proposal to new one
			if err = tx.Model(&model.ProposalVoteRecord{}).Where("proposal_id = ?", proposalId).Update("proposal_id", proposalRcd.ID).Error; err != nil {
				log.Error().Msgf("move proposal vote record error: %+v", err)
				return err
			}

			if err = tx.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_id = ?", proposalId).Update("proposal_id", proposalRcd.ID).Error; err != nil {
				log.Error().Msgf("move proposal vote record error: %+v", err)
				return err
			}

			// Create new content block and component records for new proposal
			if err := SaveProposalContentRecords(tx, proposalRcd.ID, reqData.ContentBlocks); err != nil {
				log.Error().Msgf("create proposal block error: %+v", err)
				return err
			}

			// Create proposal components
			if err := SaveProposalComponentRecords(tx, proposalRcd.ID, userWallet, reqData.Components); err != nil {
				log.Error().Msgf("create proposal component blocks error: %+v", err)
				return err
			}

			if err := SaveProposalVoteOptionRecords(tx, proposalRcd.ID, pTemplate.VoteType, reqData.VoteOptions); err != nil {
				log.Error().Msgf("create proposal vote option records error: %+v", err)
				return err
			}

			return nil
		}); err != nil {
			log.Error().Msgf("update proposal error: %+v", err)
			return nil, err
		}

		return &proposalRcd, nil
	} else {
		// Init proposal record to get ID
		proposalRecord := model.Proposal{
			CreateTs:                time.Now().UTC().Unix(),
			Title:                   reqData.Title,
			Applicant:               common.FormatUserWallet(userWallet),
			ProposalCategoryID:      pTemplate.ProposalCategoryID,
			Version:                 1,
			VoteType:                pTemplate.VoteType,
			CanBeVetoed:             pCategory.CanBeVetoed,
			IsBasedOnCustomTemplate: pTemplate.IsCustomTemplate,
			AssociateProposalId:     reqData.CreateProjectProposalId,
			ProposalTemplateID:      &reqData.TemplateId,
			ExtraResultCheckRule:    pTemplate.ExtraResultCheckRule,
		}
		proposalRecord.PublicitySecond = voteTimeProps.PublicitySecond
		proposalRecord.PendingExecutionSecond = voteTimeProps.PendingExecutionSecond
		proposalRecord.VoteDurationSecond = voteTimeProps.VoteDurationSecond

		if err = db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&proposalRecord).Error; err != nil {
				log.Error().Msgf("create proposal error: %+v", err)
				return err
			}

			// Create proposal content blocks
			if err := SaveProposalContentRecords(tx, proposalRecord.ID, reqData.ContentBlocks); err != nil {
				log.Error().Msgf("create proposal block error: %+v", err)
				return err
			}

			// Create proposal components
			if err := SaveProposalComponentRecords(tx, proposalRecord.ID, userWallet, reqData.Components); err != nil {
				log.Error().Msgf("create proposal component blocks error: %+v", err)
				return err
			}

			if err := SaveProposalVoteOptionRecords(tx, proposalRecord.ID, pTemplate.VoteType, reqData.VoteOptions); err != nil {
				log.Error().Msgf("create proposal vote option records error: %+v", err)
				return err
			}

			return nil
		}); err != nil {
			log.Error().Msgf("create proposal error: %+v", err)
			return nil, err
		}

		return &proposalRecord, nil
	}
}

func SaveProposalContentRecords(db *gorm.DB, proposalRecordId uint, reqContentBlockData []*FrontendContentBlockRecord) error {
	log.Debug().Msgf("Update proposal %d content block records, block data: %+v", proposalRecordId, reqContentBlockData)
	// Fetch currently content block record IDs associated to the proposal
	var existingContentBlockIds []uint
	err := db.Model(&model.ProposalContentBlock{}).Where(model.ProposalContentBlock{ProposalID: proposalRecordId}).Pluck("id", &existingContentBlockIds).Error
	if err != nil {
		log.Error().Msgf("get proposal content block ids error: %+v", err)
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// Update or create blocks in request data
		var updatedIds []uint
		for _, block := range reqContentBlockData {
			if err := tx.Save(&model.ProposalContentBlock{
				ID:            block.ID,
				ProposalID:    proposalRecordId,
				Title:         block.Title,
				Content:       block.Content,
				Type:          block.Type,
				ComponentList: block.ComponentList,
				CreateTs:      time.Now().UTC().Unix(),
			}).Error; err != nil {
				log.Error().Msgf("create proposal block error: %+v, block data: %+v", err, block)
				return err
			}
			if block.ID != 0 {
				updatedIds = append(updatedIds, block.ID)
			}
		}
		// Remove deleted blocks
		for _, blockId := range existingContentBlockIds {
			if !lo.Contains(updatedIds, blockId) {
				if err := tx.Delete(&model.ProposalContentBlock{ID: blockId}).Error; err != nil {
					log.Error().Msgf("delete proposal block error: %+v", err)
					return err
				}
			}
		}
		return nil
	})
}

func SaveProposalComponentRecords(db *gorm.DB, proposalId uint, applicantWallet string, reqComponentData []*ComponentRequestData) error {
	var existingComponentIds []uint
	err := db.Model(&model.ProposalComponentRecord{}).Where(model.ProposalComponentRecord{ProposalID: proposalId}).Pluck("id", &existingComponentIds).Error
	if err != nil {
		log.Error().Msgf("get proposal content block ids error: %+v", err)
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var updatedIds []uint
		for _, componentData := range reqComponentData {
			// Try to get component record from DB
			componentRecord := model.ProposalComponent{
				Name: componentData.Name,
			}
			if err := tx.Where(componentRecord).First(&componentRecord).Error; err != nil {
				tx.Rollback()
				log.Error().Msgf("find proposal component %s error: %+v", componentData.Name, err)
				return err
			}

			// Append applicant information
			componentData.Data["applicant"] = common.FormatUserWallet(applicantWallet)
			componentData.Data["proposal_id"] = fmt.Sprintf("os-%d", proposalId)

			proposalDataStr, err := json.Marshal(componentData.Data)
			if err != nil {
				tx.Rollback()
				log.Error().Msgf("create proposal component error: %+v", err)
				return err
			}

			// Create proposal component record and save to DB
			if err := tx.Save(&model.ProposalComponentRecord{
				CreateTs:    time.Now().UTC().Unix(),
				ComponentID: componentRecord.ID,
				ProposalID:  proposalId,
				Data:        string(proposalDataStr),
			}).Error; err != nil {
				log.Error().Msgf("create proposal component error: %+v", err)
				return err
			}

			if componentData.ID != 0 {
				updatedIds = append(updatedIds, componentData.ID)
			}
		}

		// Remove deleted blocks
		for _, componentId := range existingComponentIds {
			if !lo.Contains(updatedIds, componentId) {
				if err := tx.Delete(&model.ProposalComponentRecord{ID: componentId}).Error; err != nil {
					log.Error().Msgf("delete proposal component error: %+v", err)
					return err
				}
			}
		}
		return nil
	})
}

// SaveProposalVoteOptionRecords saves proposal vote option records into DB
// Those records will be converted to metaforo format data while submitting to metaforo
// To implement this feature, one proposal can ONLY HAVE AT MOST ONE vote record
func SaveProposalVoteOptionRecords(tx *gorm.DB, proposalId uint, voteType int, customVoteOptions []string) error {
	log.Debug().Msgf("save proposal vote option records: %d, %d, %v", proposalId, voteType, customVoteOptions)
	switch voteType {
	case model.ProposalVoteTypeNone:
		log.Debug().Msgf("skip saving proposal vote option records for none vote type")
		return nil
	case model.ProposalVoteTypeDecision, model.ProposalVoteTypeNumericSingle, model.ProposalVoteTypeNumericAvg:
		var voteOptions []*model.ProposalVoteOptionRecord
		if voteType == model.ProposalVoteTypeDecision {
			voteOptions = lo.Map(internal.ProposalDecisionVoteOptions, func(s []string, _ int) *model.ProposalVoteOptionRecord {
				return &model.ProposalVoteOptionRecord{
					ProposalId: proposalId,
					Text:       s[0],
					Value:      s[1],
				}
			})
		} else {
			voteOptions = lo.Map(internal.ProposalNumericVoteOptions, func(s []string, _ int) *model.ProposalVoteOptionRecord {
				return &model.ProposalVoteOptionRecord{
					ProposalId: proposalId,
					Text:       s[0],
					Value:      s[1],
				}
			})
		}

		proposalVoteRecord, err := db_agent.UpsertProposalVoteRecord(tx, proposalId, voteType, voteOptions)
		if err != nil {
			log.Error().Msgf("upsert proposal vote record error: %+v", err)
			return err
		}

		log.Debug().Msgf("proposal vote record: %+v", proposalVoteRecord)
		return nil
	case model.ProposalVoteTypeCustomerDefinedEqualFailed, model.ProposalVoteTypeCustomerDefinedAlwaysPassed:
		log.Debug().Msgf("custom vote options: %+v", customVoteOptions)
		voteOptions := lo.Map(customVoteOptions, func(s string, _ int) *model.ProposalVoteOptionRecord {
			return &model.ProposalVoteOptionRecord{
				ProposalId: proposalId,
				Text:       s,
			}
		})
		proposalVoteRecord, err := db_agent.UpsertProposalVoteRecord(tx, proposalId, voteType, voteOptions)
		if err != nil {
			log.Error().Msgf("upsert proposal vote record error: %+v", err)
			return err
		}
		log.Debug().Msgf("proposal vote record: %+v", proposalVoteRecord)
		return nil
	default:
		log.Error().Msgf("unsupported vote type: %d, no vote will be created", voteType)
		return nil
	}
}

// SaveProposalToMetaforo updates proposal record to Metaforo
// If proposal is in pending submit state, update existing proposal record (state, ProposalRecordId, ArweaveHash) and save back,
// the version is keep the same. The metaforo API invoked here is CreateProposal.
//
// Otherwise, copy the proposal to new record with ver+1, update the metaforo data, and save back as a new record,
// and the metaforo API invoked here is updateProposal.
// TODO: Refactor this function
func SaveProposalToMetaforo(db *gorm.DB, origProposalRecordId uint, voteType int, metaforoAccessToken string, EditorType int, metaforoGroupName string) error {
	if TryAcquireUpdateProposalDbLockOrReturn(origProposalRecordId) == false {
		err := fmt.Errorf("proposal %d is updating", origProposalRecordId)
		log.Error().Msg(err.Error())
		return err
	}
	defer ReleaseUpdateProposalDbLock(origProposalRecordId)

	log.Debug().Msgf("enter save proposal to metaforo: %d", origProposalRecordId)
	var err error

	var origProposalRecord model.Proposal
	if err = db.Find(&origProposalRecord, origProposalRecordId).Error; err != nil {
		log.Error().Msgf("find proposal error: %+v", err)
		return err
	}

	// Load current proposal data
	var proposalCategory *model.ProposalCategory
	err = db.Where(&model.ProposalCategory{ID: origProposalRecord.ProposalCategoryID}).First(&proposalCategory).Error
	if err != nil {
		log.Error().Msgf("get proposal category error: %+v", err)
		return err
	}

	var contentBlocks []*model.ProposalContentBlock
	err = db.Where(&model.ProposalContentBlock{ProposalID: origProposalRecordId}).Find(&contentBlocks).Error
	if err != nil {
		log.Error().Msgf("get proposal content block error: %+v", err)
		return err
	}

	metaforoContent := ""
	for _, block := range contentBlocks {
		metaforoContent += "# " + block.Title + "\n\n" + block.Content + "\n\n"
	}

	var metaforoProposalResponse *metaforo.ProposalResponse
	updatedProposalRecord := origProposalRecord

	// Vote start and end time, used for create and update proposal
	voteStartTime := time.Now().UTC().Add(origProposalRecord.PublicityDuration())
	voteEndTime := time.Now().UTC().Add(origProposalRecord.PublicityDuration() + origProposalRecord.VoteDuration())
	log.Debug().Msgf("vote start time: %s, vote end time: %s", voteStartTime.Format(time.RFC3339), voteEndTime.Format(time.RFC3339))

	var metaforoThreadId int

	if origProposalRecord.ProposalRecordId != "" {
		// DB Record has ProposalRecordId, this is updating metaforo proposal action, which contains
		// * Invoke metaforo.UpdateProposal to update metaforo proposal data
		// * Save the new metaforo proposal data back to new created DB record
		// This logic is invoked while changing proposal state form Withdrawn, Rejected to Draft

		metaforoThreadId = updatedProposalRecord.GetMetaforoThreadId()
		// Update metaforo proposal
		// VoteFormData is empty since no update of vote is allowed in this API
		_, err = metaforo.UpdateProposal(
			metaforoAccessToken,
			metaforoGroupName,
			fmt.Sprintf("%d", proposalCategory.MetaforoId),
			origProposalRecord.Title,
			metaforoContent, nil, "", metaforoThreadId,
		)
		if err != nil {
			log.Error().Msgf("update metaforoProposal error: %+v", err)
			return err
		}

		var voteRecords []*model.ProposalVoteRecord
		err = db.Model(updatedProposalRecord).Association("VoteRecords").Find(&voteRecords)
		if err != nil {
			log.Error().Msgf("get vote records error: %+v", err)
			return err
		}

		// Get vote record from original record and update the timestamp
		// The updated vote record will be saved by response in GetProposal function
		RefreshMetaforoAdminToken()
		for _, record := range voteRecords {
			err = metaforo.UpdateVoteTime(
				metaforoAccessToken,
				metaforoGroupName,
				record.MetaforoID,
				voteStartTime.Unix(),
				voteEndTime.Unix())
			if err != nil {
				log.Error().Msgf("update metaforoProposal vote error: %+v", err)
				return err
			}
		}
	} else {
		// DB Record has no ProposalRecordId, this is creating metaforo proposal action, which contains:
		// * Invoke metaforo.CreateProposal to create metaforo proposal record
		// * Generate vote record form bytes, and send to metaforo via API
		// * Save the metaforo proposal data back to DB record
		// This logic is invoked while changing proposal state form PendingSubmit to Draft

		// Regards vote record, the data is generated here, and uploaded to metaforo in CreateProposal API.
		// And the db records will be updated by data returned from Metaforo
		var pTmpl model.ProposalTemplate
		err = db.Find(&pTmpl, origProposalRecord.ProposalTemplateID).Error
		if err != nil {
			log.Error().Msgf("get proposal template error: %+v", err)
			return err
		}

		var voteGates []*model.ProposalVoteGate
		err = db.Model(&pTmpl).Association("VoteGates").Find(&voteGates)
		if err != nil {
			log.Error().Msgf("get vote gates error: %+v", err)
			return err
		}

		voteFormBytes, err := BuildMetaforoVoteFormDataBytes(db, origProposalRecordId, voteGates, voteStartTime, voteEndTime)
		if err != nil {
			log.Error().Msgf("build metaforoProposal vote data error: %+v", err)
			return err
		}

		metaforoCreateProposalResponse, err := metaforo.CreateProposal(
			metaforoAccessToken,
			metaforoGroupName,
			fmt.Sprintf("%d", proposalCategory.MetaforoId),
			updatedProposalRecord.Title,
			metaforoContent, nil, string(voteFormBytes))
		if err != nil {
			log.Error().Msgf("update metaforoProposal error: %+v", err)
			return err
		} else {
			log.Debug().Msgf("create metaforoProposal success, response: %+v", metaforoCreateProposalResponse)
			metaforoThreadId = metaforoCreateProposalResponse.Thread.Id
		}
	}

	// Get metaforo proposal detail
	metaforoProposalResponse, err = metaforo.GetProposal(metaforoThreadId, metaforoGroupName, metaforoAccessToken, 0)
	if err != nil {
		log.Error().Msgf("get metaforoProposal %d error: %+v", metaforoThreadId, err)
		return err
	}

	err = UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update db records from metaforoProposalResponse error: %+v", err)
	}

	// Save data backed from metaforo API response to DB
	// Setting proposal to draft state here, and after this function return, check for publicity and vote type will be applied to this record
	if err := db.Model(&updatedProposalRecord).
		Where("id = ?", updatedProposalRecord.ID).
		Updates(&model.Proposal{
			ProposalRecordId: model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposalResponse.Thread.Id),
			State:            int(model.ProposalStateDraft),
		}).Error; err != nil {
		log.Error().Msgf("update proposal error: %+v", err)
		return err
	}

	pollStatusChanged, err := UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(db, updatedProposalRecord.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update propsal vote option records with metaforo response error: %+v", err)
		return err
	}

	if pollStatusChanged {
		if err = HandleProposalPollStatusChange(db, updatedProposalRecord.ID); err != nil {
			log.Error().Msgf("handle proposal poll status change error: %+v", err)
			return err
		}
	}

	db.Find(&updatedProposalRecord, updatedProposalRecord.ID)
	log.Debug().Msgf("proposal saved to metafor successfully, now the object is: %+v", updatedProposalRecord)

	return nil
}

func prepareOsVoteOptions(voteType int, customVoteOptions []string) []string {
	if customVoteOptions != nil {
		return customVoteOptions
	} else if (voteType == model.ProposalVoteTypeNumericAvg) || (voteType == model.ProposalVoteTypeNumericSingle) {
		return lo.Map(internal.ProposalNumericVoteOptions, func(r []string, _ int) string { return r[0] })
	} else if voteType == model.ProposalVoteTypeDecision {
		// The default option is decision vote
		return lo.Map(internal.ProposalDecisionVoteOptions, func(r []string, _ int) string { return r[0] })
	} else {
		log.Warn().Msgf("unknown vote type, no vote options will be generated")
		return []string{}
	}
}

// BuildMetaforoVoteFormDataBytes generates the byte representation of the Metaforo vote form data.
//
// Returns:
// - []byte: The byte representation of the vote form data.
// - error: An error if there was a problem generating the byte representation.
func BuildMetaforoVoteFormDataBytes(db *gorm.DB, proposalId uint, voteGates []*model.ProposalVoteGate, startTime time.Time, endTime time.Time) ([]byte, error) {
	voteRecords, err := db_agent.GetProposalVoteRecord(db, proposalId)
	if err != nil {
		log.Error().Msgf("fetch proposal vote record error: %+v", err)
		return nil, err
	}

	// Vote params explanation
	// voteType: 1 = no vote gate, 2 = ERC20, 3 = ERC721/ERC1155
	// chain_type: 1 = eth, 8 = polygon, 7 = bsc, 9 = arbitrum
	// setting_id: vote gate ID saved in metaforo
	// min_tokens: ERC20 token amount
	// token_address: address for token
	// token_id: token ID for ERC1155
	// contract_type: 1 = ERC721, 2 = ERC1155
	voteGateId := 0
	voteType := 1
	tokenAddress := ""
	contractType := 0
	chainType := 0
	tokenId := 0
	minToken := "0"
	if len(voteGates) > 0 {
		if len(voteGates) > 1 {
			log.Warn().Msgf("found %d vote gates for proposal %d, only the first one will be used", len(voteGates), proposalId)
		}
		api.PrintStructAsJson(voteGates[0], "TTT: vote gate")
		voteGateId = voteGates[0].MetaforoId
		switch voteGates[0].TokenType {
		case 0: // ERC20
			voteType = 2
			minToken = voteGates[0].Amount
		case 1: // ERC721
			voteType = 3
			contractType = 1
		case 2: // ERC1155
			voteType = 3
			contractType = 2
			tokenId, _ = strconv.Atoi(voteGates[0].TokenId)
		}
		tokenAddress = voteGates[0].TokenAddress
		chainType = voteGates[0].ChainType

		log.Info().Msgf("create proposal %d with vote gate: %+v", proposalId, voteGates[0])
	}

	voteData := make([]*metaforo.NewVoteFormRequest, 0)
	for idx := range voteRecords {
		mfVoteOpts, err := db_agent.GenerateMetaforoVoteOptions(voteRecords[idx].ProposalID)
		if err != nil {
			log.Error().Msgf("generate metaforo vote options error: %+v", err)
			continue
		}

		voteData = append(voteData, &metaforo.NewVoteFormRequest{
			Options:            mfVoteOpts,
			Type:               "1",
			Title:              voteRecords[idx].Title,
			ShowType:           "1",
			ShowResult:         true,
			Period:             "1",
			CloseAt:            endTime.Format(time.RFC3339),
			VoteStartAt:        startTime.Format(time.RFC3339),
			Max:                1,
			PollCategory:       "0",
			LastCategroyChange: "0",
			Quorum:             false,
			Weight:             true,
			Step:               2,
			ChartType:          "1",

			// Params for vote gate
			SettingId:    voteGateId,
			VoteType:     fmt.Sprintf("%d", voteType),
			ContractType: contractType,
			ChainType:    metaforo.ChainType(chainType),
			TokenId:      tokenId,
			TokenAddress: tokenAddress,
			MinTokens:    minToken,
		})
	}

	api.PrintStructAsJson(voteData, "TTT: mf vote data")

	log.Debug().Msgf("metaforo vote data: %+v", voteData)

	voteDataBytes, err := json.Marshal(voteData)
	if err != nil {
		log.Error().Msgf("marshal proposal vote data error: %+v", err)
		return nil, err
	}
	return voteDataBytes, nil
}

// TODO: Change to use indexer data

func IsUserMetVoteGate(userSeepassData *sdk.SeepassResponse, proposalVoteGate *model.ProposalVoteGate) bool {
	log.Debug().Msgf("check user met vote gate: %+v, seepass data: %+v", proposalVoteGate, userSeepassData)
	if userSeepassData == nil {
		return false
	}

	// No nft gate set, this vote should be opened to all users
	if proposalVoteGate == nil {
		return true
	}

	// Check ERC20 amount
	switch proposalVoteGate.TokenType {
	case 0:
		//ERC20
		if strings.EqualFold(proposalVoteGate.TokenAddress, internal.ScrContractAddr) {
			userScrAmount, err := decimal.NewFromString(userSeepassData.Scr.Amount)
			if err != nil {
				log.Error().Msgf("parse user scr amount error: %+v", err)
				return true
			}

			gateAmount, _ := decimal.NewFromString(proposalVoteGate.Amount)
			return userScrAmount.GreaterThanOrEqual(gateAmount)
		} else {
			log.Warn().Msgf("unknown erc20 token, mark as true")
			return true
		}
	case 1:
		// ERC721
		if strings.EqualFold(proposalVoteGate.TokenAddress, internal.SeedContractAddr) {
			return len(userSeepassData.Seed) > 0
		} else {
			log.Warn().Msgf("unknown erc721 token, mark ask true")
			return true
		}
	case 2:
		// Fake account for testing
		if strings.EqualFold(userSeepassData.Wallet, "0x183F09C3cE99C02118c570e03808476b22d63191") {
			return true
		}
		// ERC1155
		for _, sbtInfo := range userSeepassData.Sbt {
			if strings.EqualFold(sbtInfo.ContractAddr, proposalVoteGate.TokenAddress) {
				if strings.EqualFold(proposalVoteGate.TokenId, sbtInfo.TokenId) {
					return true
				}
			}
		}
		return false
	default:
		log.Error().Msgf("unknown token type, mark as has perm")
		return true
	}
}

func UpdateDbRecordsFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcdId uint, metaforoProposal *metaforo.ProposalResponse) error {
	// Save all version proposals' arweave hash
	if err = UpdateArweaveHashFromMetaforoProposalResponse(db, dbProposalRcdId, metaforoProposal); err != nil {
		log.Error().Msgf("update arweave hash error: %+v", err)
		return err
	}

	if err = UpdateUserRecordsFromMetaforoProposalResponse(db, metaforoProposal); err != nil {
		log.Error().Msgf("update user records error: %+v", err)
		return err
	}

	return nil
}

// UpdateArweaveHashFromMetaforoProposalResponse updates proposal arweave hash from metaforo response
func UpdateArweaveHashFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcdId uint, metaforoProposal *metaforo.ProposalResponse) error {
	var err error

	var dbProposalRcd *model.Proposal
	db.Find(&dbProposalRcd, dbProposalRcdId)

	// Save all version proposals' arweave hash
	proposalRecordId := model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposal.Thread.Id)
	if dbProposalRcd.ProposalRecordId != proposalRecordId {
		err = fmt.Errorf("db proposal record id %s does not match metaforo id %s, skip this update", dbProposalRcd.ProposalRecordId, proposalRecordId)
		log.Error().Msg(err.Error())
		return err
	}

	var dbProposals []*model.Proposal
	err = db.Where(&model.Proposal{ProposalRecordId: dbProposalRcd.ProposalRecordId}).Order("version DESC").Find(&dbProposals).Error
	if err != nil {
		log.Error().Msgf("fetch proposal data with recordId %s error: %+v", dbProposalRcd.ProposalRecordId, err)
		return err
	}

	if len(dbProposals) == 0 {
		log.Debug().Msgf("no db proposal record found with recordId %s", dbProposalRcd.ProposalRecordId)
		return nil
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for idx := 0; idx < min(metaforoProposal.Thread.EditHistory.Count, len(dbProposals)); idx++ {
			tx.Model(&model.Proposal{}).Where(
				"id = ? AND arweave_hash != ?", dbProposals[idx].ID, metaforoProposal.Thread.EditHistory.Lists[idx].Arweave,
			).Update("arweave_hash", metaforoProposal.Thread.EditHistory.Lists[idx].Arweave)
			if idx == 0 {
				// Save arwave hash data to record for setting it correctly in response
				tx.Model(&model.Proposal{}).Where(
					"id = ? AND arweave_hash != ?", dbProposalRcdId, metaforoProposal.Thread.EditHistory.Lists[idx].Arweave,
				).Updates(model.Proposal{ArweaveHash: metaforoProposal.Thread.EditHistory.Lists[idx].Arweave})
			}
		}
		return nil
	})

	if err != nil {
		log.Error().Msgf("update proposal arweave hash error: %+v", err)
		return err
	} else {
		log.Debug().Msgf("update proposal arweave hash success")
		return nil
	}
}

func UpdateUserRecordsFromMetaforoProposalResponse(db *gorm.DB, metaforoProposal *metaforo.ProposalResponse) error {
	// Save user id and wallet from comments data
	if err = db.Transaction(func(tx *gorm.DB) error {
		for _, metaforoComment := range metaforoProposal.Thread.Posts {
			for _, pubKeyData := range metaforoComment.User.Web3PublicKeys {
				metaforoUserRcd := &model.MetaforoUser{
					MetaforoUserId: metaforoComment.UserId,
					UserWallet:     common.FormatUserWallet(pubKeyData.Address),
				}
				err = tx.Where(&metaforoUserRcd).FirstOrCreate(&metaforoUserRcd).Error
				if err != nil {
					log.Warn().Msgf("create metaforo user record error: %+v, continue to next record", err)
				}
			}
		}
		return nil
	}); err != nil {
		log.Warn().Msgf("update metaforo user record error: %+v", err)
		return err
	} else {
		log.Debug().Msgf("update metaforo user record success")
		return nil
	}
}

// UpdateDbVoteOptionRecordsFromMetaforoProposalResponse This Go code snippet defines a function that updates proposal vote option records from a Metaforo response.
// It iterates through the polls in the Metaforo response, updates the database with the poll information, and handles different poll statuses.
// It returns a flag tells invoker whether the poll status has changed or not.
func UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcdId uint, metaforoProposal *metaforo.ProposalResponse) (bool, error) {
	var err error
	var dbProposalRcd *model.Proposal
	db.Find(&dbProposalRcd, dbProposalRcdId)

	pollStatusChanged := false

	for _, poll := range metaforoProposal.Thread.Polls {
		var proposalVoteRecord = model.ProposalVoteRecord{}
		if err = db.Where(&model.ProposalVoteRecord{ProposalID: dbProposalRcdId}).Updates(&model.ProposalVoteRecord{
			Title:      poll.Title,
			MetaforoID: poll.Id,
			StartTs:    poll.PollStartAt.UTC().Unix(),
			EndTs:      poll.CloseAt.UTC().Unix(),
			ProposalID: dbProposalRcdId,
			VoteType:   dbProposalRcd.VoteType,
		}).First(&proposalVoteRecord).Error; err != nil {
			log.Warn().Msgf("save DB proposal vote record error: %+v", err)
			return pollStatusChanged, err
		} else {
			log.Debug().Msgf("save DB proposal vote record success: %+v", proposalVoteRecord)
		}

		api.PrintStructAsJson(proposalVoteRecord, "TTT: proposal vote record")

		currState := proposalVoteRecord.State
		if currState != poll.Status {
			pollStatusChanged = true
			err := db.Model(&model.ProposalVoteRecord{}).
				Where("metaforo_id = ? AND state =?", poll.Id, currState).
				Update("state", poll.Status).Error
			if err != nil {
				log.Error().Msgf("update proposal vote record state error: %+v", err)
				return pollStatusChanged, err
			}
		} else {
			log.Debug().Msgf("proposal vote record state: %s, new state: %s", currState, poll.Status)
			pollStatusChanged = false
		}

		db.Find(&proposalVoteRecord, proposalVoteRecord.ID)

		err = db.Transaction(func(tx *gorm.DB) error {
			for _, voteOpt := range poll.Options {
				proposalVoteOptionRecord := model.ProposalVoteOptionRecord{
					MetaforoID:           voteOpt.Id,
					MetaforoVoteID:       poll.Id,
					ProposalVoteRecordId: proposalVoteRecord.ID,
				}

				// Get option label from metaforo response
				var optLabel string
				switch reflect.TypeOf(voteOpt.Html).Kind() {
				case reflect.Float64:
					optLabel = strconv.FormatFloat(voteOpt.Html.(float64), 'f', -1, 64)
				default:
					optLabel = voteOpt.Html.(string)
				}

				err = tx.Model(&model.ProposalVoteOptionRecord{}).
					Where("proposal_id = ? AND text =?", dbProposalRcdId, optLabel).
					Updates(&model.ProposalVoteOptionRecord{
						VoterCount:     voteOpt.Voters,
						MetaforoID:     voteOpt.Id,
						MetaforoVoteID: poll.Id,
					}).Error
				if err != nil {
					log.Warn().Msgf("save DB proposal vote option error: %+v", err)
					return err
				} else {
					log.Debug().Msgf("update DB proposal vote option success: %+v", proposalVoteOptionRecord)
				}
				api.PrintStructAsJson(proposalVoteOptionRecord, "TTT: proposal vote option record")
			}
			return nil
		})

		if err != nil {
			log.Warn().Msgf("update proposal vote DB record error: %+v", err)
			return pollStatusChanged, err
		} else {
			log.Debug().Msgf("update proposal vote DB record success. voteRecord: %+v", proposalVoteRecord)
			return pollStatusChanged, nil
		}
	}

	return pollStatusChanged, nil
}

func HandleProposalPollStatusChange(db *gorm.DB, proposalId uint) error {
	var pVoteRcds []*model.ProposalVoteRecord
	err := db.Model(&model.ProposalVoteRecord{}).Where("proposal_id = ?", proposalId).Find(&pVoteRcds).Error
	if err != nil {
		log.Error().Msgf("get proposal status error: %+v", err)
		return err
	}

	if len(pVoteRcds) == 0 {
		err = fmt.Errorf("proposal %d has no vote records associated, skip", proposalId)
		log.Error().Msg(err.Error())
		return err
	}

	effectVoteRcd := pVoteRcds[0]
	log.Debug().Msgf("effect proposal vote records: %+v", effectVoteRcd)

	// Metaforo support multiple vote in one proposal, but in OS only one vote will be created, so only check the first value
	if effectVoteRcd.State == "open" {
		// Refresh dbProposal record
		var dbProposalRcd model.Proposal
		db.Find(&dbProposalRcd, proposalId)

		if dbProposalRcd.Sip != 0 {
			log.Debug().Msgf("proposal %d has SIP set, no update will be performed even poll is open", proposalId)
			return nil
		}

		log.Debug().Msgf("proposal %d has no SIP set, update the value, current proposal: %+v", proposalId, dbProposalRcd)
		if !dbProposalRcd.IsInFinState() && dbProposalRcd.State != int(model.ProposalStateVoting) {
			// Query proposal template to check whether it is closing proposal
			var pTemplate *model.ProposalTemplate
			if err = db.Model(&dbProposalRcd).Association("ProposalTemplate").Find(&pTemplate); err != nil {
				log.Error().Msgf("get proposal template error: %+v", err)
				return err
			}

			// setProposalSip can change the proposal state to voting, and the query contains condition, so the code is lock free
			if err = setProposalSip(db, pTemplate, &dbProposalRcd); err != nil {
				log.Error().Msgf("set proposal sip error: %+v", err)
				return err
			}

			log.Debug().Msgf("proposal %d has gained sip %d successfully", dbProposalRcd.ID, dbProposalRcd.Sip)
			return nil
		} else {
			log.Error().Msgf("proposal %d has no SIP set, but state is not updatable, please verify. proposal data: %+v", proposalId, dbProposalRcd)
			return nil
		}
	} else if effectVoteRcd.State == "close" {
		err := UpdateProposalStateAfterVoteClosed(db, proposalId, effectVoteRcd)
		if err != nil {
			log.Warn().Msgf("process proposal state error: %+v", err)
			return err
		} else {
			log.Debug().Msgf("process proposal state success")
			return nil
		}
	} else {
		log.Warn().Msgf("unknown poll status: %+v", effectVoteRcd.State)
		return nil
	}
}

// UpdateProposalStateAfterVoteClosed will be invoked when vote for proposal has changed to closed state
// It will update the proposal state based on vote result and external check rules if configured
// TODO: Check how to migrate this state change function into UpdateProposalStateAndLaunchStateChangeActions
func UpdateProposalStateAfterVoteClosed(db *gorm.DB, proposalId uint, pVoteRcd *model.ProposalVoteRecord) error {
	if TryAcquireUpdateProposalDbLockOrReturn(proposalId) == false {
		err := fmt.Errorf("proposal %d is updating", proposalId)
		log.Error().Msg(err.Error())
		return err
	}
	defer ReleaseUpdateProposalDbLock(proposalId)

	log.Debug().Msgf("update proposal %d state after vote closed", proposalId)

	var dbProposalRcd model.Proposal
	err = db.Where(&model.Proposal{ID: proposalId}).First(&dbProposalRcd).Error
	if err != nil {
		log.Warn().Msgf("get proposal record error: %+v", err)
		return err
	}

	api.PrintStructAsJson(dbProposalRcd, "TTT: before update")
	if dbProposalRcd.IsInFinState() || dbProposalRcd.State == int(model.ProposalStatePendingExecution) {
		log.Warn().Msgf("proposal %d in state %d, not need to apply post job.", dbProposalRcd.ID, dbProposalRcd.State)
		return nil
	}

	var voteOptRcds []*model.ProposalVoteOptionRecord
	err = db.Where(&model.ProposalVoteOptionRecord{ProposalVoteRecordId: pVoteRcd.ID}).Find(&voteOptRcds).Error
	if err != nil {
		log.Warn().Msgf("fetch vote options for proposal vote record: %+v error: %+v", pVoteRcd, err)
		return err
	}

	totalVoterCount := lo.SumBy(voteOptRcds, func(r *model.ProposalVoteOptionRecord) int { return r.VoterCount })

	var proposalFinalState model.ProposalState
	var voteResult string
	switch pVoteRcd.VoteType {
	case model.ProposalVoteTypeNone:
		if dbProposalRcd.PendingExecutionSecond == 0 {
			proposalFinalState = model.ProposalStateExecuted
		} else {
			proposalFinalState = model.ProposalStatePendingExecution
		}
	case model.ProposalVoteTypeCustomerDefinedAlwaysPassed:
		if totalVoterCount == 0 {
			proposalFinalState = model.ProposalStateVoteFailed
		} else {
			if dbProposalRcd.PendingExecutionSecond == 0 {
				proposalFinalState = model.ProposalStateExecuted
			} else {
				proposalFinalState = model.ProposalStatePendingExecution
			}
		}
	case model.ProposalVoteTypeDecision:
		if totalVoterCount == 0 {
			proposalFinalState = model.ProposalStateVoteFailed
		} else {
			approvedCount := 0
			for _, r := range voteOptRcds {
				if r.Text == internal.ProposalDecisionApprove {
					approvedCount = r.VoterCount
				}
			}

			if approvedCount > totalVoterCount/2 {
				proposalFinalState = model.ProposalStateVotePassed
				voteResult = "1"
			} else {
				proposalFinalState = model.ProposalStateVoteFailed
				voteResult = "0"
			}
		}
	case model.ProposalVoteTypeNumericAvg:
		if totalVoterCount == 0 {
			proposalFinalState = model.ProposalStateVoteFailed
		} else {
			proposalFinalState = model.ProposalStateVotePassed

			// Sort the vote option records in desc order
			slices.SortFunc(voteOptRcds, func(a, b *model.ProposalVoteOptionRecord) int {
				return cmp.Compare(b.VoterCount, a.VoterCount)
			})

			// Save records used for calc result into additional array,
			// then go through the sorted vote options to find records with duplicated voter count
			recordsForCalcResults := []*model.ProposalVoteOptionRecord{voteOptRcds[0]}
			maxVoterCount := voteOptRcds[0].VoterCount
			for _, rcd := range voteOptRcds[1:] {
				if rcd.VoterCount < maxVoterCount {
					// The record's voter count is less than max value, break out and do calc with saved records
					break
				}
				recordsForCalcResults = append(recordsForCalcResults, rcd)
			}

			if len(recordsForCalcResults) > 1 {
				var decimalVals []decimal.Decimal
				for i := range recordsForCalcResults {
					optRcd := recordsForCalcResults[i]
					decimalVal, err := decimal.NewFromString(optRcd.Value)
					if err != nil {
						log.Error().Msgf("parse vote option value error: %+v, vote option: %+v", err, optRcd)
						continue
					}
					decimalVals = append(decimalVals, decimalVal)
				}

				voteResult = decimal.Avg(decimalVals[0], decimalVals[1:]...).String()
			} else {
				voteResult = recordsForCalcResults[0].Value
			}
		}
	case model.ProposalVoteTypeNumericSingle, model.ProposalVoteTypeCustomerDefinedEqualFailed:
		if totalVoterCount == 0 {
			proposalFinalState = model.ProposalStateVoteFailed
		} else {
			var voteOptRcds []*model.ProposalVoteOptionRecord
			err = db.Where(&model.ProposalVoteOptionRecord{ProposalVoteRecordId: pVoteRcd.ID}).Find(&voteOptRcds).Error
			if err != nil {
				log.Warn().Msgf("fetch vote options for proposal vote record: %+v error: %+v", pVoteRcd, err)
				return err
			}

			if len(voteOptRcds) > 1 {
				// Sort the vote option records in desc order
				slices.SortFunc(voteOptRcds, func(a, b *model.ProposalVoteOptionRecord) int {
					return cmp.Compare(b.VoterCount, a.VoterCount)
				})

				if voteOptRcds[0].VoterCount == voteOptRcds[1].VoterCount {
					log.Warn().Msgf("vote result has same voter count, mark as failed")
					proposalFinalState = model.ProposalStateVoteFailed
				} else {
					proposalFinalState = model.ProposalStateVotePassed
					voteResult = voteOptRcds[0].Value
				}
			} else if len(voteOptRcds) == 1 {
				proposalFinalState = model.ProposalStateVotePassed
				voteResult = voteOptRcds[0].Value
			} else {
				log.Error().Msgf("vote option count error, set proposal to failed")
				proposalFinalState = model.ProposalStateVoteFailed
			}
		}
	default:
		log.Warn().Msgf("unknown proposal type, no logic to set the proposal state")
		return err
	}

	if proposalFinalState == model.ProposalStateVotePassed && dbProposalRcd.ExtraResultCheckRule != nil {
		currSeason, err := model.GetCurrentSeason(db)
		if err != nil {
			log.Error().Msgf("get current season error: %+v", err)
			return err
		}
		proposalFinalState = updateProposalStateByExtraCheckRule(dbProposalRcd.ExtraResultCheckRule, totalVoterCount, currSeason.Idx)
	}

	log.Debug().Msgf("update proposal %d state from %d to %+v", dbProposalRcd.ID, dbProposalRcd.State, proposalFinalState)
	err = db.Model(&dbProposalRcd).Where(&model.Proposal{ID: dbProposalRcd.ID}).Update("state", proposalFinalState).Error
	api.PrintStructAsJson(dbProposalRcd, "TTT: proposal record after update")
	if err != nil {
		log.Error().Msgf("update proposal state to %d error: %+v. DB proposal: %+v", proposalFinalState, err, dbProposalRcd)
		return err
	}

	// refresh db record
	db.Find(&dbProposalRcd, dbProposalRcd.ID)
	if !dbProposalRcd.IsInFinState() {
		if err = createProposalAutomationTasks(db, dbProposalRcd.ID, proposalFinalState, voteResult, dbProposalRcd.VoteType); err != nil {
			log.Error().Msgf("create proposal automation tasks error: %+v", err)
			return err
		}
	}

	return nil
}

// createProposalAutomationTasks creates automation tasks after proposal finished (passed or failed)
// In this function, the lock of proposal is still in hold
func createProposalAutomationTasks(db *gorm.DB, proposalId uint, finState model.ProposalState, voteResult string, voteType int) error {
	log.Debug().Msgf("enter createProposalAutomationTasks proposal id: %d, finState: %+v", proposalId, finState)
	sqlQuery := QueryComponentActionNameBaseSQL + " WHERE proposal_id = ?"
	var proposalComponentActions []*proposalComponentActions
	err := db.Raw(sqlQuery, proposalId).Find(&proposalComponentActions).Error
	if err != nil {
		log.Error().Msgf("fetch proposal component actions error: %+v", err)
		return err
	}

	log.Debug().Msgf("proposal %d component actions: %+v", proposalId, proposalComponentActions)

	switch finState {
	case model.ProposalStateVotePassed:
		if len(proposalComponentActions) == 0 {
			log.Debug().Msgf("proposal %d has no component actions", proposalId)
			// No automation action found for the proposal, check whether the proposal has pending execution time
			// If yes, change the proposal state to pending execution, and create a new cron job to update proposal state after pending execution second
			// If no, change the proposal state to executed directly
			// 2024.02.12: for now, only create project proposal can reach this branch
			var proposal model.Proposal
			if err = db.Find(&proposal, proposalId).Error; err != nil {
				log.Error().Msgf("find proposal error: %+v", err)
				return err
			}

			pTmplType, err := getProposalTemplateType(db, *proposal.ProposalTemplateID)
			if err != nil {
				log.Error().Msgf("get proposal template type error: %+v", err)
				return err
			}

			if pTmplType != model.ProposalTemplateTypeNewProject {
				log.Debug().Msgf("proposal %d is not a create project proposal", proposalId)
				return nil
			} else {
				log.Debug().Msgf("proposal %d is a create project proposal", proposalId)
				if proposal.PendingExecutionSecond != 0 {
					// Create cronjob to update proposal state
					// cronjob requires an associated proposal component record, create a new one with 0 component_id

					var proposalComponentRecord model.ProposalComponentRecord
					if err = db.Model(&proposalComponentRecord).
						Where(map[string]any{"proposal_id": proposal.ID, "component_id": 0}). // Note: 0 won't be passed to query if using struct data
						First(&proposalComponentRecord).Error; err != nil {
						if errors.Is(err, gorm.ErrRecordNotFound) {
							proposalComponentRecord.ProposalID = proposal.ID
							proposalComponentRecord.ComponentID = 0
							log.Debug().Msgf("create proposal component record for automation tasks: %+v", proposalComponentRecord)
							db.Create(&proposalComponentRecord)
						} else {
							log.Error().Msgf("create proposal component record error: %+v", err)
							return err
						}
					} else {
						log.Debug().Msgf("automation for updating state has already created, return")
						return nil
					}

					updateProposalStateTaskParams := map[string]any{
						"proposal_id": proposalId,
						"state":       int(model.ProposalStateExecuted),
					}

					jobParamsStr, err := json.Marshal(updateProposalStateTaskParams)
					if err != nil {
						log.Error().Msgf("marshal update proposal state params error: %+v", err)
						return err
					}
					// TODO: The cronjob created here should be create project instead of update proposal state
					err = createCronJob(db, proposal.ID, proposal.PendingExecutionSecond, internal.TaskUpdateProposalState, string(jobParamsStr), "", 0, int(proposalComponentRecord.ID))
					if err != nil {
						log.Error().Msgf("marshal update proposal state params error: %+v", err)
						return err
					}

					if err = db.Model(&proposal).Where("id = ?", proposalId).Update("state", model.ProposalStatePendingExecution).Error; err != nil {
						log.Error().Msgf("update proposal %d state to pending execution error", proposalId)
						return err
					}
					return nil
				} else {
					log.Error().Msgf("NOTICE: this log is generated for branch that creating project proposal with vote, WHICH IS NOT EXPECTED, if you found this log, check db or requirements")
					if err = db.Model(&proposal).Where("id = ?", proposalId).Update("state", model.ProposalStateExecuted).Error; err != nil {
						log.Error().Msgf("update proposal %d state to pending execution error", proposalId)
						return err
					}

					return nil
				}
			}
		}

		var proposal model.Proposal
		if err = db.Find(&proposal, proposalId).Error; err != nil {
			log.Error().Msgf("find proposal error: %+v", err)
			return err
		}

		for _, componentAction := range proposalComponentActions {
			log.Debug().Msgf("proposal %d vote finState: %+v, action: %+v", proposalId, finState, componentAction)
			var actionName string
			actionName = componentAction.ApproveActionName
			err = createCronJob(db, proposal.ID, proposal.PendingExecutionSecond, actionName, componentAction.ComponentParams, voteResult, voteType, componentAction.ProposalComponentRecordId)
			if err != nil {
				log.Error().Msgf("create cron job error: %+v", err)
				return err
			}

			// Update proposal state to PendingExecution, the next state change will be launched by cron job or veto proposal
			// TODO: Change to single state transaction function
			err = db.Model(&proposal).Where("id = ?", proposal.ID).Update("state", int(model.ProposalStatePendingExecution)).Error
			if err != nil {
				log.Error().Msgf("update proposl state to pending execution error: %+v", err)
				return err
			} else {
				log.Debug().Msgf("update proposal %d state to pending execution", proposal.ID)
				return nil
			}
		}
		return nil
	case model.ProposalStateVoteFailed:
		var proposal model.Proposal
		if err = db.Find(&proposal, proposalId).Error; err != nil {
			log.Error().Msgf("find proposal error: %+v", err)
			return err
		}

		//For close project proposal, need to change the project status to close_failed
		proposalIsForClosingProject, project, err := IsProposalIsForClosingProject(db, proposal.ID)
		if err != nil {
			log.Error().Msgf("checking proposal is for closing project failed, err: %+v", err)
			return err
		}

		if !proposalIsForClosingProject {
			log.Debug().Msgf("proposal is not for closing project, skip other actions")
			return nil
		}

		log.Debug().Msgf("proposal %d is for closing project: %+v", proposal.ID, proposalIsForClosingProject)

		updateTx := db.Model(&project).
			Where("status = 'closing'").
			Update("status", model.ProjectStatusCloseFailed)

		if err = updateTx.Error; err != nil {
			err = fmt.Errorf("set project to closed failed status error: %+v", err)
			db.Model(&proposal).Where("id = ? AND state=?", proposal.ID, model.ProposalStateVoteFailed).Update("state", int(model.ProposalStateExecutionFailed))
			return err
		} else if updateTx.RowsAffected == 0 {
			err = fmt.Errorf("project not in correct status for updating to %+v", model.ProjectStatusCloseFailed)
			log.Error().Msgf(err.Error())
			db.Model(&proposal).Where("id = ? AND state=?", proposal.ID, model.ProposalStateVoteFailed).Update("state", int(model.ProposalStateExecutionFailed))
			return err
		} else {
			log.Debug().Msgf("complete project status update")
			return nil
		}
	default:
		err = fmt.Errorf("unknown proposal state: %d, mark ask execution failed", finState)
		log.Error().Msgf(err.Error())
		db.Model(model.Proposal{}).Where("id = ?", proposalId).Update("state", int(model.ProposalStateExecutionFailed))
		return err
	}
}

func createCronJob(db *gorm.DB, dbProposalId uint, proposalPendingExecutionSecond int64, actionName string, jobParams string, voteResult string, voteType int, pComponentRecordId int) error {
	currentTs := time.Now().UTC().Unix()
	proposalExecutionTs := currentTs + proposalPendingExecutionSecond

	finTask := &model.CronJob{
		CreateTs:                  currentTs,
		UpdateTs:                  currentTs,
		HandlerName:               actionName,
		ProposalComponentRecordId: pComponentRecordId,
		ProposalId:                dbProposalId,
		LastExecTs:                0,
		NextExecTs:                proposalExecutionTs,
		JobParams:                 jobParams,
		VoteResult:                voteResult,
		VoteType:                  voteType,
		State:                     model.CronJobStateActive,
		LastExecResult:            "",
	}
	createTaskTx := db.Where(model.CronJob{HandlerName: actionName, ProposalComponentRecordId: pComponentRecordId, ProposalId: dbProposalId}).
		Assign(&finTask).FirstOrCreate(&finTask)

	if createTaskTx.Error != nil {
		log.Error().Msgf("create proposal fin task error: %+v", createTaskTx.Error)
		return createTaskTx.Error
	} else if createTaskTx.RowsAffected == 0 {
		log.Warn().Msgf("proposal fin task already exists: %+v", finTask)
	}

	return nil
}

// updateProposalStateByExtraCheckRule update the state of proposal based on the extra check rules
// It iterates through the rules, compares certain metrics with the provided values, and based on the comparison,
// determines if the proposal has passed or failed the extra check rules.
// If all the checks pass, it returns ProposalStateVotePassed, otherwise it returns ProposalStateVoteFailed.
func updateProposalStateByExtraCheckRule(checkRules []*model.ExtraResultCheckRuleData, totalVoterCount int, seasonIdx uint) model.ProposalState {
	log.Debug().Msgf("enter updatePropsalStateByExtraCheckRule: %+v, totalVoter: %d", checkRules, totalVoterCount)
	indexClient := sdk.GetIndexerClient()
	checkPassed := false
	for _, r := range checkRules {
		log.Debug().Msgf("updateProposalStateByExtraCheckRule: check rule: %+v", r)
		ruleValue, err := strconv.ParseFloat(r.Value, 64)
		if err != nil {
			log.Warn().Msgf("updateProposalStateByExtraCheckRule: error value in checking rule: %+v", r)
			continue
		}
		valueToBeCompared := 0
		switch r.Metric {
		// TODO: Get data from cache
		case internal.ExtraCheckRuleMetricSeed:
			valueToBeCompared = indexClient.GetCurrentSeedHolderCount()
		case internal.ExtraCheckRuleMetricCurrentSeasonNode:
			valueToBeCompared = indexClient.GetCurrentSeasonNodeCount(fmt.Sprintf("%d", seasonIdx))
		default:
			log.Warn().Msgf("updateProposalStateByExtraCheckRule: unknown metric: %s", r.Metric)
		}
		log.Debug().Msgf("updateProposalStateByExtraCheckRule: value to be compared: %+v", valueToBeCompared)

		switch r.CheckType {
		case internal.ExtraCheckRuleTypeRatio:
			checkPassed = float64(totalVoterCount*100.0/valueToBeCompared) >= ruleValue
		case internal.ExtraCheckRuleTypeCount:
			checkPassed = float64(totalVoterCount) >= ruleValue
		default:
			log.Warn().Msgf("updateProposalStateByExtraCheckRule: unknown extra check type: %s", r.CheckType)
		}

		log.Debug().Msgf("updateProposalStateByExtraCheckRule: check passed: %+v", checkPassed)

		if !checkPassed {
			break
		}
	}

	if checkPassed {
		return model.ProposalStateVotePassed
	} else {
		return model.ProposalStateVoteFailed
	}
}

func getProposalComponentIdNameMapping(db *gorm.DB) map[uint]string {
	rsltBytes, err := storage.GetCachedData("component_name_id_mapping")
	if err == nil {
		rslt := map[uint]string{}
		if err = json.Unmarshal(rsltBytes, &rslt); err != nil {
			log.Error().Msgf("unmarshal component name id mapping error: %+v", err)
			return map[uint]string{}
		}
		return rslt
	}

	if !errors.Is(err, bigcache.ErrEntryNotFound) {
		log.Error().Msgf("fetch component name id mapping error: %+v", err)
		return map[uint]string{}
	}

	// Cache missing

	var components []*model.ProposalComponent
	if err := db.Model(&model.ProposalComponent{}).Find(&components).Error; err != nil {
		log.Error().Msgf("fetch proposal component error: %+v", err)
		return make(map[uint]string)
	}

	rslt := make(map[uint]string)
	for _, c := range components {
		rslt[c.ID] = c.Name
	}

	rsltBytes, err = json.Marshal(rslt)
	if err != nil {
		log.Error().Msgf("marshal component name id mapping error: %+v", err)
		return rslt
	}
	if err = storage.StoreCachedData("component_name_id_mapping", rsltBytes); err != nil {
		log.Error().Msgf("store component name id mapping error: %+v", err)
		return rslt
	}

	return rslt
}

func CreateProjectFromAutoTasks(db *gorm.DB, proposalId uint) (*model.Project, error) {
	var err error

	var proposal model.Proposal
	db.Find(&proposal, proposalId)

	var pTemplate model.ProposalTemplate
	err = db.Find(&pTemplate, proposal.ProposalTemplateID).Error
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return nil, err
	}

	var pCategory model.ProposalCategory
	err = db.Find(&pCategory, pTemplate.ProposalCategoryID).Error
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return nil, err
	}

	newProjectData := model.Project{
		Proposals:    []string{fmt.Sprintf("%d", proposal.ID)},
		Name:         proposal.Title,
		SIP:          fmt.Sprintf("%d", proposal.Sip),
		ApprovalLink: fmt.Sprintf("/proposal/thread/%d", proposal.ID),
		CreateTs:     model.GetCurrentUtcEpochSecond(),
		UpdateTs:     model.GetCurrentUtcEpochSecond(),
		Status:       model.ProjectStatusOpen,
		Category:     pCategory.Name,
		Sponsors: []string{
			common.FormatUserWallet(proposal.Applicant),
		},
	}

	log.Error().Msgf("TTT: project record: %+v", newProjectData)

	// Load data from component records
	var pComponents []*model.ProposalComponentRecord
	if err = db.Model(&proposal).Association("Components").Find(&pComponents); err != nil {
		log.Error().Msgf("fetch proposal components error: %+v", err)
		return nil, err
	}

	// Saves db budget records data
	var projectBudgetRcds []*model.ProjectBudget

	for _, pComponentRecord := range pComponents {
		api.PrintStructAsJson(pComponentRecord, "TTT: component record")
		if compName, found := getProposalComponentIdNameMapping(db)[pComponentRecord.ComponentID]; found {
			if compName == internal.ComponentNameBudgetP1 {
				var budgetParams budgetComponentDataP1
				err := json.Unmarshal([]byte(pComponentRecord.Data), &budgetParams)
				if err != nil {
					log.Error().Msgf("unmarshal project deliverables data error: %+v", err)
					return nil, err
				}

				projectBudgetRcds = budgetParams.prepareBudgetRecords(proposalId)
			} else if compName == internal.ComponentNameBudget {
				var budgetParams budgetComponentData
				err := json.Unmarshal([]byte(pComponentRecord.Data), &budgetParams)
				if err != nil {
					log.Error().Msgf("unmarshal project deliverables data error: %+v", err)
					return nil, err
				}

				projectBudgetRcds = budgetParams.prepareBudgetRecords(proposalId)
			} else if compName == internal.ComponentNameDeliverables {
				var deliverableParams commonCreateProjectRelatedData
				err := json.Unmarshal([]byte(pComponentRecord.Data), &deliverableParams)
				if err != nil {
					log.Error().Msgf("unmarshal project deliverables data error: %+v", err)
					return nil, err
				}
				newProjectData.Deliverable = deliverableParams.Desc
			} else if compName == internal.ComponentNameDeadline {
				var deadlineParams commonCreateProjectRelatedData
				err := json.Unmarshal([]byte(pComponentRecord.Data), &deadlineParams)
				if err != nil {
					log.Error().Msgf("unmarshal project deadline data error: %+v", err)
					return nil, err
				}
				deadlineTs, err := time.Parse(time.RFC3339, deadlineParams.Desc)
				if err != nil {
					log.Error().Msgf("parse deadline date error: %+v", err)
					return nil, err
				}
				newProjectData.PlanTime = fmt.Sprintf("%d", deadlineTs.UTC().UnixMilli())
			}
		}
	}

	// TODO: Add uniq index field to project to avoid duplicated creation
	if err = db.Create(&newProjectData).Error; err != nil {
		log.Error().Msgf("create project error: %+v", err)
		return nil, err
	}

	if len(projectBudgetRcds) > 0 {
		projectBudgetRcds = lo.Map(projectBudgetRcds, func(item *model.ProjectBudget, index int) *model.ProjectBudget {
			item.ProjectID = newProjectData.ID
			return item
		})
		if err = db.Create(&projectBudgetRcds).Error; err != nil {
			log.Error().Msgf("create project budget record error: %+v", err)
			return nil, err
		}
	}

	return &newProjectData, nil
}

func IsProposalIsForClosingProject(db *gorm.DB, proposalId uint) (bool, *model.Project, error) {
	var proposal model.Proposal
	db.Find(&proposal, proposalId)

	pTmplType, err := getProposalTemplateType(db, *proposal.ProposalTemplateID)
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return false, nil, err
	}

	if pTmplType != model.ProposalTemplateTypeCloseProject {
		return false, nil, nil
	}

	createProjectProposal := model.Proposal{ID: proposal.AssociateProposalId}
	if err = db.Find(&createProjectProposal).Error; err != nil {
		log.Error().Msgf("get creating project proposal error: %+v", err)
		return false, nil, err
	}

	createdProject := model.Project{
		SIP: fmt.Sprintf("%d", createProjectProposal.Sip),
	}

	if err := db.Model(&createdProject).Where(&createdProject).First(&createdProject).Error; err != nil {
		log.Error().Msgf("get associated project error: %+v", err)
		return false, nil, err
	}
	return true, &createdProject, nil
}

func setProposalSip(db *gorm.DB, pTemplate *model.ProposalTemplate, dbProposalRcd *model.Proposal) error {
	var proposalSip = 0
	db.Find(&dbProposalRcd, dbProposalRcd.ID)

	if pTemplate != nil && pTemplate.Type == model.ProposalTemplateTypeCloseProject {
		createProjectProposal := model.Proposal{ID: dbProposalRcd.AssociateProposalId}
		if err = db.Find(&createProjectProposal).Error; err != nil {
			log.Error().Msgf("get creating project proposal error: %+v", err)
			return err
		}
		proposalSip = createProjectProposal.Sip
	} else {
		proposalSip, err = model.GetNextSipValue(db)
		log.Error().Msgf("TTT: get next sip value: %d", proposalSip)
		if err != nil {
			log.Error().Msgf("get next sip value error: %+v", err)
			return err
		}
	}

	// Only update proposal has same state with passed in object
	updateTx := db.Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"}).
		Model(&dbProposalRcd).
		Where("id = ? AND state = ? AND sip = 0", dbProposalRcd.ID, dbProposalRcd.State).
		Updates(&model.Proposal{Sip: proposalSip, State: int(model.ProposalStateVoting)})

	if err = updateTx.Error; err != nil {
		log.Error().Msgf("update proposal state error: %+v", err)
		rollbackedSip, _ := model.RollbackSipValueByOne(db)
		log.Debug().Msgf("rollbacked sip value return %d", rollbackedSip)
		return err
	} else if updateTx.RowsAffected == 0 {
		err = fmt.Errorf("proposal %d already has a sip value, no update will be performed", dbProposalRcd.ID)
		log.Error().Msgf(err.Error())
		rollbackedSip, _ := model.RollbackSipValueByOne(db)
		log.Debug().Msgf("rollbacked sip value return %d", rollbackedSip)
		return err
	} else {
		log.Debug().Msgf("complete update proposal status")
		return nil
	}
}

// verifyProjectCanBeClosed verifies whether project related to proposal is in open or close_failed status
func verifyProjectCanBeClosed(db *gorm.DB, createProjectProposalId uint) (bool, error) {
	log.Debug().Msgf("verify project can be closed: %d", createProjectProposalId)
	dbPrjRcd, err := findProjectCreatedByProposal(db, createProjectProposalId)
	if err != nil {
		log.Error().Msgf("find project created by proposal error: %+v", err)
		return false, err
	}
	log.Debug().Msgf("project created by proposal: %+v", dbPrjRcd)
	return dbPrjRcd.Status == model.ProjectStatusOpen || dbPrjRcd.Status == model.ProjectStatusCloseFailed, nil
}

func updateProposalAssociatedProjectStatusInCloseProjectToClosing(db *gorm.DB, reqData CreateOrUpdateProposalData) error {
	pTmplType, err := getProposalTemplateType(db, reqData.TemplateId)
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return err
	}

	// In close project proposal, verify whether the project to be closed is in open or closed_failed state,
	// and only set project to closing in those status. For other cases, return error
	if pTmplType == model.ProposalTemplateTypeCloseProject {
		var createProjectProposal model.Proposal
		if err = db.Find(&createProjectProposal, reqData.CreateProjectProposalId).Error; err != nil {
			log.Error().Msgf("get create project proposal error: %+v", err)
			return err
		}

		createdProject := model.Project{
			SIP: fmt.Sprintf("%d", createProjectProposal.Sip),
		}

		updateTx := db.Clauses(clause.Locking{Strength: "UPDATE"}).
			Model(&createdProject).
			Where(&createdProject).
			Where("status IN ('open', 'close_failed')").
			Update("status", model.ProjectStatusClosing)
		if err = updateTx.Error; err != nil {
			log.Error().Msgf("close project error: %+v", err)
			return err
		} else if updateTx.RowsAffected == 0 {
			err = fmt.Errorf("project not in correct status for updating to %+v", model.ProjectStatusClosing)
			log.Error().Msgf(err.Error())
			return err
		} else {
			log.Debug().Msgf("complete updating project status")
		}
	}
	return nil
}

func RefreshMetaforoAdminToken() {
	log.Debug().Msgf("refresh metaforo admin token job")
	db := storage.GetGormDB()
	mfData, err := model.GetMetaforoData(db)
	if err != nil {
		log.Error().Msgf("get metaforo data error: %+v", err)
		return
	}

	seeAuthToken, err := model.GetSeeAuthPk(db)
	if err != nil {
		log.Error().Msgf("get see auth pk error: %+v", err)
		return
	}

	mfAdminTokenResp, err := metaforo.GetUserToken(mfData[internal.SysVarMfAdminWalletPk], mfData[internal.SysVarMfAdminWalletAddr], seeAuthToken)
	if err != nil {
		log.Error().Msgf("get metaforo user token error: %+v", err)
		return
	}

	log.Debug().Msgf("prepare to update metaforo admin token")
	err = model.UpdateMetaforoAdminToken(db, mfAdminTokenResp.Token)
	if err != nil {
		log.Error().Msgf("update metaforo admin token error: %+v", err)
	} else {
		log.Debug().Msgf("update metaforo admin token done")
	}
}
