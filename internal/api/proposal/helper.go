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
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
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

type projectBudgetData struct {
	Name        string `json:"name"`
	TotalAmount string `json:"total_amount"`
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

	log.Debug().Msgf("save proposal record to DB: %+v, proposalIdStr: %d, user wallet: %s", reqData, proposalId, userWallet)

	// Update title for testing
	if cfg.MetaforoData.ProposalPrefix != "" && !strings.HasPrefix(reqData.Title, cfg.MetaforoData.ProposalPrefix) {
		reqData.Title = cfg.MetaforoData.ProposalPrefix + reqData.Title
	}

	var voteTimeProps model.VoteTimeProperties

	var pCategory model.ProposalCategory
	err := db.Find(&pCategory, reqData.ProposalCategoryId).Error
	if err != nil {
		log.Error().Msgf("get proposal category error: %+v", err)
		return nil, err
	}

	var pTemplate model.ProposalTemplate
	err = db.Find(&pTemplate, reqData.TemplateId).Error
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return nil, err
	}

	voteTimeProps.PublicitySecond = pTemplate.PublicitySecond
	voteTimeProps.VoteDurationSecond = pTemplate.VoteDurationSecond
	voteTimeProps.PendingExecutionSecond = pTemplate.PendingExecutionSecond
	reqData.VoteType = pTemplate.VoteType

	// FIXME: data race: method 1: cache over DB, method 2: lock on record, method 3: upgrade SQL
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
		proposalRcd.ProposalCategoryID = reqData.ProposalCategoryId
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
			if err = tx.Model(&model.ProposalVoteRecord{}).Where("proposal_id = ?", proposalId).Update("proposal_id = ?", proposalId).Error; err != nil {
				log.Error().Msgf("move proposal vote record error: %+v", err)
				return err
			}

			if err = tx.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_id = ?", proposalId).Update("proposal_id = ?", proposalId).Error; err != nil {
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
			ProposalCategoryID:      reqData.ProposalCategoryId,
			Version:                 1,
			VoteType:                reqData.VoteType,
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

//func SaveProposalVoteOptionRecords(db *gorm.DB, proposalId uint, applicantWallet string, voteType int, voteOptions []*string) error {
//	switch voteType {
//	case model.ProposalVoteTypeNone:
//		return nil
//	case model.ProposalVoteTypeDecision:
//		proposalVoteRecord := model.ProposalVoteRecord{
//			Options: lo.Map(internal.ProposalDecisionVoteOptions, func(s []string, _ int) *model.ProposalVoteOptionRecord {
//				return &model.ProposalVoteOptionRecord{
//					ProposalId: proposalId,
//					Text:       s[0],
//					Value:      s[1],
//				}
//			}),
//			ProposalID: proposalId,
//			VoteType:   voteType,
//		}
//		if err := db.Model(&model.ProposalVoteRecord{}).Where(&proposalVoteRecord).First(&proposalVoteRecord).Error; err != nil {
//			if errors.Is(err, gorm.ErrRecordNotFound) {
//				db.Create(&proposalVoteRecord)
//			} else {
//			}
//		}
//		voteOptions = lo.Map(internal.ProposalDecisionVoteOptions, func(s []string, _ int) *string {
//			return &model.ProposalVoteOptionRecord{
//				ProposalVoteRecordId: 0,
//				ProposalId:           proposalId,
//				Text:                 "",
//				MetaforoID:           0,
//				MetaforoVoteID:       0,
//				Value:                "",
//				VoterCount:           0,
//			}
//		})
//	case model.ProposalVoteTypeNumericSingle:
//		return nil
//	case model.ProposalVoteTypeNumericAvg:
//		return nil
//	case model.ProposalVoteType:
//
//	}
//
//	var existingVoteOptionRecordsId []uint
//	err := db.Model(&model.ProposalVoteOptionRecord{}).
//		Where(model.ProposalVoteOptionRecord{ProposalVoteRecordId: proposalId}).
//		Pluck("id", &existingVoteOptionRecordsId).Error
//	if err != nil {
//		log.Error().Msgf("get proposal vote record ids error: %+v", err)
//		return err
//	}
//
//	return db.Transaction(func(tx *gorm.DB) error {
//		var updatedIds []uint
//		for _, componentData := range reqComponentData {
//			// Try to get component record from DB
//			componentRecord := model.ProposalComponent{
//				Name: componentData.Name,
//			}
//			if err := tx.Where(componentRecord).First(&componentRecord).Error; err != nil {
//				tx.Rollback()
//				log.Error().Msgf("find proposal component %s error: %+v", componentData.Name, err)
//				return err
//			}
//
//			// Append applicant information
//			componentData.Data["applicant"] = common.FormatUserWallet(applicantWallet)
//			componentData.Data["proposal_id"] = fmt.Sprintf("os-%d", proposalId)
//
//			proposalDataStr, err := json.Marshal(componentData.Data)
//			if err != nil {
//				tx.Rollback()
//				log.Error().Msgf("create proposal component error: %+v", err)
//				return err
//			}
//
//			// Create proposal component record and save to DB
//			if err := db.Save(&model.ProposalComponentRecord{
//				CreateTs:    time.Now().UTC().Unix(),
//				ComponentID: componentRecord.ID,
//				ProposalID:  proposalId,
//				Data:        string(proposalDataStr),
//			}).Error; err != nil {
//				log.Error().Msgf("create proposal component error: %+v", err)
//				return err
//			}
//
//			if componentData.ID != 0 {
//				updatedIds = append(updatedIds, componentData.ID)
//			}
//		}
//
//		// Remove deleted blocks
//		for _, componentId := range existingComponentIds {
//			if !lo.Contains(updatedIds, componentId) {
//				if err := db.Delete(&model.ProposalComponentRecord{ID: componentId}).Error; err != nil {
//					log.Error().Msgf("delete proposal component error: %+v", err)
//					return err
//				}
//			}
//		}
//		return nil
//	})
//}

// SaveProposalToMetaforo updates proposal record to Metaforo
// If proposal is in pending submit state, update existing proposal record (state, ProposalRecordId, ArweaveHash) and save back,
// the version is keep the same. The metaforo API invoked here is CreateProposal.
//
// Otherwise, copy the proposal to new record with ver+1, update the metaforo data, and save back as a new record,
// and the metaforo API invoked here is updateProposal.
// TODO: Refactor this function
func SaveProposalToMetaforo(db *gorm.DB, origProposalRecordId uint, voteType int, customVoteOptions []string, metaforoAccessToken string, EditorType int, metaforoGroupName string) error {
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
		voteRecords := []*model.ProposalVoteRecord{
			{
				GateID:   0,
				StartTs:  voteStartTime.Unix(),
				EndTs:    voteEndTime.Unix(),
				VoteType: updatedProposalRecord.VoteType,
			},
		}
		osVoteOptions := prepareOsVoteOptions(voteType, customVoteOptions)
		voteFormBytes, err := BuildMetaforoVoteFormDataBytes(voteRecords, osVoteOptions)
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
	metaforoProposalResponse, err = metaforo.GetProposal(metaforoThreadId, metaforoGroupName, "", 0)
	if err != nil {
		log.Error().Msgf("get metaforoProposal %d error: %+v", metaforoThreadId, err)
		return err
	}

	err = UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update db records from metaforoProposalResponse error: %+v", err)
	}

	// FIXME: Check whether the state can be updated by poll status changed
	updatedProposalRecord.State = int(model.ProposalStateDraft)
	updatedProposalRecord.ProposalRecordId = model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposalResponse.Thread.Id)

	// Save data backed from metaforo API response to DB
	if err := db.Model(&updatedProposalRecord).
		Where("id = ?", updatedProposalRecord.ID).
		Updates(&model.Proposal{
			ProposalRecordId: updatedProposalRecord.ProposalRecordId,
			State:            updatedProposalRecord.State,
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
func BuildMetaforoVoteFormDataBytes(voteRecords []*model.ProposalVoteRecord, osVoteOptions []string) ([]byte, error) {
	// Generate metaforo vote options
	voteOptions := lo.Map(osVoteOptions, func(r string, idx int) *metaforo.VoteOption {
		return &metaforo.VoteOption{Text: r, Type: idx}
	})

	voteData := lo.Map(voteRecords, func(r *model.ProposalVoteRecord, _ int) *metaforo.NewVoteFormRequest {
		return &metaforo.NewVoteFormRequest{
			Options:            voteOptions,
			Type:               "1",
			Title:              r.Title,
			ShowType:           "1",
			ShowResult:         true,
			ChartType:          "1",
			VoteType:           "1",
			ChainType:          0,
			ContractType:       0,
			SettingId:          0,
			Period:             "1",
			CloseAt:            time.Unix(r.EndTs, 0).Format(time.RFC3339),
			VoteStartAt:        time.Unix(r.StartTs, 0).Format(time.RFC3339),
			Max:                1,
			MinTokens:          "0",
			PollCategory:       "0",
			LastCategroyChange: "0",
			TokenId:            0,
			Quorum:             false,
			Weight:             true,
			Step:               2,
		}
	})

	voteDataBytes, err := json.Marshal(voteData)
	if err != nil {
		log.Error().Msgf("marshal proposal vote data error: %+v", err)
		return nil, err
	}
	return voteDataBytes, nil
}

// TODO: Change to use indexer data
func IsUserMetVoteGate(userSeepassData *sdk.SeepassResponse, proposalVoteGate *model.ProposalVoteGate) bool {
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
			return len(userSeepassData.Seed) > 1
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

	if err = UpdateUserRecordsFromMetaforoProposalResponse(db, dbProposalRcdId, metaforoProposal); err != nil {
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
			tx.Model(&dbProposals[idx]).Where("id = ?", dbProposals[idx].ID).Update("arweave_hash", metaforoProposal.Thread.EditHistory.Lists[idx].Arweave)
			if idx == 0 {
				// Save arwave hash data to record for setting it correctly in response
				tx.Model(&model.Proposal{}).Where(&model.Proposal{ID: dbProposalRcdId}).Updates(model.Proposal{ArweaveHash: metaforoProposal.Thread.EditHistory.Lists[idx].Arweave})
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

func UpdateUserRecordsFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcdId uint, metaforoProposal *metaforo.ProposalResponse) error {
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
		proposalVoteRecord := model.ProposalVoteRecord{MetaforoID: poll.Id}
		err = db.Where(&proposalVoteRecord).Assign(&model.ProposalVoteRecord{
			Title:      poll.Title,
			StartTs:    poll.PollStartAt.UTC().Unix(),
			EndTs:      poll.CloseAt.UTC().Unix(),
			ProposalID: dbProposalRcdId,
			VoteType:   dbProposalRcd.VoteType,
		}).FirstOrCreate(&proposalVoteRecord).Error
		if err != nil {
			log.Warn().Msgf("save DB proposal vote record error: %+v", err)
			return pollStatusChanged, err
		} else {
			log.Debug().Msgf("save DB proposal vote record success: %+v", proposalVoteRecord)
		}

		updatePollStateTx := db.Where(&model.ProposalVoteRecord{MetaforoID: poll.Id}).Update("state", poll.Status)
		if updatePollStateTx.Error != nil {
			log.Error().Msgf("update proposal vote record state error: %+v", updatePollStateTx.Error)
			return pollStatusChanged, updatePollStateTx.Error
		} else if updatePollStateTx.RowsAffected > 0 {
			pollStatusChanged = true
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			for _, voteOpt := range poll.Options {
				proposalVoteOptionRecord := model.ProposalVoteOptionRecord{
					MetaforoID:           voteOpt.Id,
					MetaforoVoteID:       proposalVoteRecord.MetaforoID,
					ProposalVoteRecordId: proposalVoteRecord.ID,
				}
				var optLabel string
				switch reflect.TypeOf(voteOpt.Html).Kind() {
				case reflect.Float64:
					optLabel = fmt.Sprintf("%f", voteOpt.Html.(float64))
				default:
					optLabel = voteOpt.Html.(string)
				}
				err = tx.Where(&proposalVoteOptionRecord).Assign(&model.ProposalVoteOptionRecord{
					Text:  optLabel,
					Value: model.GetPredefinedVoteOptionValue(optLabel, dbProposalRcd.VoteType),
				}).FirstOrCreate(&proposalVoteOptionRecord).Error
				if err != nil {
					log.Warn().Msgf("save DB proposal vote option error: %+v", err)
					return err
				}

				err = tx.Model(&proposalVoteOptionRecord).Where("id = ?", proposalVoteOptionRecord.ID).Updates(&model.ProposalVoteOptionRecord{VoterCount: voteOpt.Voters}).Error
				if err != nil {
					log.Warn().Msgf("save DB proposal vote option error: %+v", err)
					return err
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

	log.Debug().Msgf("proposal vote records: %+v", pVoteRcds)

	effectVoteRcd := pVoteRcds[0]

	// Metaforo support multiple vote in one proposal, but in OS only one vote will be created, so only check the first value
	if effectVoteRcd.State == "open" {
		// Refresh dbProposal record
		var dbProposalRcd model.Proposal
		db.Find(&dbProposalRcd, proposalId)

		if dbProposalRcd.Sip != 0 {
			log.Debug().Msgf("proposal %d has SIP set, skip", proposalId)
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

			// setProposalSip also change the proposal state to voting
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
					if _, err = UpdateProposalStateAndLaunchStateChangeActions(db, nil, fmt.Sprintf("%d", proposalId), model.ProposalStatePendingExecution, nil); err != nil {
						log.Error().Msgf("update proposal %d state to pending execution error", proposal.ID)
						return err
					}
					return nil
				} else {
					log.Error().Msgf("NOTICE: this log is generated for creating project proposal with vote, if you found this log, check db or requirements")
					if _, err = UpdateProposalStateAndLaunchStateChangeActions(db, nil, fmt.Sprintf("%d", proposalId), model.ProposalStateExecuted, nil); err != nil {
						log.Error().Msgf("update proposal %d state to pending execution error", proposal.ID)
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
		// FIXME: Get data from cache
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
				projectBudgetRcd := projectBudgetData{
					Name:        fmt.Sprintf("%s %s", budgetParams.Amount, budgetParams.AssetInfo.Name),
					TotalAmount: "0",
				}
				prjBudgetBytes, err := json.Marshal([]projectBudgetData{projectBudgetRcd})
				if err != nil {
					log.Error().Msgf("unmarshal project deliverables data error: %+v", err)
					return nil, err
				}
				newProjectData.Budgets = string(prjBudgetBytes)
			} else if compName == internal.ComponentNameBudget {
				var budgetParams budgetComponentData
				err := json.Unmarshal([]byte(pComponentRecord.Data), &budgetParams)
				if err != nil {
					log.Error().Msgf("unmarshal project deliverables data error: %+v", err)
					return nil, err
				}

				projectBudgetRcds := make([]*projectBudgetData, 0)
				for _, item := range budgetParams.BudgetList {
					projectBudgetRcds = append(projectBudgetRcds, &projectBudgetData{
						Name:        fmt.Sprintf("%s %s", item.Amount, item.AssetInfo.Name),
						TotalAmount: "0",
					})
				}

				prjBudgetBytes, err := json.Marshal(projectBudgetRcds)
				if err != nil {
					log.Error().Msgf("unmarshal project deliverables data error: %+v", err)
					return nil, err
				}
				newProjectData.Budgets = string(prjBudgetBytes)
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

	// FIXME: Add uniq index field to project to avoid duplicated creation
	if err = db.Create(&newProjectData).Error; err != nil {
		log.Error().Msgf("create project error: %+v", err)
		return nil, err
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

	updateTx := db.Clauses(clause.Locking{
		Strength: "UPDATE",
		Options:  "NOWAIT",
	}).Model(&createdProject).Where(&createdProject).First(&createdProject)
	if err = updateTx.Error; err != nil {
		log.Error().Msgf("get associated project error: %+v", err)
		return false, nil, err
	}
	return true, &createdProject, nil
}

func setProposalSip(db *gorm.DB, pTemplate *model.ProposalTemplate, dbProposalRcd *model.Proposal) error {
	var proposalSip = 0

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

	updateTx := db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Model(&dbProposalRcd).
		Where(&dbProposalRcd).
		Where("sip = 0").
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
