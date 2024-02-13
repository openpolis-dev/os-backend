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

func SaveProposalRecordToDB(db *gorm.DB, reqData *CreateOrUpdateProposalData, userWallet string, proposalIdStr string, cfg *config.Config) (*model.Proposal, error) {
	// If proposalIdStr is not empty string, this request should be an update action, otherwise it is a creation action.
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

	log.Debug().Msgf("save proposal record to DB: %+v, proposalIdStr: %s, user wallet: %s", reqData, proposalIdStr, userWallet)

	// Update title for testing
	if !strings.HasPrefix(reqData.Title, cfg.MetaforoData.ProposalPrefix) {
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

	if proposalIdStr != "" {
		// Updating existing proposals
		dbProposalRcd, err := GetProposalFromStringId(db, proposalIdStr)
		if err != nil {
			log.Error().Msgf("get proposal error: %+v", err)
			return nil, err
		}
		proposalRcd := dbProposalRcd
		// Bump proposal version if it is not in PendingSubmit state
		if dbProposalRcd.State != int(model.ProposalStatePendingSubmit) {
			// Proposal has already published on chain, create new record and bump up version
			newProposalRecord, err := dbProposalRcd.BumpUpVersion(db)
			if err != nil {
				log.Error().Msgf("duplicate proposal error: %+v", err)
				return nil, err
			}
			proposalRcd = newProposalRecord
		}

		// Update fields
		// Proposal is in PendingSubmit state, the data can be updated directory w/o bumping up version
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

		err = db.Save(&proposalRcd).Error
		if err != nil {
			log.Error().Msgf("duplicate proposal error: %+v", err)
			return nil, err
		}

		// Move vote record from original proposal to new one
		if err = db.Model(&model.ProposalVoteRecord{}).Where("proposal_id = ?", proposalIdStr).Updates(&model.ProposalVoteRecord{ProposalID: proposalRcd.ID}).Error; err != nil {
			log.Error().Msgf("move proposal vote record error: %+v", err)
			return nil, err
		}

		if err := SaveProposalContentRecords(db, proposalRcd.ID, reqData.ContentBlocks); err != nil {
			log.Error().Msgf("create proposal block error: %+v", err)
			return nil, err
		}

		// Create proposal components
		if err := SaveProposalComponentRecords(db, proposalRcd.ID, userWallet, reqData.Components); err != nil {
			log.Error().Msgf("create proposal component blocks error: %+v", err)
			return nil, err
		}
		//
		//if err := SaveProposalVoteOptionRecords(db, proposalRcd.ID, userWallet, reqData.VoteType, reqData.VoteOptions); err != nil {
		//	log.Error().Msgf("create proposal vote option records error: %+v", err)
		//	return nil, err
		//}

		return proposalRcd, nil
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

		if err := db.Create(&proposalRecord).Error; err != nil {
			log.Error().Msgf("create proposal error: %+v", err)
			return nil, err
		}

		// Create proposal content blocks
		if err := SaveProposalContentRecords(db, proposalRecord.ID, reqData.ContentBlocks); err != nil {
			log.Error().Msgf("create proposal block error: %+v", err)
			return nil, err
		}

		// Create proposal components
		if err := SaveProposalComponentRecords(db, proposalRecord.ID, userWallet, reqData.Components); err != nil {
			log.Error().Msgf("create proposal component blocks error: %+v", err)
			return nil, err
		}

		return &proposalRecord, nil
	}
}

func SaveProposalContentRecords(db *gorm.DB, proposalRecordId uint, reqContentBlockData []*FrontendContentBlockRecord) error {
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

	if origProposalRecord.ProposalRecordId != "" {
		// DB Record has ProposalRecordId, this is updating metaforo proposal action, which contains
		// * Invoke metaforo.UpdateProposal to update metaforo proposal data
		// * Save the new metaforo proposal data back to new created DB record
		// This logic is invoked while changing proposal state form Withdrawn, Rejected to Draft

		metaforoThreadId := updatedProposalRecord.GetMetaforoThreadId()
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

		api.PrintStructAsJson(voteRecords, "TTT: vote records")

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

		metaforoProposalResponse, err = metaforo.GetProposal(metaforoThreadId, metaforoGroupName, "", 0)
		if err != nil {
			log.Error().Msgf("get metaforoProposal %d error: %+v", metaforoThreadId, err)
			return err
		}

		err = UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord.ID, metaforoProposalResponse)
		if err != nil {
			log.Error().Msgf("update db records from metaforoProposalResponse error: %+v", err)
		}

		updatedProposalRecord.State = int(model.ProposalStateDraft)
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
		}

		metaforoProposalResponse, err = metaforo.GetProposal(metaforoCreateProposalResponse.Thread.Id, metaforoGroupName, "", 0)
		if err != nil {
			log.Error().Msgf("get metaforoProposal %d error: %+v", metaforoCreateProposalResponse.Thread.Id, err)
			return err
		}

		err = UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord.ID, metaforoProposalResponse)
		if err != nil {
			log.Error().Msgf("update db records from metaforoProposalResponse error: %+v", err)
		}

		// This branch only invoked while changing proposal state from PendingSubmit to Draft
		updatedProposalRecord.State = int(model.ProposalStateDraft)
	}

	updatedProposalRecord.ProposalRecordId = model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposalResponse.Thread.Id)
	err = UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update db proposal record with metaforo response error: %+v", err)
		return err
	}

	// Save data backed from metaforo API response to DB
	// TODO: Use single transaction function to update proposal
	if err := db.Model(&updatedProposalRecord).
		Where("id = ?", updatedProposalRecord.ID).
		Updates(&model.Proposal{ProposalRecordId: updatedProposalRecord.ProposalRecordId, State: updatedProposalRecord.State}).Error; err != nil {
		log.Error().Msgf("update proposal error: %+v", err)
		return err
	}

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

// UpdateDbRecordsFromMetaforoProposalResponse updates proposal data with db records. For now, it contains:
// * Arweave hash: current and historical versions
func UpdateDbRecordsFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcdId uint, metaforoProposal *metaforo.ProposalResponse) error {
	var err error

	var dbProposalRcd *model.Proposal
	db.Find(&dbProposalRcd, dbProposalRcdId)

	// Save all version proposal arweave hash
	proposalRecordId := model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposal.Thread.Id)
	if dbProposalRcd.ProposalRecordId == proposalRecordId {
		var dbProposals []*model.Proposal
		err = db.Where(&model.Proposal{ProposalRecordId: dbProposalRcd.ProposalRecordId}).Order("version DESC").Find(&dbProposals).Error
		if err == nil && len(dbProposals) > 0 {
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
		} else {
			if err != nil {
				log.Error().Msgf("fetch proposal data with recordId %s hash error: %+v", dbProposalRcd.ProposalRecordId, err)
			} else {
				// Init state, no db proposal with given metaforoID found, do not update anything
			}
		}
	}

	// Save user id and wallet from comments data
	err = db.Transaction(func(tx *gorm.DB) error {
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
	})

	// Save proposal vote records
	for _, poll := range metaforoProposal.Thread.Polls {
		proposalVoteRecord := model.ProposalVoteRecord{MetaforoID: poll.Id}
		err := db.Where(&proposalVoteRecord).Assign(&model.ProposalVoteRecord{
			Title:      poll.Title,
			StartTs:    poll.PollStartAt.UTC().Unix(),
			EndTs:      poll.CloseAt.UTC().Unix(),
			ProposalID: dbProposalRcdId,
			VoteType:   dbProposalRcd.VoteType,
		}).FirstOrCreate(&proposalVoteRecord).Error
		if err != nil {
			log.Warn().Msgf("save DB proposal vote record error: %+v", err)
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
		}

		// TODO: Update the check logic of vote result with voter user limitations
		if poll.Status == "open" {
			// Refresh dbProposal record
			db.Find(&dbProposalRcd, dbProposalRcd.ID)
			if dbProposalRcd.Sip == 0 {
				if !dbProposalRcd.IsInFinState() && dbProposalRcd.State != int(model.ProposalStateVoting) {
					var pTemplate *model.ProposalTemplate
					if err := db.Model(&dbProposalRcd).Association("ProposalTemplate").Find(&pTemplate); err != nil {
						log.Error().Msgf("get proposal template error: %+v", err)
						return err
					}

					var proposalSip = 0

					if pTemplate != nil && pTemplate.Type == model.ProposalTemplateTypeCloseProject {
						createProjectProposal := model.Proposal{ID: dbProposalRcd.AssociateProposalId}
						if err = db.Find(&createProjectProposal).Error; err != nil {
							log.Error().Msgf("get creating project proposal error: %+v", err)
							return err
						}
						proposalSip = createProjectProposal.Sip
					} else {
						// TODO: The GetNextSipValue may be invoked more than once, need preventing it by lock or others
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
					}
				}
			}
		} else if poll.Status == "close" {
			// In current logic, only one vote can be existing in proposal, so if got one close state vote, exit the loop
			// If poll closed in proposal, only process this one and exit

			err := UpdateProposalStateBasedOnVoteResult(db, &proposalVoteRecord, dbProposalRcd)
			if err != nil {
				log.Warn().Msgf("process proposal state error: %+v", err)
				continue
			}

			break
		} else {
			log.Warn().Msgf("unknown poll status: %+v", poll.Status)
		}
	}

	return nil
}

func UpdateProposalStateBasedOnVoteResult(db *gorm.DB, proposalVoteRecord *model.ProposalVoteRecord, dbProposalRcd *model.Proposal) error {
	log.Debug().Msgf("update proposal %d state based on vote result: %+v", dbProposalRcd.ID, proposalVoteRecord)
	api.PrintStructAsJson(dbProposalRcd, "TTT: before update")
	db.First(&dbProposalRcd, dbProposalRcd.ID)
	api.PrintStructAsJson(dbProposalRcd, "TTT: after update")
	if dbProposalRcd.IsInFinState() || dbProposalRcd.State == int(model.ProposalStatePendingExecution) {
		log.Warn().Msgf("proposal %d in state %d, not need to apply post job.", dbProposalRcd.ID, dbProposalRcd.State)
		return nil
	}

	var err error

	var voteOptRcds []*model.ProposalVoteOptionRecord
	err = db.Where(&model.ProposalVoteOptionRecord{ProposalVoteRecordId: proposalVoteRecord.ID}).Find(&voteOptRcds).Error
	if err != nil {
		log.Warn().Msgf("fetch vote options for proposal vote record: %+v error: %+v", proposalVoteRecord, err)
		return err
	}

	totalVoterCount := lo.SumBy(voteOptRcds, func(r *model.ProposalVoteOptionRecord) int { return r.VoterCount })

	var proposalFinalState model.ProposalState
	var voteResult string
	switch proposalVoteRecord.VoteType {
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
			err = db.Where(&model.ProposalVoteOptionRecord{ProposalVoteRecordId: proposalVoteRecord.ID}).Find(&voteOptRcds).Error
			if err != nil {
				log.Warn().Msgf("fetch vote options for proposal vote record: %+v error: %+v", proposalVoteRecord, err)
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
		go createProposalAutomationTasks(db, dbProposalRcd.ID, proposalFinalState, voteResult, dbProposalRcd.VoteType)
	}

	return nil
}

// createProposalAutomationTasks creates automation tasks after proposal finished (passed or failed)
func createProposalAutomationTasks(db *gorm.DB, proposalId uint, finState model.ProposalState, voteResult string, voteType int) {
	log.Debug().Msgf("enter createProposalAutomationTasks proposal id: %d, finState: %+v", proposalId, finState)
	sqlQuery := QueryComponentActionNameBaseSQL + " WHERE proposal_id = ?"
	var proposalComponentActions []*proposalComponentActions
	err := db.Raw(sqlQuery, proposalId).Find(&proposalComponentActions).Error
	if err != nil {
		log.Error().Msgf("fetch proposal component actions error: %+v", err)
		return
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
				return
			}
			var pTemplate *model.ProposalTemplate
			if err = db.Model(&proposal).Association("ProposalTemplate").Find(&pTemplate); err != nil {
				log.Error().Msgf("find proposal template error: %+v", err)
				return
			}

			if proposal.PendingExecutionSecond != 0 {
				var proposalComponentRecord model.ProposalComponentRecord
				if err = db.Model(&proposalComponentRecord).
					Where(map[string]any{"proposal_id": proposal.ID, "component_id": 0}). // Note: 0 won't be passed to query if using struct data
					First(&proposalComponentRecord).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						proposalComponentRecord.ProposalID = proposal.ID
						proposalComponentRecord.ComponentID = 0
						db.Create(&proposalComponentRecord)
					} else {
						log.Error().Msgf("create proposal component record error: %+v", err)
						return
					}
				} else {
					log.Debug().Msgf("automation for updating state has already created, return")
					return
				}

				updateProposalStateTaskParams := map[string]any{
					"proposal_id": proposalId,
					"state":       int(model.ProposalStateExecuted),
				}

				jobParamsStr, err := json.Marshal(updateProposalStateTaskParams)
				if err != nil {
					log.Error().Msgf("marshal update proposal state params error: %+v", err)
					return
				}
				err = createCronJob(db, proposal.ID, proposal.PendingExecutionSecond, internal.TaskUpdateProposalState, string(jobParamsStr), "", 0, int(proposalComponentRecord.ID))
				if err != nil {
					log.Error().Msgf("marshal update proposal state params error: %+v", err)
					return
				}
				// TODO: Merge to single state transit function
				if err = db.Model(&proposal).Where("id = ?", proposal.ID).Update("state", model.ProposalStatePendingExecution).Error; err != nil {
					log.Error().Msgf("update proposal %d state to pending execution error", proposal.ID)
					return
				}
			} else {
				// TODO: Merge to single state transit function
				proposal.State = int(model.ProposalStateExecuted)
				if err = db.Model(&proposal).Where("id = ?", proposal.ID).Update("state", model.ProposalStateExecuted).Error; err != nil {
					if err != nil {
						log.Error().Msgf("update proposal %d state to executed error", proposal.ID)
					}
				}
			}
		}

		var proposal model.Proposal
		if err = db.Find(&proposal, proposalId).Error; err != nil {
			log.Error().Msgf("find proposal error: %+v", err)
			return
		}

		for _, componentAction := range proposalComponentActions {
			log.Debug().Msgf("proposal %d vote finState: %+v, action: %+v", proposalId, finState, componentAction)
			var actionName string
			actionName = componentAction.ApproveActionName
			err = createCronJob(db, proposal.ID, proposal.PendingExecutionSecond, actionName, componentAction.ComponentParams, voteResult, voteType, componentAction.ProposalComponentRecordId)
			if err != nil {
				log.Error().Msgf("create cron job error: %+v", err)
			}

			// Update proposal state to PendingExecution, the next state change will be launched by cron job or veto proposal
			// TODO: Change to single state transaction function
			err = db.Model(&proposal).Where("id = ?", proposal.ID).Update("state", int(model.ProposalStatePendingExecution)).Error
			if err != nil {
				log.Error().Msgf("update proposl state to pending execution error: %+v", err)
			}
		}
	case model.ProposalStateVoteFailed:
		var proposal model.Proposal
		if err = db.Find(&proposal, proposalId).Error; err != nil {
			log.Error().Msgf("find proposal error: %+v", err)
			return
		}

		//For close project proposal, need to change the project status to close_failed
		proposalIsForClosingProject, project, err := IsProposalIsForClosingProject(db, proposal.ID)
		if err != nil {
			log.Error().Msgf("checking proposal is for closing project failed, err: %+v", err)
		}

		if !proposalIsForClosingProject {
			log.Debug().Msgf("proposal is not for closing project")
		}

		log.Debug().Msgf("proposal %d is for closing project: %+v", proposal.ID, proposalIsForClosingProject)

		updateTx := db.Model(&project).
			Where("status = 'closing'").
			Update("status", model.ProjectStatusCloseFailed)

		if err = updateTx.Error; err != nil {
			log.Error().Msgf("close project error: %+v", err)
		} else if updateTx.RowsAffected == 0 {
			err = fmt.Errorf("project not in correct status for updating to %+v", model.ProjectStatusCloseFailed)
			log.Error().Msgf(err.Error())
		} else {
			log.Debug().Msgf("complete project status update")
		}
	default:
		log.Error().Msgf("unknown proposal state: %d", finState)
		return
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
