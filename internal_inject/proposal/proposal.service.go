package proposal_inject

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/allegro/bigcache/v3"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/project"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/db_agent"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/theseed-labs/os-backend/internal/storage"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProposalService struct {
}

func (s *ProposalService) TryAcquireUpdateProposalDbLockOrReturn(proposalId uint) bool {
	updateLock.Lock()
	defer updateLock.Unlock()
	if _, ok := updatingProposals[proposalId]; ok {
		return false
	} else {
		updatingProposals[proposalId] = true
		return true
	}
}

func (s *ProposalService) ReleaseUpdateProposalDbLock(proposalId uint) {
	updateLock.Lock()
	defer updateLock.Unlock()
	delete(updatingProposals, proposalId)
}

func (s *ProposalService) GetProposalFromStringId(db *gorm.DB, idStr string) (*model.Proposal, error) {
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
func (s *ProposalService) ValidateProposalComponentParams(db *gorm.DB, reqData *CreateOrUpdateProposalData, userWallet string, proposalId uint, cfg *config.Config) error {
	for _, componentData := range reqData.Components {
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
			//log.Error().Msgf("TTT: project budgts: %+v", projectBudgets)
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

			//log.Error().Msgf("TTT: remain budget amount: %+v", remainBudgetAmount)
			//
			//log.Error().Msgf("TTT: Component data: %+v", componentData)

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

			// Validate user address
			for _, r := range motivationComponentData.RewardList {
				if !common.ValidateUserWallet(r.Address) {
					err = fmt.Errorf("invalid address %s for motivation component %s", r.Address, r.AssetInfo.Name)
					log.Error().Msgf(err.Error())
					return err
				}
			}

			// Validate entity has enough budget
			componentRewardAmountData := lo.SliceToMap(motivationComponentData.RewardList, func(reward *model.ComponentMotivationRewardRecord) (string, decimal.Decimal) {
				return reward.AssetInfo.Name, decimal.RequireFromString(reward.Amount)
			})
			log.Error().Msgf("TTT: component reward amount: %+v", componentRewardAmountData)

			for assetName, remainBudgetAmount := range remainBudgetAmount {
				if componentBudgetAmount, found := componentRewardAmountData[assetName]; found {
					if componentBudgetAmount.GreaterThan(remainBudgetAmount) {
						return fmt.Errorf("motivation component %s budget amount %s is greater than project budget %s", assetName, componentBudgetAmount.String(), remainBudgetAmount.String())
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

func (s *ProposalService) SaveProposalRecordToDB(db *gorm.DB, reqData *CreateOrUpdateProposalData, userWallet string, proposalId uint, cfg *config.Config) (*model.Proposal, error) {
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
	if proposalId != 0 && !s.TryAcquireUpdateProposalDbLockOrReturn(proposalId) {
		err := fmt.Errorf("proposal %d is updating", proposalId)
		log.Error().Msg(err.Error())
		return nil, err
	}
	defer s.ReleaseUpdateProposalDbLock(proposalId)

	// Validate component data before processing the proposal
	componentValidationError := s.ValidateProposalComponentParams(db, reqData, userWallet, proposalId, cfg)
	if componentValidationError != nil {
		err := fmt.Errorf("component data validation error: %+v", componentValidationError)
		log.Error().Msg(err.Error())
		return nil, err
	}

	log.Debug().Msgf("save proposal record to DB: %+v, proposalIdStr: %d, user wallet: %s", reqData, proposalId, userWallet)

	// Update title for testing
	if cfg.MetaforoData.ProposalPrefix != "" && !strings.HasPrefix(reqData.Title, cfg.MetaforoData.ProposalPrefix) {
		reqData.Title = cfg.MetaforoData.ProposalPrefix + reqData.Title
	}

	var voteTimeProps model.VoteTimeProperties

	var pTemplate *model.ProposalTemplate
	err := db.Find(&pTemplate, reqData.TemplateId).Error
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return nil, err
	}

	var voteGates []*model.ProposalVoteGate
	voteGates, err = service.ProposalTemplateService.GetUsageVoteGates(db, pTemplate)
	if err != nil {
		log.Error().Msgf("get vote gates error: %+v", err)
		return nil, err
	}

	if len(voteGates) > 1 {
		err = fmt.Errorf("more than one vote gate found for template %d, only the frist one will be used", pTemplate.ID)
		log.Warn().Msg(err.Error())
	}

	voteTimeProps.PublicitySecond = pTemplate.PublicitySecond
	voteTimeProps.VoteDurationSecond = pTemplate.VoteDurationSecond
	voteTimeProps.PendingExecutionSecond = pTemplate.PendingExecutionSecond

	var pCategory model.ProposalCategory
	err = db.Find(&pCategory, pTemplate.ProposalCategoryID).Error
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
		proposalRcd.VoteStartTs = dbProposalRcd.VoteStartTs
		proposalRcd.VoteDurationSecond = dbProposalRcd.VoteDurationSecond
		proposalRcd.VoteDurationSecond = dbProposalRcd.VoteDurationSecond
		proposalRcd.AssociateProposalId = reqData.CreateProjectProposalId
		proposalRcd.IsMultipleVote = reqData.IsMultipleVote

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
			if err := s.SaveProposalContentRecords(tx, proposalRcd.ID, reqData.ContentBlocks); err != nil {
				log.Error().Msgf("create proposal block error: %+v", err)
				return err
			}

			// Create proposal components
			if err := s.SaveProposalComponentRecords(tx, proposalRcd.ID, userWallet, reqData.Components); err != nil {
				log.Error().Msgf("create proposal component blocks error: %+v", err)
				return err
			}

			if err := s.SaveProposalVoteOptionRecords(tx, proposalRcd.ID, pTemplate.VoteType, reqData.VoteOptions); err != nil {
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

		// If no vote gate found, using 0 for db record vote_gate_id field
		voteGateId := uint(0)
		if len(voteGates) > 0 {
			voteGateId = voteGates[0].ID
		}

		proposalRecord := model.Proposal{
			CreateTs:                time.Now().UTC().Unix(),
			Title:                   reqData.Title,
			Applicant:               common.FormatUserWallet(userWallet),
			ProposalCategoryID:      pTemplate.ProposalCategoryID,
			Version:                 1,
			VoteGateId:              voteGateId,
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
			if err := s.SaveProposalContentRecords(tx, proposalRecord.ID, reqData.ContentBlocks); err != nil {
				log.Error().Msgf("create proposal block error: %+v", err)
				return err
			}

			// Create proposal components
			if err := s.SaveProposalComponentRecords(tx, proposalRecord.ID, userWallet, reqData.Components); err != nil {
				log.Error().Msgf("create proposal component blocks error: %+v", err)
				return err
			}

			if err := s.SaveProposalVoteOptionRecords(tx, proposalRecord.ID, pTemplate.VoteType, reqData.VoteOptions); err != nil {
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

func (s *ProposalService) SaveProposalContentRecords(db *gorm.DB, proposalRecordId uint, reqContentBlockData []*FrontendContentBlockRecord) error {
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

func (s *ProposalService) SaveProposalComponentRecords(db *gorm.DB, proposalId uint, applicantWallet string, reqComponentData []*ComponentRequestData) error {
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
func (s *ProposalService) SaveProposalVoteOptionRecords(tx *gorm.DB, proposalId uint, voteType int, customVoteOptions []string) error {
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
func (s *ProposalService) SaveProposalToMetaforo(db *gorm.DB, dbProposalId uint, voteType int, metaforoAccessToken string, EditorType int, isMultipleVote bool, metaforoGroupName string) error {
	if !s.TryAcquireUpdateProposalDbLockOrReturn(dbProposalId) {
		err := fmt.Errorf("proposal %d is updating", dbProposalId)
		log.Error().Msg(err.Error())
		return err
	}
	defer s.ReleaseUpdateProposalDbLock(dbProposalId)

	log.Debug().Msgf("enter save proposal to metaforo: %d", dbProposalId)
	var err error

	var origProposalRecord model.Proposal
	if err = db.Find(&origProposalRecord, dbProposalId).Error; err != nil {
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
	err = db.Where(&model.ProposalContentBlock{ProposalID: dbProposalId}).Find(&contentBlocks).Error
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
		voteStartTime = time.Unix(origProposalRecord.VoteStartTs, 0).UTC()
		voteEndTime = time.Unix(origProposalRecord.VoteStartTs, 0).UTC().Add(origProposalRecord.VoteDuration())
		log.Debug().Msgf("Resubmit withdraw propoesal, vote start time: %s, vote end time: %s", voteStartTime.Format(time.RFC3339), voteEndTime.Format(time.RFC3339))

		if voteStartTime.Before(time.Now().UTC()) {
			err = fmt.Errorf("proposal vote start time %s is earlier than now", voteStartTime.String())
			log.Error().Msg(err.Error())
			return err
		}

		s.RefreshMetaforoAdminToken()
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
		var pTmpl *model.ProposalTemplate
		err = db.Find(&pTmpl, origProposalRecord.ProposalTemplateID).Error
		if err != nil {
			log.Error().Msgf("get proposal template error: %+v", err)
			return err
		}

		voteGates, err := service.ProposalTemplateService.GetUsageVoteGates(db, pTmpl)
		if err != nil {
			log.Error().Msgf("get vote gates error: %+v", err)
			return err
		}

		voteFormBytes, err := s.BuildMetaforoVoteFormDataBytes(db, dbProposalId, voteGates, voteStartTime, voteEndTime, isMultipleVote)
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

	err = s.UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord.ID, metaforoProposalResponse)
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
			VoteStartTs:      voteStartTime.UTC().Unix(),
		}).Error; err != nil {
		log.Error().Msgf("update proposal error: %+v", err)
		return err
	}

	pollStatusChanged, err := s.UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(db, updatedProposalRecord.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update propsal vote option records with metaforo response error: %+v", err)
		return err
	}

	if pollStatusChanged {
		if err = s.HandleProposalPollStatusChange(db, updatedProposalRecord.ID, metaforoGroupName); err != nil {
			log.Error().Msgf("handle proposal poll status change error: %+v", err)
			return err
		}
	}

	db.Find(&updatedProposalRecord, updatedProposalRecord.ID)
	log.Debug().Msgf("proposal saved to metafor successfully, now the object is: %+v", updatedProposalRecord)

	return nil
}

func (s *ProposalService) PrepareOsVoteOptions(voteType int, customVoteOptions []string) []string {
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
func (s *ProposalService) BuildMetaforoVoteFormDataBytes(db *gorm.DB, proposalId uint, voteGates []*model.ProposalVoteGate, startTime time.Time, endTime time.Time, isMultiple bool) ([]byte, error) {
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

		voteMax := 1
		if isMultiple {
			voteMax = len(mfVoteOpts)
		}

		voteData = append(voteData, &metaforo.NewVoteFormRequest{
			Options:            mfVoteOpts,
			Type:               "1",
			Title:              voteRecords[idx].Title,
			ShowType:           "3", // 1 - always visible, 2 - show after vote, 3 - show after vote closed
			ShowResult:         true,
			Period:             "1",
			CloseAt:            endTime.Format(time.RFC3339),
			VoteStartAt:        startTime.Format(time.RFC3339),
			Max:                voteMax, // How many options all use to select
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

	log.Debug().Msgf("metaforo vote data: %+v", voteData)

	voteDataBytes, err := json.Marshal(voteData)
	if err != nil {
		log.Error().Msgf("marshal proposal vote data error: %+v", err)
		return nil, err
	}
	return voteDataBytes, nil
}

// TODO: Change to use indexer data

func (s *ProposalService) IsUserMetVoteGate(userSeepassData *sdk.SeepassResponse, proposalVoteGate *model.ProposalVoteGate) bool {
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

func (s *ProposalService) UpdateDbRecordsFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcdId uint, metaforoProposal *metaforo.ProposalResponse) error {
	// Save all version proposals' arweave hash
	if err := s.UpdateArweaveHashFromMetaforoProposalResponse(db, dbProposalRcdId, metaforoProposal); err != nil {
		log.Error().Msgf("update arweave hash error: %+v", err)
		return err
	}

	if err := s.UpdateUserRecordsFromMetaforoProposalResponse(db, metaforoProposal); err != nil {
		log.Error().Msgf("update user records error: %+v", err)
		return err
	}

	return nil
}

// UpdateArweaveHashFromMetaforoProposalResponse updates proposal arweave hash from metaforo response
func (s *ProposalService) UpdateArweaveHashFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcdId uint, metaforoProposal *metaforo.ProposalResponse) error {
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

// UpdateUserRecordsFromMetaforoProposalResponse creates or updates user records from metaforo proposal response
// The user record returned from metaforo API contains user wallet, which can be used as uniq key of user
func (s *ProposalService) UpdateUserRecordsFromMetaforoProposalResponse(db *gorm.DB, metaforoProposal *metaforo.ProposalResponse) error {
	// Save user id and wallet from comments data
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, metaforoComment := range metaforoProposal.Thread.Posts {
			for _, pubKeyData := range metaforoComment.User.Web3PublicKeys {
				metaforoUserRcd := &model.MetaforoUser{
					MetaforoUserId: metaforoComment.UserId,
					UserWallet:     common.FormatUserWallet(pubKeyData.Address),
				}
				err := tx.Where(&metaforoUserRcd).FirstOrCreate(&metaforoUserRcd).Error
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
func (s *ProposalService) UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcdId uint, metaforoProposal *metaforo.ProposalResponse) (bool, error) {
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

				voterCount := voteOpt.Weights
				if voterCount == 0 {
					voterCount = voteOpt.Voters
				}

				err = tx.Model(&model.ProposalVoteOptionRecord{}).
					Where("proposal_id = ? AND text =?", dbProposalRcdId, optLabel).
					Updates(&model.ProposalVoteOptionRecord{
						// check this proposal 'https://forum.seedao.xyz/thread/search-52075', it has two voters has 2 seeds,
						// the result's `Voters` is `27`, but the `Weights` is `29`, so we change to use `Weights` property
						// If the vote is not created with wegiht, the weights value will be 0, change back to use voters
						// @2024/06/05
						//VoterCount:     voteOpt.Voters,
						VoterCount:     voterCount,
						MetaforoID:     voteOpt.Id,
						MetaforoVoteID: poll.Id,
					}).Error
				if err != nil {
					log.Warn().Msgf("save DB proposal vote option error: %+v", err)
					return err
				} else {
					log.Debug().Msgf("update DB proposal vote option success: %+v", proposalVoteOptionRecord)
				}
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

func (s *ProposalService) HandleProposalPollStatusChange(db *gorm.DB, proposalId uint, mfGroupName string) error {
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
			if err = s.SetProposalSip(db, pTemplate, &dbProposalRcd); err != nil {
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
		err := s.UpdateProposalStateAfterVoteClosed(db, proposalId, effectVoteRcd)
		if err != nil {
			log.Warn().Msgf("process proposal state error: %+v", err)
			return err
		} else {
			err = s.UpdateUserVoteRecordViaMetaforo(db, mfGroupName, proposalId)
			if err != nil {
				log.Warn().Msgf("update user vote record error: %+v", err)
				return err
			} else {
				log.Debug().Msgf("process proposal state success")
				return nil
			}
		}
	} else {
		log.Warn().Msgf("unknown poll status: %+v", effectVoteRcd.State)
		return nil
	}
}

// UpdateProposalStateAfterVoteClosed will be invoked when vote for proposal has changed to closed state
// It will update the proposal state based on vote result and external check rules if configured
// TODO: Check how to migrate this state change function into UpdateProposalStateAndLaunchStateChangeActions
func (s *ProposalService) UpdateProposalStateAfterVoteClosed(db *gorm.DB, proposalId uint, pVoteRcd *model.ProposalVoteRecord) error {
	if !s.TryAcquireUpdateProposalDbLockOrReturn(proposalId) {
		err := fmt.Errorf("proposal %d is updating", proposalId)
		log.Error().Msg(err.Error())
		return err
	}
	defer s.ReleaseUpdateProposalDbLock(proposalId)

	log.Debug().Msgf("update proposal %d state after vote closed", proposalId)

	var dbProposalRcd model.Proposal
	err := db.Where(&model.Proposal{ID: proposalId}).First(&dbProposalRcd).Error
	if err != nil {
		log.Warn().Msgf("get proposal record error: %+v", err)
		return err
	}

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
		log.Debug().Msgf("vote type decision, totalVoterCount: %d", totalVoterCount)
		if totalVoterCount == 0 {
			proposalFinalState = model.ProposalStateVoteFailed
		} else {
			approvedCount := 0
			for _, r := range voteOptRcds {
				if r.Text == internal.ProposalDecisionApprove {
					approvedCount = r.VoterCount
				}
			}
			log.Debug().Msgf("vote type decision, approvedCount: %d, totalVoterCount: %d", approvedCount, totalVoterCount)

			if approvedCount > totalVoterCount/2 {
				proposalFinalState = model.ProposalStateVotePassed
				voteResult = "1"
			} else {
				proposalFinalState = model.ProposalStateVoteFailed
				voteResult = "0"
			}
			log.Debug().Msgf("vote type decision, final state %d", proposalFinalState)
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
		proposalFinalState = s.UpdateProposalStateByExtraCheckRule(dbProposalRcd.ExtraResultCheckRule, totalVoterCount, currSeason.Idx)
	}

	log.Debug().Msgf("update proposal %d state from %d to %+v", dbProposalRcd.ID, dbProposalRcd.State, proposalFinalState)
	err = db.Model(&dbProposalRcd).Where(&model.Proposal{ID: dbProposalRcd.ID}).Update("state", proposalFinalState).Error
	if err != nil {
		log.Error().Msgf("update proposal state to %d error: %+v. DB proposal: %+v", proposalFinalState, err, dbProposalRcd)
		return err
	}

	// refresh db record
	db.Find(&dbProposalRcd, dbProposalRcd.ID)
	if !dbProposalRcd.IsInFinState() {
		if err = s.CreateProposalAutomationTasks(db, dbProposalRcd.ID, proposalFinalState, voteResult, dbProposalRcd.VoteType); err != nil {
			log.Error().Msgf("create proposal automation tasks error: %+v", err)
			return err
		}
	}

	return nil
}

// createProposalAutomationTasks creates automation tasks after proposal finished (passed or failed)
// In this function, the lock of proposal is still in hold
func (s *ProposalService) CreateProposalAutomationTasks(db *gorm.DB, proposalId uint, finState model.ProposalState, voteResult string, voteType int) error {
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

			pTmplType, err := s.GetProposalTemplateType(db, *proposal.ProposalTemplateID)
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
					err = s.CreateCronJob(db, proposal.ID, proposal.PendingExecutionSecond, internal.TaskUpdateProposalState, string(jobParamsStr), "", 0, int(proposalComponentRecord.ID))
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
			actionName := componentAction.ApproveActionName
			err = s.CreateCronJob(db, proposal.ID, proposal.PendingExecutionSecond, actionName, componentAction.ComponentParams, voteResult, voteType, componentAction.ProposalComponentRecordId)
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
		proposalIsForClosingProject, project, err := s.IsProposalIsForClosingProject(db, proposal.ID)
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

func (s *ProposalService) CreateCronJob(db *gorm.DB, dbProposalId uint, proposalPendingExecutionSecond int64, actionName string, jobParams string, voteResult string, voteType int, pComponentRecordId int) error {
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
func (s *ProposalService) UpdateProposalStateByExtraCheckRule(checkRules []*model.ExtraResultCheckRuleData, totalVoterCount int, seasonIdx uint) model.ProposalState {
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

func (s *ProposalService) GetProposalComponentIdNameMapping(db *gorm.DB) map[uint]string {
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

func (s *ProposalService) CreateProjectFromAutoTasks(db *gorm.DB, proposalId uint) (*model.Project, error) {
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
		if compName, found := s.GetProposalComponentIdNameMapping(db)[pComponentRecord.ComponentID]; found {
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

func (s *ProposalService) IsProposalIsForClosingProject(db *gorm.DB, proposalId uint) (bool, *model.Project, error) {
	var proposal model.Proposal
	db.Find(&proposal, proposalId)

	pTmplType, err := s.GetProposalTemplateType(db, *proposal.ProposalTemplateID)
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

func (s *ProposalService) SetProposalSip(db *gorm.DB, pTemplate *model.ProposalTemplate, dbProposalRcd *model.Proposal) error {
	txErr := db.Transaction(func(tx *gorm.DB) error {
		var proposalSip = 0
		db.Find(&dbProposalRcd, dbProposalRcd.ID)

		if pTemplate != nil && pTemplate.Type == model.ProposalTemplateTypeCloseProject {
			createProjectProposal := model.Proposal{ID: dbProposalRcd.AssociateProposalId}
			if err := tx.Find(&createProjectProposal).Error; err != nil {
				log.Error().Msgf("get creating project proposal error: %+v", err)
				return err
			}
			proposalSip = createProjectProposal.Sip
		} else {
			// proposalSip, err = model.GetNextSipValue(db)
			maxSipRow := tx.Table("proposals").Select("max(sip) + 1 as next_sip").Row()
			err := maxSipRow.Scan(&proposalSip)
			if err != nil {
				log.Error().Msgf("get max sip value error: %+v", err)
				return err
			}
			log.Error().Msgf("TTT: get next sip value: %d", proposalSip)
		}

		// Only update proposal has same state with passed in object
		updateTx := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "NOWAIT"}).
			Model(&dbProposalRcd).
			Where("id = ? AND state = ? AND sip = 0", dbProposalRcd.ID, dbProposalRcd.State).
			Updates(&model.Proposal{Sip: proposalSip, State: int(model.ProposalStateVoting)})

		if err := updateTx.Error; err != nil {
			log.Error().Msgf("update proposal state error: %+v", err)
			// if pTemplate != nil && pTemplate.Type == model.ProposalTemplateTypeCloseProject {
			// 	log.Debug().Msgf("no need rollbacked sip value proposalSip is %d", proposalSip)
			// } else {
			// 	rollbackedSip, _ := model.RollbackSipValueByOne(db)
			// 	log.Debug().Msgf("rollbacked sip value return %d", rollbackedSip)
			// }
			return err
		} else if updateTx.RowsAffected == 0 {
			err = fmt.Errorf("proposal %d already has a sip value, no update will be performed", dbProposalRcd.ID)
			log.Error().Msgf(err.Error())
			// if pTemplate != nil && pTemplate.Type == model.ProposalTemplateTypeCloseProject {
			// 	log.Debug().Msgf("no need rollbacked sip value proposalSip is %d", proposalSip)
			// } else {
			// 	rollbackedSip, _ := model.RollbackSipValueByOne(db)
			// 	log.Debug().Msgf("rollbacked sip value return %d", rollbackedSip)
			// }
			return err
		} else {
			log.Debug().Msgf("complete update proposal status")
			return nil
		}
	})
	return txErr
}

// verifyProjectCanBeClosed verifies whether project related to proposal is in open or close_failed status
func (s *ProposalService) VerifyProjectCanBeClosed(db *gorm.DB, createProjectProposalId uint) (bool, error) {
	log.Debug().Msgf("verify project can be closed: %d", createProjectProposalId)
	dbPrjRcd, err := s.FindProjectCreatedByProposal(db, createProjectProposalId)
	if err != nil {
		log.Error().Msgf("find project created by proposal error: %+v", err)
		return false, err
	}
	log.Debug().Msgf("project created by proposal: %+v", dbPrjRcd)
	return dbPrjRcd.Status == model.ProjectStatusOpen || dbPrjRcd.Status == model.ProjectStatusCloseFailed, nil
}

func (s *ProposalService) UpdateProposalAssociatedProjectStatusInCloseProjectToClosing(db *gorm.DB, reqData CreateOrUpdateProposalData) error {
	pTmplType, err := s.GetProposalTemplateType(db, reqData.TemplateId)
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

func (s *ProposalService) RefreshMetaforoAdminToken() {
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
	storage.GetConfig().MetaforoData.AccessToken = mfAdminTokenResp.Token
	if err != nil {
		log.Error().Msgf("update metaforo admin token error: %+v", err)
	} else {
		log.Debug().Msgf("update metaforo admin token done")
	}
}

///////////////////////
// Some converter functions
///////////////////////

func (s *ProposalService) ConvertProposalToFrontendDetailRecord(db *gorm.DB, proposalId uint, startPostId int, accessToken string, metaforoGroupName string) (*FrontendProposalDetailRecord, error, int) {
	var proposalBlocks []*model.ProposalContentBlock
	if err := db.Where(&model.ProposalContentBlock{ProposalID: proposalId}).Order("id").Find(&proposalBlocks).Error; err != nil {
		return nil, err, -1
	}

	var proposalComponentRecords []*model.ProposalComponentRecord
	if err := db.Where(&model.ProposalComponentRecord{ProposalID: proposalId}).
		Where("component_id != ?", 0).
		Order("id").Find(&proposalComponentRecords).Error; err != nil {
		return nil, err, -1
	}

	proposalContentResponse := lo.Map(proposalBlocks, func(item *model.ProposalContentBlock, _ int) *FrontendContentBlockRecord {
		return &FrontendContentBlockRecord{
			ID:            item.ID,
			Title:         item.Title,
			Content:       item.Content,
			Type:          item.Type,
			ComponentList: item.ComponentList,
		}
	})

	proposalComponentResponse := lo.Map(proposalComponentRecords, func(item *model.ProposalComponentRecord, _ int) *ComponentInstance {
		var componentRecord model.ProposalComponent
		err := db.Find(&componentRecord, item.ComponentID).Error
		if err != nil {
			log.Error().Msgf("fetch component %d from DB error: %+v", item.ComponentID, err)
			return nil
		}

		if componentRecord.Name == internal.ComponentNameAssociateProposal {
			// Special processing for `associate_proposal`
			var parsedData associatedProposalData
			err := json.Unmarshal([]byte(item.Data), &parsedData)

			if err != nil {
				log.Error().Msgf("unmarshal associate proposal data error: %+v", err)
			} else {
				var associatedProposalRecord model.Proposal
				err = db.Find(&associatedProposalRecord, parsedData.Proposal.Id).Error
				if err != nil {
					log.Error().Msgf("query associated proposal %d from DB error: %+v", parsedData.Proposal.Id, err)
				} else {
					parsedData.Proposal.State = model.ProposalStateName[associatedProposalRecord.State]
				}

				var associatedApplicantRecord model.User
				err = db.Model(&model.User{}).Where("wallet = ?", common.FormatUserWallet(parsedData.Applicant)).First(&associatedApplicantRecord).Error
				if err != nil {
					log.Error().Msgf("query associated applicant %s from DB error: %+v", parsedData.Applicant, err)
				} else {
					parsedData.ApplicantAvatar = associatedApplicantRecord.Avatar
				}

				dataBytes, err := json.Marshal(parsedData)
				if err != nil {
					log.Error().Msgf("marshal associate proposal data error: %+v", err)
				}

				item.Data = string(dataBytes)
			}
		}

		return &ComponentInstance{
			ID:            item.ID,
			ComponentId:   item.ComponentID,
			ComponentName: componentRecord.Name,
			Schema:        "",
			Data:          item.Data,
			CreateTs:      item.CreateTs,
		}
	})

	var proposal model.Proposal
	if err := db.Find(&proposal, proposalId).Error; err != nil {
		log.Error().Msgf("query proposal %d from DB error: %+v", proposalId, err)
		return nil, err, -1
	}

	var editHistoryRecords []*FrontendProposalEditHistoryRecord
	var frontendCommentsRecords []*FrontendProposalCommentRecord
	var votes []metaforo.PollRecord
	rejectedComment := model.ProposalComment{}
	commentCount := 0

	if proposal.ProposalRecordId != "" {
		metaforoProposal, err := metaforo.GetProposal(proposal.GetMetaforoThreadId(), metaforoGroupName, accessToken, startPostId)
		if err != nil {
			return nil, err, internal.ERRCODE_GetMetaforoDataError
		}

		commentCount = metaforoProposal.Thread.PostsCount
		votes = metaforoProposal.Thread.Polls

		err = s.UpdateDbRecordsFromMetaforoProposalResponse(db, proposalId, metaforoProposal)
		if err != nil {
			log.Error().Msgf("update proposal %d from metaforo error: %+v", proposalId, err)
			return nil, err, -1
		}

		pollStatusChanged, err := s.UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(db, proposalId, metaforoProposal)
		if err != nil {
			log.Error().Msgf("update propsal vote option records with metaforo response error: %+v", err)
			return nil, err, -1
		} else {
			log.Debug().Msgf("poll of proposal %d status changed: %+v", proposalId, pollStatusChanged)
		}

		if pollStatusChanged {
			if err = s.HandleProposalPollStatusChange(db, proposalId, metaforoGroupName); err != nil {
				log.Error().Msgf("handle proposal poll status change error: %+v", err)
				return nil, err, -1
			}
		}

		err = db.Model(model.ProposalComment{}).Where("proposal_id = ? AND is_reject_comment = ?", proposalId, true).First(&rejectedComment).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error().Msgf("fetch rejected comment error: %+v", err)
			return nil, err, -1
		}

		editHistoryRecords, err = s.GetLocalEditHistoriesWithOsUserData(db, proposal.ProposalRecordId)
		if err != nil {
			log.Error().Msgf("fetch local history record error: %+v", err)
			return nil, err, -1
		}

		// Process comments
		frontendCommentsRecords, err = s.GetProposalCommentsWithOsUserData(db, metaforoProposal.Thread.Posts)
		if err != nil {
			log.Error().Msgf("fetch proposal comments error: %+v", err)
			return nil, err, -1
		}
	}

	// Fetch vote_gate info
	// TODO: duplicated code in vote check logic, use function to replace it.
	var proposalCategory *model.ProposalCategory
	err := db.Model(&model.ProposalCategory{}).
		Joins("ProposalVoteGate").
		Where(model.ProposalCategory{ID: proposal.ProposalCategoryID}).First(&proposalCategory).Error
	if err != nil {
		log.Error().Msgf("fetch proposal category error: %+v", err)
		return nil, err, -1
	}

	var voteGate *FrontendVoteGateResponse
	if proposalCategory.ProposalVoteGate != nil {
		voteGate = &FrontendVoteGateResponse{
			ID:        proposalCategory.ProposalVoteGate.ID,
			Name:      proposalCategory.ProposalVoteGate.Name,
			TokenAddr: proposalCategory.ProposalVoteGate.TokenAddress,
			TokenId:   proposalCategory.ProposalVoteGate.TokenId,
			TokenType: proposalCategory.ProposalVoteGate.TokenTypeName(),
			ChainType: proposalCategory.ProposalVoteGate.ChainName(),
		}
	}

	templateName := ""
	if proposal.ProposalTemplateID != nil {
		var template model.ProposalTemplate
		err := db.Find(&template, *proposal.ProposalTemplateID).Error
		if err != nil {
			log.Error().Msgf("fetch proposal template error: %+v", err)
			return nil, err, -1
		}
		templateName = template.Name
	}

	proposalExecTs := int64(0)
	var proposalCronJobs []*model.CronJob
	if err = db.Where(&model.CronJob{ProposalId: proposalId}).Find(&proposalCronJobs).Error; err != nil {
		log.Error().Msgf("get proposal cronjob error: %+v", err)
	}

	var voteRecords []*model.ProposalVoteRecord
	err = db.Model(proposal).Association("VoteRecords").Find(&voteRecords)
	if err != nil {
		log.Error().Msgf("get vote records error: %+v", err)
		return nil, err, -1
	}

	for _, job := range proposalCronJobs {
		if job.NextExecTs > proposalExecTs {
			proposalExecTs = job.NextExecTs
		}
	}

	var osVoteOptionRecords []*model.ProposalVoteOptionRecord
	err = db.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_id = ?", proposalId).Find(&osVoteOptionRecords).Error
	if err != nil {
		log.Error().Msgf("get vote option records error: %+v", err)
		return nil, err, -1
	}

	frontendVoteOptions := lo.Map(osVoteOptionRecords, func(r *model.ProposalVoteOptionRecord, _ int) *FrontendProposalVoteOptionRecord {
		return &FrontendProposalVoteOptionRecord{
			ID:         r.ID,
			Label:      r.Text,
			MetaforoId: r.MetaforoID,
		}
	})

	// Check whether the proposal has associated project
	budgetsResponse := make([]*project.ProjectBudgetResp, 0)
	if proposal.Sip != 0 {
		var associatedProject *model.Project
		err = db.Where(&model.Project{SIP: fmt.Sprintf("%d", proposal.Sip)}).First(&associatedProject).Error
		if err == nil {
			budgetRecords, err := model.ProjectBudgetModel.ListByProjectId(db, associatedProject.ID)
			if err != nil {
				log.Error().Msgf("fetch project budget error: %+v", err)
			} else {
				budgetsResponse = project.GenerateProjectBudgetResp(budgetRecords)
			}
		}
	} else {
		// Proposal has sip = 0 means it is a saved but not submitted proposal, verify whether it is close project proposal
		proposalIsForClosingProject, closingProject, err := s.IsProposalIsForClosingProject(db, proposal.ID)
		if err != nil {
			log.Error().Msgf("checking proposal is for closing project failed, err: %+v", err)
			return nil, err, -1
		}

		if proposalIsForClosingProject {
			budgetRecords, err := model.ProjectBudgetModel.ListByProjectId(db, closingProject.ID)
			if err != nil {
				log.Error().Msgf("fetch project budget error: %+v", err)
			} else {
				budgetsResponse = project.GenerateProjectBudgetResp(budgetRecords)
			}
		} else {
			// This is not a close project proposal, no budget records
		}

	}

	// Refresh proposal record
	if err = db.Find(&proposal, proposalId).Error; err != nil {
		log.Error().Msgf("fetch proposal error: %+v", err)
		return nil, err, -1
	}

	proposalMultipleVoteFlag := proposal.IsMultipleVote
	if proposal.ProposalRecordId != "" {
		proposalMultipleVoteFlag = len(votes) > 0 && votes[0].Max > 1
	}

	return &FrontendProposalDetailRecord{
		ID:                      proposalId,
		Title:                   proposal.Title,
		ContentBlocks:           proposalContentResponse,
		ProposalCategoryId:      proposal.ProposalCategoryID,
		State:                   model.ProposalStateName[proposal.State],
		Components:              proposalComponentResponse,
		Applicant:               proposal.Applicant,
		ApplicantAvatar:         db_agent.GetUserAvatar(proposal.Applicant),
		IsRejected:              proposal.State == int(model.ProposalStateRejected),
		RejectReason:            rejectedComment.Content,
		RejectTs:                rejectedComment.CreateTs,
		RejectMetaforoCommentId: rejectedComment.MetaforoCommentId,
		Histories: &FrontendProposalEditHistories{
			TotalCount: len(editHistoryRecords),
			Lists:      editHistoryRecords,
		},
		Sip:                      proposal.Sip,
		Arweave:                  proposal.ArweaveHash,
		CommentCount:             commentCount,
		Comments:                 frontendCommentsRecords,
		VoteGate:                 voteGate,
		Votes:                    votes,
		OsVoteOptions:            frontendVoteOptions,
		VoteType:                 proposal.VoteType,
		IsMultipleVote:           proposalMultipleVoteFlag,
		CreateTs:                 proposal.CreateTs,
		IsBasedOnCustomTemplate:  proposal.IsBasedOnCustomTemplate,
		TemplateName:             templateName,
		IsInstantExecution:       proposal.PendingExecutionSecond == 0,
		ExecutionTs:              proposalExecTs,
		PublicityTs:              proposal.VoteStartTs,
		AssociatedProjectBudgets: budgetsResponse,
	}, nil, 0
}

func (s *ProposalService) GetMetaforoProposalByInternalId(db *gorm.DB, proposalIdStr string, metaforoGroupName string, mfAccessToken string) (*model.Proposal, *metaforo.ProposalResponse, error) {
	osProposalRcd, err := s.GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		log.Error().Msgf("get db proposal id %s error: %+v", proposalIdStr, err)
		return nil, nil, err
	}

	metaforoProposalResponse, err := metaforo.GetProposal(osProposalRcd.GetMetaforoThreadId(), metaforoGroupName, mfAccessToken, 0)
	if err != nil {
		log.Error().Msgf("get metaforo proposal error: %+v", err)
		return nil, nil, err
	}

	err = s.UpdateDbRecordsFromMetaforoProposalResponse(db, osProposalRcd.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update db records from metaforoProposalResponse error: %+v", err)
	}

	return osProposalRcd, metaforoProposalResponse, nil
}

func (s *ProposalService) GetLocalEditHistoriesWithOsUserData(db *gorm.DB, proposalRecordId string) ([]*FrontendProposalEditHistoryRecord, error) {
	// Get proposal records with same RecordId
	var localHistoryRecords []*FrontendProposalEditHistoryRecord
	querySql := QueryProposalWithJointUserBaseSQL + " WHERE proposal_record_id = ? ORDER BY create_ts desc"
	err := db.Raw(querySql, proposalRecordId).Find(&localHistoryRecords).Error
	if err != nil {
		log.Error().Msgf("fetch history proposal record error: %+v", err)
		return nil, err
	}

	return localHistoryRecords, nil
}

func (s *ProposalService) GetOsUserFromMetaforoUserId(db *gorm.DB, metaforoUserIds []int) ([]*JointMetaforoAndOsUser, error) {
	var records []*JointMetaforoAndOsUser
	err := db.Raw(QueryMetaforoUserWithOsUserBaseSQL+" WHERE mu.metaforo_user_id in ?", metaforoUserIds).Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (s *ProposalService) GetProposalCommentsWithOsUserData(db *gorm.DB, metaforoComments []metaforo.PostData) ([]*FrontendProposalCommentRecord, error) {
	var err error
	var frontendCommentsRecords []*FrontendProposalCommentRecord

	for _, metaforoComment := range metaforoComments {
		userWallet := ""
		if len(metaforoComment.User.Web3PublicKeys) > 0 {
			userWallet = common.FormatUserWallet(metaforoComment.User.Web3PublicKeys[0].Address)
		}

		proposalTitle := ""
		proposalTs := int64(0)
		proposalArweaveHash := ""
		dbComment := model.ProposalComment{MetaforoCommentId: metaforoComment.Id}
		err = db.Model(model.ProposalComment{}).Joins("Proposal").Where(dbComment).First(&dbComment).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Warn().Msgf("porposal comment with metaforo id %d not found, metaforo resposne: %+v", metaforoComment.Id, metaforoComment)
			} else {
				return nil, err
			}
		} else {
			proposalTitle = dbComment.Proposal.Title
			proposalTs = dbComment.Proposal.CreateTs
			proposalArweaveHash = dbComment.Proposal.ArweaveHash
		}

		userRecords, err := s.GetOsUserFromMetaforoUserId(db, []int{metaforoComment.UserId})
		if err != nil {
			log.Error().Msgf("get user info by wallet %s error: %+v", dbComment.AuthorWallet, err)
			return nil, err
		}

		userAvatar := ""
		if len(userRecords) > 0 {
			userAvatar = userRecords[0].OsAvatar
		}

		var childrenRecords []*FrontendProposalCommentRecord
		if metaforoComment.ChildrenCount > 0 {
			childrenRecords, err = s.GetProposalCommentsWithOsUserData(db, metaforoComment.Children.Posts)
			if err != nil {
				log.Error().Msgf("convert children comments error: %+v", err)
				return nil, err
			}
		}

		var mfContent string
		switch reflect.TypeOf(metaforoComment.Content).Kind() {
		case reflect.Float64:
			mfContent = strconv.FormatFloat(metaforoComment.Content.(float64), 'f', -1, 64)
		default:
			mfContent = metaforoComment.Content.(string)
		}

		frontendCommentsRecords = append(frontendCommentsRecords, &FrontendProposalCommentRecord{
			MetaforoPostId:      metaforoComment.Id,
			Content:             mfContent,
			Wallet:              userWallet,
			Avatar:              userAvatar,
			ReplyMetaforoPostId: metaforoComment.ReplyPid,
			Deleted:             metaforoComment.DeletedBy != nil,
			Children:            childrenRecords,
			ProposalTitle:       proposalTitle,
			ProposalTs:          proposalTs,
			ProposalArweaveHash: proposalArweaveHash,
			CreatedTs:           metaforoComment.CreatedAt.UTC().Unix(),
			IsRejected:          dbComment.IsRejectComment,
		})
	}

	return frontendCommentsRecords, nil
}

// fetchUserWalletFromMetaforoIds populates user wallet from metaforo user ids
// The function first check whether the user wallet is empty, if so it will try to get the user detail from metaforo API then save the wallet to db
// It will also create user record in db if not exist
func (s *ProposalService) FetchUserWalletFromMetaforoIds(db *gorm.DB, metaforoUserIds []int) (map[int]string, error) {
	// Verify the user are new record which haven't been created in our db
	var missingWalletMetaforoUser []*model.MetaforoUser
	err := db.Model(&model.MetaforoUser{}).Where("user_wallet = ?", "").Find(&missingWalletMetaforoUser).Error
	if err != nil {
		log.Error().Msgf("get missing wallet metaforo user error: %+v", err)
		return nil, err
	}

	mfUserIdProfile := make(map[int]*metaforo.UserDetailResponseForProfileAPI)
	for _, mfUser := range missingWalletMetaforoUser {
		mfUserData, err := metaforo.UserDetail(mfUser.MetaforoUserId)
		if err != nil {
			log.Error().Msgf("get metaforo user detail error: %+v", err)
			return nil, err
		}

		mfUserIdProfile[mfUserData.User.Id] = mfUserData
		time.Sleep(time.Millisecond * 100)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for userId, profileData := range mfUserIdProfile {
			metaforoUser := model.MetaforoUser{
				MetaforoUserId: userId,
				UserWallet:     common.FormatUserWallet(profileData.User.Web3PublicKey),
			}
			mfUserTx := tx.Where(&model.MetaforoUser{MetaforoUserId: userId}).Find(&metaforoUser)

			if mfUserTx.Error != nil {
				log.Error().Msgf("get metaforo user record error: %+v", mfUserTx.Error)
				return mfUserTx.Error
			} else if mfUserTx.RowsAffected == 0 {
				if err = tx.Create(&metaforoUser).Error; err != nil {
					log.Error().Msgf("create metaforo user record error: %+v", err)
					return err
				}
			} else if mfUserTx.RowsAffected == 1 {
				if err = tx.Updates(&metaforoUser).Error; err != nil {
					log.Error().Msgf("update metaforo user record error: %+v", err)
					return err
				}
			} else {
				err = fmt.Errorf("unexpected rows affected: %d", mfUserTx.RowsAffected)
				log.Error().Msgf(err.Error())
				return err
			}

			// Create user record if not existing
			userRecord := model.User{Wallet: metaforoUser.UserWallet}
			db.Clauses(clause.OnConflict{DoNothing: true}).Model(&model.User{}).Where(&userRecord).Assign(model.User{
				CreateTs: model.GetCurrentUtcEpochSecond(),
				UpdateTs: model.GetCurrentUtcEpochSecond(),
				Avatar:   profileData.User.PhotoUrl,
				Name:     profileData.User.Username,
			}).FirstOrCreate(&userRecord)
		}
		return nil
	})

	userIdWalletMap := make(map[int]string)
	mfUserRecords := make([]*model.MetaforoUser, 0)
	err = db.Model(&model.MetaforoUser{}).Where("metaforo_user_id in ?", metaforoUserIds).Find(&mfUserRecords).Error
	if err != nil {
		log.Error().Msgf("get metaforo user records error: %+v", err)
		return nil, err
	}

	for _, mfUser := range mfUserRecords {
		userIdWalletMap[mfUser.MetaforoUserId] = mfUser.UserWallet
	}

	return userIdWalletMap, nil
}

// UpdateProposalStateAndLaunchStateChangeActions changes proposal state and launch specified actions associated with state change
// This function is invoked in directly API calls, like approve, withdrawn, etc. and proposal state change automation tasks
// The state change actions contains:
// - Withdrawn: Set vote start time to 1 yr later after withdrawn
// - Approved: Set proposal sip, create project for p1 create project proposal (no vote and 0 pending execution time), or start vote
// - Rejected: Set vote start time to 1 yr later after rejected
// - PendingExecution: None
// - Executed: Create project if it is new project proposal
func (s *ProposalService) UpdateProposalStateAndLaunchStateChangeActions(db *gorm.DB, user *middleware.CurUser, proposalStrId string, newState model.ProposalState, cfg *config.Config) (uint, error) {
	proposalId, err := strconv.Atoi(proposalStrId)
	if err != nil {
		log.Error().Msgf("parse proposal ID %s to int error: %+v", proposalStrId, err)
		return 0, err
	}

	if !s.TryAcquireUpdateProposalDbLockOrReturn(uint(proposalId)) {
		err := fmt.Errorf("proposal %s is updating", proposalStrId)
		log.Error().Msg(err.Error())
		return 0, err
	}
	defer s.ReleaseUpdateProposalDbLock(uint(proposalId))

	log.Debug().Msgf("enter UpdateProposalStateAndLaunchStateChangeActions, proposalStrId: %s, newState: %s", proposalStrId, model.ProposalStateName[newState])
	proposalRecord, err := s.GetProposalFromStringId(db, proposalStrId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalStrId)
		}
		return 0, err
	}

	var pTemplate *model.ProposalTemplate
	if err = db.Model(&proposalRecord).Association("ProposalTemplate").Find(&pTemplate); err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return 0, err
	}

	// Login to admin account to avoid token expiration
	//metaforo.Login(cfg.Metaforo.GroupName, cfg.Metaforo.AdminWallet, cfg.Metaforo.Sign, cfg.Metaforo.SignMsg, cfg.Metaforo.WalletType)
	s.RefreshMetaforoAdminToken()

	// Check whether user has permission to the change the proposal state
	switch newState {
	case model.ProposalStateWithdrawn:
		if user == nil || !strings.EqualFold(user.Wallet, proposalRecord.Applicant) {
			return 0, errors.New("proposal can only be withdrawn by applicant")
		}

		var voteRecords []*model.ProposalVoteRecord
		err = db.Model(proposalRecord).Association("VoteRecords").Find(&voteRecords)
		if err != nil {
			log.Error().Msgf("get vote records error: %+v", err)
			return 0, err
		}

		if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			log.Debug().Msgf("proposal %d has no vote records, clear cronjob created for updating state", proposalId)
			if err = db.Model(&model.CronJob{}).Where(&model.CronJob{ProposalId: proposalRecord.ID}).Delete(&model.CronJob{}).Error; err != nil {
				log.Error().Msgf("delete cronjob error while withdrawing proposal: %+v", err)
				return 0, err
			}
		}

		for _, record := range voteRecords {
			err := metaforo.UpdateVoteTime(cfg.MetaforoData.AccessToken,
				cfg.MetaforoData.GroupName,
				record.MetaforoID,
				time.Now().UTC().Add(oneYearDuration).Unix(), // Set the start time 1 minute in advanced
				time.Now().UTC().Add(oneYearDuration+proposalRecord.VoteDuration()).Unix(),
			)
			if err != nil {
				log.Error().Msgf("update vote information error: %+v", err)
				return 0, err
			}
		}

		err = db.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", model.ProposalStateWithdrawn).Error
		if err != nil {
			log.Error().Msgf("change proposal to withdrawn error")
			return 0, err
		}
	case model.ProposalStateApproved:
		return 0, db.Transaction(func(tx *gorm.DB) error {
			if err = s.SetProposalSip(db, pTemplate, proposalRecord); err != nil {
				log.Error().Msgf("set proposal sip error: %+v", err)
				return err
			}

			var voteRecords []*model.ProposalVoteRecord
			err = tx.Model(&proposalRecord).Association("VoteRecords").Find(&voteRecords)
			if err != nil {
				log.Error().Msgf("get vote records error: %+v", err)
				return err
			}

			if proposalRecord.VoteType == model.ProposalVoteTypeNone {
				if proposalRecord.PendingExecutionSecond == 0 {
					proposalRecord.State = int(model.ProposalStateExecuted)

					// Additional tasks for executed proposal
					if pTemplate.Type == model.ProposalTemplateTypeNewProject {
						// Get create project params from proposal components
						prjRecord, err := s.CreateProjectFromAutoTasks(tx, proposalRecord.ID)
						if err != nil {
							log.Error().Msgf("create project error: %+v", err)
							return err
						}
						api.PrintStructAsJson(prjRecord, "TTT: Create Project After approving no vote and no pending execution proposal")
					}
				} else {
					// TODO: this code branch is missing create job to execute task if required, but for now, only one type of no vote proposal, so this code branch should be a dead code.
					// Create a cronjob mark the proposal to executed directly after pending execution time
					log.Error().Msgf("NOTICE: this code branch shouldn't be hit, since there is only one type of novote type proposal, if you see this log, check db data or requirements")
					proposalRecord.State = int(model.ProposalStatePendingExecution)
					// Delete cronjob created while creating for updating proposal state to approved
					if err = tx.Model(&model.CronJob{}).Delete(&model.CronJob{}, &model.CronJob{
						ProposalId:  proposalRecord.ID,
						HandlerName: internal.TaskUpdateProposalState,
					}).Error; err != nil {
						log.Error().Msgf("create proposal state change error: %+v", err)
						return err
					}

					if err = tx.Model(&model.ProposalComponentRecord{}).
						Delete(&model.ProposalComponentRecord{}, map[string]any{"proposal_id": proposalRecord.ID, "component_id": 0}).Error; err != nil {
						log.Error().Msgf("create proposal state change error: %+v", err)
						return err
					}

					if err = s.CreateJobToUpdateNoVoteProposalToNextState(tx, proposalRecord.ID, proposalRecord.VoteType, model.GetCurrentUtcEpochSecond()+proposalRecord.PendingExecutionSecond, model.ProposalStateExecuted); err != nil {
						log.Error().Msgf("create proposal state change error: %+v", err)
						return err
					}
				}
			} else {
				for _, record := range voteRecords {
					err := metaforo.UpdateVoteTime(cfg.MetaforoData.AccessToken,
						cfg.MetaforoData.GroupName,
						record.MetaforoID,
						time.Now().UTC().Add(-1*time.Minute).Unix(), // Set the start time 1 minute in advanced
						time.Now().UTC().Add(proposalRecord.VoteDuration()).Unix(),
					)
					if err != nil {
						log.Error().Msgf("update vote information error: %+v", err)
						break
					}
				}
				// Only update proposal to voting state when no error returns
				if err == nil {
					log.Debug().Msgf("update proposal state to voting")
					proposalRecord.State = int(model.ProposalStateVoting)
				}
			}
			err = tx.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", proposalRecord.State).Error
			if err != nil {
				log.Error().Msgf("change proposal to approved error")
				return err
			}
			return nil
		})
	case model.ProposalStateRejected:
		// Update vote to 1 yr later
		var voteRecords []*model.ProposalVoteRecord
		err = db.Model(proposalRecord).Association("VoteRecords").Find(&voteRecords)
		if err != nil {
			log.Error().Msgf("get vote records error: %+v", err)
			return 0, err
		}

		if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			log.Debug().Msgf("proposal %d has no vote records, clear cronjob created for updating state", proposalId)
			if err = db.Model(&model.CronJob{}).Where(&model.CronJob{ProposalId: proposalRecord.ID}).Delete(&model.CronJob{}).Error; err != nil {
				log.Error().Msgf("delete cronjob error while rejecting proposal: %+v", err)
				return 0, err
			}
		}

		for _, record := range voteRecords {
			err := metaforo.UpdateVoteTime(cfg.MetaforoData.AccessToken,
				cfg.MetaforoData.GroupName,
				record.MetaforoID,
				time.Now().UTC().Add(oneYearDuration).Unix(), // Set the start time 1 minute in advanced
				time.Now().UTC().Add(oneYearDuration+proposalRecord.VoteDuration()).Unix(),
			)
			if err != nil {
				log.Error().Msgf("update vote information error: %+v", err)
				return 0, err
			}
		}

		err = db.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", model.ProposalStateRejected).Error
		if err != nil {
			log.Error().Msgf("change proposal to rejected error")
			return 0, err
		}
	case model.ProposalStatePendingExecution:
		if err = db.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", model.ProposalStatePendingExecution).Error; err != nil {
			log.Error().Msgf("update proposal %d state to pending execution error", proposalRecord.ID)
			return 0, err
		}
	case model.ProposalStateExecuted:
		// Additional tasks for executed proposal
		if pTemplate.Type == model.ProposalTemplateTypeNewProject {
			// Get create project params from proposal components
			prjRecord, err := s.CreateProjectFromAutoTasks(db, proposalRecord.ID)
			if err != nil {
				log.Error().Msgf("create project error: %+v", err)
				return 0, err
			}
			api.PrintStructAsJson(prjRecord, "TTT: Create Project After approving no vote and no pending execution proposal")
		}

		err = db.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", model.ProposalStateExecuted).Error
		if err != nil {
			log.Error().Msgf("change proposal to executed error")
			return 0, err
		}
	default:
		return 0, fmt.Errorf("changing proposal from state %s to %s is not approved", proposalRecord.StateName(), model.ProposalStateName[newState])
	}
	return proposalRecord.ID, nil
}

func (s *ProposalService) GenerateFrontendProposalRecords(db *gorm.DB, querySql string, page *gormfind.Page, listBySip bool) (int64, []*FrontendProposalListRecord, error) {
	var tmpRcd []*FrontendProposalListRecord

	countTx := db.Raw(querySql).Scan(&tmpRcd)
	if err := countTx.Error; err != nil {
		log.Error().Msgf("get proposal count error: %+v", err)
		return 0, nil, err
	}
	total := countTx.RowsAffected

	if page != nil {
		// Specify custom order by state
		// Note: this is PG specified function
		if listBySip {
			querySql += "\nORDER BY sip desc, create_ts desc"
		} else {
			querySql += fmt.Sprintf("\nORDER BY array_position(array[%s], p.state), create_ts desc",
				strings.Join(lo.Map(StateOrder, func(state model.ProposalState, _ int) string { return fmt.Sprintf("%d", state) }), ", "))
		}
		querySql += fmt.Sprintf("\nLIMIT %d OFFSET %d", page.Size, (page.Page-1)*page.Size)
	}

	var resultRows []*FrontendProposalListRecord
	err := db.Raw(querySql).Find(&resultRows).Error
	if err != nil {
		log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", querySql, err)
		return 0, nil, err
	}

	resultRows = lo.Map(resultRows, func(r *FrontendProposalListRecord, _ int) *FrontendProposalListRecord {
		dup_r := r
		dup_r.State = model.ProposalStateName[r.StateId]
		return dup_r
	})

	return total, resultRows, nil
}

// TODO: Temporary solution to get open project proposal info while creating close project proposal

func (s *ProposalService) CreateJobToUpdateNoVoteProposalToNextState(db *gorm.DB, proposalId uint, proposalVoteType int, jobExecTs int64, nextState model.ProposalState) error {
	proposalComponentRecord := model.ProposalComponentRecord{
		ProposalID:  proposalId,
		ComponentID: 0,
	}

	if err := db.Model(&proposalComponentRecord).
		Where(map[string]any{"proposal_id": proposalId, "component_id": 0}). // Note: 0 won't be passed to query if using struct data
		First(&proposalComponentRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			db.Create(&proposalComponentRecord)
		} else {
			log.Error().Msgf("create proposal component record error: %+v", err)
			return err
		}
	} else {
		if proposalVoteType != model.ProposalVoteTypeNone {
			log.Debug().Msgf("automation for updating state has already created, return")
			return nil
		}
	}

	updateProposalStateTaskParams := map[string]any{
		"proposal_id": proposalId,
		"state":       int(nextState),
	}

	jobParamsStr, err := json.Marshal(updateProposalStateTaskParams)
	if err != nil {
		log.Error().Msgf("marshal update proposal state params error: %+v", err)
		return err
	}

	currentTs := model.GetCurrentUtcEpochSecond()

	finTask := &model.CronJob{
		CreateTs:                  currentTs,
		UpdateTs:                  currentTs,
		HandlerName:               internal.TaskUpdateProposalState,
		ProposalComponentRecordId: int(proposalComponentRecord.ID),
		ProposalId:                proposalId,
		LastExecTs:                0,
		NextExecTs:                jobExecTs,
		JobParams:                 string(jobParamsStr),
		VoteResult:                "",
		VoteType:                  proposalVoteType,
		State:                     model.CronJobStateActive,
		LastExecResult:            "",
	}
	createTaskTx := db.Where(model.CronJob{
		HandlerName:               internal.TaskUpdateProposalState,
		ProposalComponentRecordId: int(proposalComponentRecord.ID),
		ProposalId:                proposalId,
		NextExecTs:                jobExecTs,
	}).
		Assign(&finTask).FirstOrCreate(&finTask)

	if createTaskTx.Error != nil {
		log.Error().Msgf("create proposal fin task error: %+v", createTaskTx.Error)
		return createTaskTx.Error
	} else if createTaskTx.RowsAffected == 0 {
		log.Warn().Msgf("proposal fin task already exists: %+v", finTask)
	}

	return nil
}

func (s *ProposalService) GetProposalTemplateType(db *gorm.DB, templateId uint) (model.ProposalTemplateType, error) {
	var pTemplate model.ProposalTemplate
	err := db.Find(&pTemplate, templateId).Error
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return 0, err
	}
	return pTemplate.Type, nil
}

func (s *ProposalService) FindProjectCreatedByProposal(db *gorm.DB, proposalId uint) (*model.Project, error) {
	log.Debug().Msgf("find project created by proposal: %d", proposalId)
	var createProjectProposal model.Proposal
	if err := db.Find(&createProjectProposal, proposalId).Error; err != nil {
		log.Error().Msgf("get create project proposal error: %+v", err)
		return nil, err
	}

	createdProject := model.Project{
		SIP: fmt.Sprintf("%d", createProjectProposal.Sip),
	}
	if err := db.Clauses(clause.Locking{
		Strength: "UPDATE",
		Options:  "NOWAIT",
	}).Model(&createdProject).Where("s_ip = ?", createdProject.SIP).First(&createdProject).Error; err != nil {
		log.Error().Msgf("get create project proposal error: %+v", err)
		return nil, err
	}

	return &createdProject, nil
}

func (s *ProposalService) UpdateUserVoteRecordViaMetaforo(db *gorm.DB, mfGroupName string, proposalId uint) error {
	proposal, err := service.ProposalService.GetProposalById(db, proposalId)
	if proposal == nil {
		err = fmt.Errorf("proposal %d not found", proposalId)
		log.Error().Msgf(err.Error())
		return err
	}

	if proposal.UserVoteRecordSaved {
		log.Debug().Msgf("user vote record already saved for proposal %d", proposalId)
		return nil
	}

	// User vote record not saved, need to get all vote options for this proposal
	var voteOptions []model.ProposalVoteOptionRecord
	err = db.Where(&model.ProposalVoteOptionRecord{ProposalId: proposalId}).Find(&voteOptions).Error
	if err != nil {
		log.Error().Msgf("get proposal vote option record error: %+v", err)
		return err
	}

	// Get voter lister from metaforo vote option, the voter list returned from metaforo API only contains user id
	var voterList []*metaforo.UserPollRecord
	for _, voteOption := range voteOptions {
		// A magic number 100 is used here to get all voters for this vote option, which SHOULD NOT be reached
		for i := 1; i <= 100; i++ {
			pagedVoterList, err := metaforo.GetVoterList(mfGroupName, voteOption.MetaforoID, i)
			if err != nil {
				if strings.Contains(err.Error(), internal.MetaforoPollNotExistsPrompt) {
					// Vote option not found in metaforo, the original proposal may be deleted, break the loop and return
					db.Model(&model.Proposal{}).Where("id = ?", proposalId).Update("user_vote_record_saved", true)
					log.Warn().Msgf("vote option %d not found in metaforo, mark proposal %d as updated", voteOption.MetaforoID, proposalId)
					return nil
				} else {
					log.Error().Msgf("get voter list error: %+v", err)
					return err
				}
			}
			voterList = append(voterList, pagedVoterList...)
			if len(pagedVoterList) < 10 {
				// No more voters for this vote option, break the loop
				break
			}
			time.Sleep(time.Millisecond * 100)
		}
	}

	userIdWalletMap, err := s.FetchUserWalletFromMetaforoIds(db, lo.Map(voterList, func(voter *metaforo.UserPollRecord, _ int) int {
		return voter.User.Id
	}))
	if err != nil {
		log.Error().Msgf("populate user wallet from metaforo ids error: %+v", err)
		return err
	}

	// map for saving relationship between metaforo option id and internal proposal vote option record
	voterMfOptionIdRecordMap := lo.Associate(voteOptions, func(voteOption model.ProposalVoteOptionRecord) (int, *model.ProposalVoteOptionRecord) {
		return voteOption.MetaforoID, &voteOption
	})

	dbProposal, mfProposal, err := s.GetMetaforoProposalByInternalId(db, fmt.Sprintf("%d", proposalId), mfGroupName, "")
	if err != nil {
		log.Error().Msgf("get metaforo proposal by internal id error: %+v", err)
		return err
	}
	if dbProposal == nil {
		log.Error().Msgf("get metaforo proposal by internal id error: %+v", err)
		return err
	}

	pollEnded := true
	for _, poll := range mfProposal.Thread.Polls {
		if poll.CloseAt.After(time.Now()) {
			pollEnded = false
			break
		}
	}

	// Upsert ProposalUserVoteRecord record
	return db.Transaction(func(tx *gorm.DB) error {
		for _, voterInfo := range voterList {
			userWallet := userIdWalletMap[voterInfo.User.Id]
			internalVoteOptionRecord := voterMfOptionIdRecordMap[voterInfo.PollOptionId]

			err = model.UpsertUserVoteRecord(tx, &model.ProposalUserVoteRecord{
				ProposalID:                 proposalId,
				UserWallet:                 userWallet,
				ProposalVoteOptionRecordId: internalVoteOptionRecord.ID,
				VoteTs:                     model.GetCurrentUtcEpochSecond(),
			})
			if err != nil {
				log.Error().Msgf("create proposal user vote record error: %+v", err)
				return err
			}
		}
		if pollEnded {
			// Update proposal record to mark user vote record as saved only for finished proposal
			return tx.Model(&model.Proposal{}).Where(&model.Proposal{ID: proposalId}).Update("user_vote_record_saved", true).Error
		} else {
			return tx.Model(&model.Proposal{}).Where(&model.Proposal{ID: proposalId}).Update("user_vote_record_saved", false).Error
		}
	})
}

// UpdateTemplate requires to be invoked with Admin perm
func (s *ProposalService) UpdateTemplate(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	reqData := updateTmplRequest{}
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	newTmplRcd := model.ProposalTemplate{
		Name:               reqData.Name,
		ContentSchema:      reqData.Schema,
		ScreenshotUri:      reqData.ScreenshotUri,
		ProposalCategoryID: reqData.CategoryId,
	}

	var components []*model.ProposalComponent
	for _, compName := range reqData.Components {
		compRcd := model.ProposalComponent{Name: compName}
		if err := db.Model(&compRcd).Where("name = ?", compName).First(&compRcd).Error; err != nil {
			log.Error().Msgf("parse request data error: %+v", err)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
			return
		}

		components = append(components, &compRcd)
	}

	newTmplRcd.Components = components

	var tmplRcd model.ProposalTemplate
	if err := db.Model(&tmplRcd).Where("name = ?", reqData.Name).First(&tmplRcd).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = db.Model(&newTmplRcd).Create(&newTmplRcd).Error
			if err != nil {
				log.Error().Msgf("create proposal template error: %+v", err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
				return
			}
		} else {
			log.Error().Msgf("create proposal template error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	} else {
		// Update existing data
		newTmplRcd.ID = tmplRcd.ID
		// Clear existing m2m components
		err := db.Model(&tmplRcd).Association("Components").Clear()
		if err != nil {
			log.Error().Msgf("update proposal template error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
		err = db.Updates(&newTmplRcd).Error
		if err != nil {
			log.Error().Msgf("update proposal template error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	ctx.JSON(200, api.Success(newTmplRcd))
}

func (s *ProposalService) GetTemplateComponents(tmplDbRcd *model.ProposalTemplate) []*ComponentResponse {
	compNameMapping := lo.SliceToMap(tmplDbRcd.Components, func(c *model.ProposalComponent) (string, *model.ProposalComponent) {
		return c.Name, c
	})
	var components []*ComponentResponse
	if tmplDbRcd.ComponentNameList != nil {
		for _, name := range tmplDbRcd.ComponentNameList {
			if comp, ok := compNameMapping[name]; ok {
				components = append(components, &ComponentResponse{
					ID:            comp.ID,
					Name:          comp.Name,
					Schema:        comp.Schema,
					ScreenshotUri: comp.Schema,
				})
			}
		}
	} else {
		components = lo.Map(tmplDbRcd.Components, func(c *model.ProposalComponent, _ int) *ComponentResponse {
			return &ComponentResponse{
				ID:            c.ID,
				Name:          c.Name,
				Schema:        c.Schema,
				ScreenshotUri: c.ScreenshotUri,
			}
		})
	}

	return components
}

func (s *ProposalService) CanUserVoteOnThread(db *gorm.DB, userWallet string, proposalIdString string) (bool, error) {
	proposal, err := s.GetProposalFromStringId(db, proposalIdString)
	if err != nil {
		log.Error().Msgf("get proposal error: %+v", err)
		return false, err
	}
	log.Debug().Msgf("check voting permission of %s for proposal: %+v", userWallet, proposal)

	// Verify NFT gate
	seepassData, err := api.GetCachedSeepassData(sdk.GetSppClient(), userWallet, false)
	if err != nil {
		log.Error().Msgf("get seepass data error: %+v", err)
		return false, err
	}

	var voteGates []*model.ProposalVoteGate
	pTmplDbRcd := model.ProposalTemplate{
		ID: *proposal.ProposalTemplateID,
	}

	err = db.Model(&pTmplDbRcd).Association("VoteGates").Find(&voteGates)
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return false, err
	}

	permArray := lo.Map(voteGates, func(r *model.ProposalVoteGate, _ int) bool {
		return s.IsUserMetVoteGate(seepassData, r)
	})
	log.Debug().Msgf("voting perm array of %s for proposal: %d is %+v", userWallet, proposal.ID, permArray)

	permResult := lo.Reduce(permArray, func(rslt bool, r bool, _ int) bool {
		return rslt && r
	}, true)
	log.Debug().Msgf("voting perm result of %s for proposal: %d is %+v", userWallet, proposal.ID, permResult)

	return permResult, nil
}
