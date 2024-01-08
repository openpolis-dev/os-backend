package proposal

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

type proposalComponentActions struct {
	ProposalComponentRecordId int    `json:"proposal_component_record_id"`
	ComponentParams           string `json:"component_params"`
	ApproveActionName         string `json:"approve_action_name"`
	RejectActionName          string `json:"reject_action_name"`
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

func SaveProposalRecordToDB(db *gorm.DB, reqData *CreateOrUpdateProposalData, userWallet string, proposalIdStr string) (*model.Proposal, error) {
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
		err = db.Save(&proposalRcd).Error
		if err != nil {
			log.Error().Msgf("duplicate proposal error: %+v", err)
			return nil, err
		}

		// Update proposal content blocks, includes update existing blocks and remove deleted blocks
		if err := SaveProposalContentRecords(db, proposalRcd.ID, reqData.ContentBlocks); err != nil {
			log.Error().Msgf("create proposal block error: %+v", err)
			return nil, err
		}

		// Create proposal components
		if err := SaveProposalComponentRecords(db, proposalRcd.ID, userWallet, reqData.Components); err != nil {
			log.Error().Msgf("create proposal component blocks error: %+v", err)
			return nil, err
		}
		return proposalRcd, nil
	} else {
		// Init proposal record to get ID
		proposalRecord := model.Proposal{
			CreateTs:           time.Now().UTC().Unix(),
			Title:              reqData.Title,
			Applicant:          common.FormatUserWallet(userWallet),
			ProposalCategoryID: reqData.ProposalCategoryId,
			Version:            1,
			TemplateId:         reqData.TemplateId,
		}

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
			if err := db.Save(&model.ProposalContentBlock{
				ID:         block.ID,
				ProposalID: proposalRecordId,
				Title:      block.Title,
				Content:    block.Content,
				CreateTs:   time.Now().UTC().Unix(),
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
				if err := db.Delete(&model.ProposalContentBlock{ID: blockId}).Error; err != nil {
					log.Error().Msgf("delete proposal block error: %+v", err)
					return err
				}
			}
		}
		return nil
	})
}

func SaveProposalComponentRecords(db *gorm.DB, proposalId uint, applicantWallet string, reqComponentData map[string]*ComponentRequestData) error {
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

			proposalDataStr, err := json.Marshal(componentData.Data)
			if err != nil {
				tx.Rollback()
				log.Error().Msgf("create proposal component error: %+v", err)
				return err
			}

			// Create proposal component record and save to DB
			if err := db.Save(&model.ProposalComponentRecord{
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
				if err := db.Delete(&model.ProposalComponentRecord{ID: componentId}).Error; err != nil {
					log.Error().Msgf("delete proposal component error: %+v", err)
					return err
				}
			}
		}
		return nil
	})
}

// SaveProposalToMetaforo updates proposal record to Metaforo
// If proposal is in pending submit state, update existing proposal record (state, ProposalRecordId, ArweaveHash) and save back,
// the version is keep the same. The metaforo API invoked here is CreateProposal.
//
// Otherwise, copy the proposal to new record with ver+1, update the metaforo data, and save back as a new record,
// and the metaforo API invoked here is updateProposal.
func SaveProposalToMetaforo(db *gorm.DB, origProposalRecord *model.Proposal, metaforoAccessToken string, EditorType int) error {
	var err error

	// Load current proposal data
	var proposalCategory *model.ProposalCategory
	err = db.Where(&model.ProposalCategory{ID: origProposalRecord.ProposalCategoryID}).First(&proposalCategory).Error
	if err != nil {
		log.Error().Msgf("get proposal category error: %+v", err)
		return err
	}

	var contentBlocks []*model.ProposalContentBlock
	err = db.Where(&model.ProposalContentBlock{ProposalID: origProposalRecord.ID}).Find(&contentBlocks).Error
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
	voteStartTime := time.Now().UTC().Add(internal.DefaultVoteStartDelay)
	voteEndTime := time.Now().UTC().Add(internal.DefaultVoteStartDelay + internal.DefaultVoteDuration)

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
			internal.MetaforoGroupName,
			fmt.Sprintf("%d", proposalCategory.MetaforoId),
			origProposalRecord.Title,
			metaforoContent, nil, "", metaforoThreadId,
		)
		if err != nil {
			log.Error().Msgf("update metaforoProposal error: %+v", err)
			return err
		}

		// Get vote record from original record and update the timestamp
		// The updated vote record will be saved by response in GetProposal function
		for _, record := range origProposalRecord.VoteRecords {
			err = metaforo.UpdateVoteTime(
				metaforoAccessToken,
				internal.MetaforoGroupName,
				record.MetaforoID,
				voteStartTime.Unix(),
				voteEndTime.Unix())
			if err != nil {
				log.Error().Msgf("update metaforoProposal vote error: %+v", err)
				return err
			}
		}

		metaforoProposalResponse, err = metaforo.GetProposal(metaforoThreadId, internal.MetaforoGroupName, "", 0)
		if err != nil {
			log.Error().Msgf("get metaforoProposal %d error: %+v", metaforoThreadId, err)
			return err
		}

		err = UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord, metaforoProposalResponse)
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
				GateID:  0,
				StartTs: voteStartTime.Unix(),
				EndTs:   voteEndTime.Unix(),
			},
		}
		voteFormBytes, err := BuildMetaforoVoteFormDataBytes(voteRecords)
		if err != nil {
			log.Error().Msgf("build metaforoProposal vote data error: %+v", err)
			return err
		}

		metaforoCreateProposalResponse, err := metaforo.CreateProposal(
			metaforoAccessToken,
			internal.MetaforoGroupName,
			fmt.Sprintf("%d", proposalCategory.MetaforoId),
			updatedProposalRecord.Title,
			metaforoContent, nil, string(voteFormBytes))
		if err != nil {
			log.Error().Msgf("update metaforoProposal error: %+v", err)
			return err
		}

		metaforoProposalResponse, err = metaforo.GetProposal(metaforoCreateProposalResponse.Thread.Id, internal.MetaforoGroupName, "", 0)
		if err != nil {
			log.Error().Msgf("get metaforoProposal %d error: %+v", metaforoCreateProposalResponse.Thread.Id, err)
			return err
		}

		err = UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord, metaforoProposalResponse)
		if err != nil {
			log.Error().Msgf("update db records from metaforoProposalResponse error: %+v", err)
		}

		// This branch only invoked while changing proposal state from PendingSubmit to Draft
		updatedProposalRecord.State = int(model.ProposalStateDraft)
	}

	updatedProposalRecord.ProposalRecordId = model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposalResponse.Thread.Id)
	err = UpdateDbRecordsFromMetaforoProposalResponse(db, updatedProposalRecord, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update db proposal record with metaforo response error: %+v", err)
		return err
	}

	// Update vote data from metaforo response
	// TODO: Merge to updateDbRecords function
	// TODO: Should check metaforo Poll Id existing first
	for _, poll := range metaforoProposalResponse.Thread.Polls {
		voteRecord := &model.ProposalVoteRecord{
			Title:      poll.Title,
			StartTs:    voteStartTime.Unix(),
			EndTs:      voteEndTime.Unix(),
			MetaforoID: poll.Id,
			ProposalID: updatedProposalRecord.ID,
		}
		err := db.Create(&voteRecord).Error
		if err != nil {
			log.Error().Msgf("save metaforoProposal vode record failed, raw response: %+v error: %+v", metaforoProposalResponse, err)
			return err
		}
	}

	// Save data backed from metaforo API response to DB
	if err := db.Save(updatedProposalRecord).Error; err != nil {
		log.Error().Msgf("update proposal error: %+v", err)
		return err
	}

	return nil
}

// BuildMetaforoVoteFormDataBytes generates the byte representation of the Metaforo vote form data.
//
// Parameters:
// - voteTitle: The title of the vote.
// - startTime: The start time of the vote.
// - endTime: The end time of the vote.
//
// Returns:
// - []byte: The byte representation of the vote form data.
// - error: An error if there was a problem generating the byte representation.
func BuildMetaforoVoteFormDataBytes(voteRecords []*model.ProposalVoteRecord) ([]byte, error) {
	voteData := lo.Map(voteRecords, func(r *model.ProposalVoteRecord, _ int) *metaforo.NewVoteFormRequest {
		return &metaforo.NewVoteFormRequest{
			Options:            internal.ProposalVoteOptions,
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

func IsUserMetVoteGate(userSeepassData *sdk.SeepassResponse, proposalVoteGate *model.ProposalVoteGate) bool {
	if userSeepassData == nil {
		return false
	}

	// No nft gate set, this vote should be opened to all users
	if proposalVoteGate == nil {
		return true
	}

	for _, sbtInfo := range userSeepassData.Sbt {
		if strings.EqualFold(sbtInfo.ContractAddr, proposalVoteGate.TokenAddress) {
			if strings.EqualFold(sbtInfo.ContractAddr, proposalVoteGate.TokenAddress) {
				if proposalVoteGate.TokenTypeName() == "ERC1155" {
					if strings.EqualFold(proposalVoteGate.TokenId, sbtInfo.TokenId) {
						return true
					}
				} else {
					return true
				}
			}
		}
	}
	return false
}

// UpdateDbRecordsFromMetaforoProposalResponse updates proposal data with db records. For now, it contains:
// * Arweave hash: current and historical versions
func UpdateDbRecordsFromMetaforoProposalResponse(db *gorm.DB, dbProposalRcd *model.Proposal, metaforoProposal *metaforo.ProposalResponse) error {
	var err error

	// Save all version proposal arweave hash
	proposalRecordId := model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposal.Thread.Id)
	if dbProposalRcd.ProposalRecordId == proposalRecordId {
		var dbProposals []*model.Proposal
		err = db.Where(&model.Proposal{ProposalRecordId: dbProposalRcd.ProposalRecordId}).Order("version DESC").Find(&dbProposals).Error
		if err == nil && len(dbProposals) > 0 {
			err = db.Transaction(func(tx *gorm.DB) error {
				for idx := 0; idx < metaforoProposal.Thread.EditHistory.Count; idx++ {
					tx.Model(&dbProposals[idx]).Update("arweave_hash", metaforoProposal.Thread.EditHistory.Lists[idx].Arweave)
					if idx == 0 {
						// Save arwave hash data to record for setting it correctly in response
						tx.Where(&model.Proposal{ID: dbProposalRcd.ID}).Updates(model.Proposal{ArweaveHash: metaforoProposal.Thread.EditHistory.Lists[idx].Arweave})
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
		err := db.Where(&proposalVoteRecord).FirstOrCreate(&proposalVoteRecord).Error
		if err != nil {
			log.Warn().Msgf("get proposal vote DB record error: %+v", err)

		}
		for _, voteOpt := range poll.Options {
			switch voteOpt.Type {
			case 0:
				proposalVoteRecord.ApproveCount = voteOpt.Voters
			case 1:
				proposalVoteRecord.RejectCount = voteOpt.Voters
			case 2:
				proposalVoteRecord.AbstainCount = voteOpt.Voters
			default:
				log.Warn().Msgf("vote is not generated by os, ignore this record")
				continue
			}
		}
		err = db.Save(&proposalVoteRecord).Error
		if err != nil {
			log.Warn().Msgf("update proposal vote DB record error: %+v", err)
		}

		if poll.Status == "close" {
			if proposalVoteRecord.ApproveCount > proposalVoteRecord.RejectCount {
				err = db.Where(&model.Proposal{ID: dbProposalRcd.ID}).Updates(model.Proposal{State: int(model.ProposalStateVotePassed)}).Error
				if err != nil {
					log.Error().Msgf("update proposal state error: %+v", err) // TODO: log
				}
				go createProposalFinTasks(db, dbProposalRcd, model.ProposalStateVotePassed)
			} else {
				err = db.Where(&model.Proposal{ID: dbProposalRcd.ID}).Updates(model.Proposal{State: int(model.ProposalStateVoteFailed)}).Error
				if err != nil {
					log.Error().Msgf("update proposal state error: %+v", err) // TODO: log
				}
				go createProposalFinTasks(db, dbProposalRcd, model.ProposalStateVoteFailed)
			}
		}
	}

	return nil
}

func createProposalFinTasks(db *gorm.DB, proposal *model.Proposal, finState model.ProposalState) {
	sqlQuery := QueryComponentActionNameBaseSQL + " WHERE proposal_id = ?"
	var proposalComponentActions []*proposalComponentActions
	err := db.Raw(sqlQuery, proposal.ID).Find(&proposalComponentActions).Error
	if err != nil {
		log.Error().Msgf("fetch proposal component actions error: %+v", err)
		return
	}

	var proposalCategory *model.ProposalCategory
	err = db.Find(&proposalCategory, proposal.ProposalCategoryID).Error
	if err != nil {
		log.Error().Msgf("fetch proposal category error: %+v", err)
		return
	}

	for _, componentAction := range proposalComponentActions {
		var actionName string
		switch finState {
		case model.ProposalStateVotePassed:
			actionName = componentAction.ApproveActionName
		case model.ProposalStateVoteFailed:
			actionName = componentAction.RejectActionName
		default:
			log.Error().Msgf("unknown proposal state: %d", finState)
			return
		}

		currentTs := time.Now().UTC().Unix()
		finTask := &model.CronJob{
			CreateTs:       currentTs,
			UpdateTs:       currentTs,
			HandlerName:    actionName,
			LastExecTs:     0,
			NextExecTs:     currentTs + proposalCategory.SecondDelayBeforeTaskExecution,
			JobParams:      componentAction.ComponentParams,
			State:          model.CronJobStateActive,
			LastExecResult: "",
		}
		createTaskTx := db.Where(model.CronJob{HandlerName: actionName, ProposalComponentRecordId: componentAction.ProposalComponentRecordId}).
			Assign(&finTask).FirstOrCreate(&finTask)

		if createTaskTx.Error != nil {
			log.Error().Msgf("create proposal fin task error: %+v", err)
			return
		} else if createTaskTx.RowsAffected == 0 {
			log.Warn().Msgf("proposal fin task already exists: %+v", finTask)
		}
	}
}
