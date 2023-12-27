package proposal

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

func GetProposalFromStringId(db *gorm.DB, idStr string) (*model.Proposal, error) {
	proposalId, err := strconv.Atoi(idStr)
	if err != nil {
		log.Error().Msgf("parse proposal id %s error: %+v", idStr, err)
		return nil, err
	}
	var proposalRecord model.Proposal
	if err := db.Joins("ProposalCategory").Find(&proposalRecord, proposalId).Error; err != nil {
		return nil, err
	}

	return &proposalRecord, nil
}

func SaveProposalRecordToDB(db *gorm.DB, reqData *CreateOrUpdateProposalData, userWallet string, proposalIdStr string) (*model.Proposal, error) {
	// If proposalIdStr is not empty string, this request should be an update action, otherwise it is a create action.
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

		if dbProposalRcd.State == int(model.ProposalStatePendingSubmit) {
			// Proposal is in PendingSubmit state, the data can be updated directory w/o bumping up version
			dbProposalRcd.Title = reqData.Title
			dbProposalRcd.ProposalCategoryID = reqData.ProposalCategoryId

			voteStartTime := time.Now().UTC().Add(internal.DefaultVoteStartDelay)
			voteEndTime := time.Now().UTC().Add(internal.DefaultVoteStartDelay + internal.DefaultVoteDuration)

			if err := db.Model(model.ProposalVoteRecord{}).
				Where("proposal_id = ?", dbProposalRcd.ID).
				Updates(model.ProposalVoteRecord{StartTs: voteStartTime.Unix(), EndTs: voteEndTime.Unix()}).
				Error; err != nil {
				log.Error().Msgf("update proposal error: %+v", err)
				return nil, err
			}

			// Update proposal content blocks, includes update existing blocks and remove deleted blocks
			if err := SaveProposalContentRecords(db, dbProposalRcd, reqData.ContentBlocks); err != nil {
				log.Error().Msgf("create proposal block error: %+v", err)
				return nil, err
			}

			// Create proposal components
			if err := SaveProposalComponentRecords(db, dbProposalRcd.ID, reqData.Components); err != nil {
				log.Error().Msgf("create proposal component blocks error: %+v", err)
				return nil, err
			}
			return dbProposalRcd, nil
		} else {
			voteStartTime := time.Now().UTC().Add(internal.DefaultVoteStartDelay)
			voteEndTime := time.Now().UTC().Add(internal.DefaultVoteStartDelay + internal.DefaultVoteDuration)

			voteRecords := []*model.ProposalVoteRecord{
				{
					GateID:  0,
					StartTs: voteStartTime.Unix(),
					EndTs:   voteEndTime.Unix(),
				},
			}

			// Proposal has already published on chain, create new record and bump up version
			newProposalRecord := model.Proposal{
				CreateTs:           time.Now().UTC().Unix(),
				Title:              reqData.Title,
				Applicant:          common.FormatUserWallet(userWallet),
				ProposalCategoryID: reqData.ProposalCategoryId,
				Version:            dbProposalRcd.Version + 1,
				VoteRecords:        voteRecords,
			}

			if err := db.Create(&newProposalRecord).Error; err != nil {
				log.Error().Msgf("create proposal error: %+v", err)
				return nil, err
			}

			// Remove ID field from request data, then it will be created with new proposal ID
			newContentBlocks := lo.Map(reqData.ContentBlocks, func(block *FrontendContentBlockRecord, _ int) *FrontendContentBlockRecord {
				block.ID = 0
				return block
			})

			// Create proposal content blocks
			if err := SaveProposalContentRecords(db, &newProposalRecord, newContentBlocks); err != nil {
				log.Error().Msgf("create proposal block error: %+v", err)
				return nil, err
			}

			newComponents := lo.MapValues(reqData.Components, func(component *ComponentRequestData, _ string) *ComponentRequestData {
				component.ID = 0
				return component
			})

			// Create proposal components
			if err := SaveProposalComponentRecords(db, newProposalRecord.ID, newComponents); err != nil {
				log.Error().Msgf("create proposal component blocks error: %+v", err)
				return nil, err
			}

			// TODO: Update proposal on Metaforo and vote datetime

			return &newProposalRecord, nil
		}
	} else {
		// Create a vote will be started after 14 days, and close time will be set to 28 days
		// The vote is only saved in DB, and only will be updated to Metaforo after published
		voteStartTime := time.Now().UTC().Add(14 * 24 * time.Hour)
		voteEndTime := time.Now().UTC().Add(28 * 24 * time.Hour)

		voteRecords := []*model.ProposalVoteRecord{
			{
				GateID:  reqData.VoteGateId,
				StartTs: voteStartTime.Unix(),
				EndTs:   voteEndTime.Unix(),
			},
		}

		// Init proposal record to get ID
		proposalRecord := model.Proposal{
			CreateTs:           time.Now().UTC().Unix(),
			Title:              reqData.Title,
			Applicant:          common.FormatUserWallet(userWallet),
			ProposalCategoryID: reqData.ProposalCategoryId,
			Version:            1,
			VoteRecords:        voteRecords,
		}

		if err := db.Create(&proposalRecord).Error; err != nil {
			log.Error().Msgf("create proposal error: %+v", err)
			return nil, err
		}

		// Create proposal content blocks
		if err := SaveProposalContentRecords(db, &proposalRecord, reqData.ContentBlocks); err != nil {
			log.Error().Msgf("create proposal block error: %+v", err)
			return nil, err
		}

		// Create proposal components
		if err := SaveProposalComponentRecords(db, proposalRecord.ID, reqData.Components); err != nil {
			log.Error().Msgf("create proposal component blocks error: %+v", err)
			return nil, err
		}

		return &proposalRecord, nil
	}
}

func SaveProposalContentRecords(db *gorm.DB, proposalRecord *model.Proposal, reqContentBlockData []*FrontendContentBlockRecord) error {
	var existingContentBlockIds []uint
	err := db.Model(&model.ProposalContentBlock{}).Where(model.ProposalContentBlock{ProposalID: proposalRecord.ID}).Pluck("id", &existingContentBlockIds).Error
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
				ProposalID: proposalRecord.ID,
				Title:      block.Title,
				Content:    block.Content,
				CreateTs:   time.Now().UTC().Unix(),
			}).Error; err != nil {
				log.Error().Msgf("create proposal block error: %+v, block data: %+v", err, block)
				log.Error().Msgf("create proposal block error: %+v", err)
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

func SaveProposalComponentRecords(db *gorm.DB, proposalId uint, reqComponentData map[string]*ComponentRequestData) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, componentData := range reqComponentData {
			// Try to get component record from DB
			componentRecord := model.Component{
				Name: componentData.Name,
			}
			if err := tx.Where(&componentRecord).Error; err != nil {
				tx.Rollback()
				log.Error().Msgf("create proposal component error: %+v", err)
				return err
			}

			// Create proposal component record and save to DB
			if err := db.Save(&model.ProposalComponentRecord{
				ID:          componentRecord.ID,
				CreateTs:    time.Now().UTC().Unix(),
				ComponentID: componentRecord.ID,
				ProposalID:  proposalId,
				Data:        componentData.Data,
			}).Error; err != nil {
				log.Error().Msgf("create proposal component error: %+v", err)
				return err
			}
		}
		return nil
	})
}

// SaveProposalToMetaforo updates proposal record to Metaforo
// If proposal is in pending submit state, update existing proposal record (state, ProposalRecordId, ArveaveHash) and save back,
// the version is keep the same. The metaforo API invoked here is CreateProposal.
//
// Otherwise, copy the proposal to new record with ver+1, update the metaforo data, and save back as a new record,
// and the metaforo API invoked here is updateProposal.
func SaveProposalToMetaforo(db *gorm.DB, origProposalRecord *model.Proposal, metaforoAccessToken string) error {
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
	if origProposalRecord.ProposalRecordId != "" {
		log.Error().Msgf("DB Record has ProposalRecordId, update metaforo proposal")
		// DB Record has ProposalRecordId, this is updating metaforo proposal action, which contains
		// * Duplicate current proposal record to new one, with bumped version and same ProposalRecordId
		// * Invoke metaforo.UpdateProposal to update metaforo proposal data
		// * Save the new metaforo proposal data back to new created DB record
		// This logic is invoked while changing proposal state form Withdrawn, Rejected to Draft

		// Duplicate current proposal record to new one, with content blocks, components
		updatedProposalRecord, err = origProposalRecord.BumpUpVersion(db)
		if err != nil {
			log.Error().Msgf("duplicate proposal data error: %+v", err)
			return err
		}

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

		// TODO: Invoke update vote start/end ts API to extend the vote start time
		for _, record := range updatedProposalRecord.VoteRecords {
			fmt.Printf("vote metaforo id: %d\n", record.MetaforoID)
		}

		metaforoProposalResponse, err = metaforo.GetProposal(metaforoThreadId, internal.MetaforoGroupName)
		api.PrintStructAsJson(metaforoProposalResponse, "TTT: metaforo proposal after getting detail")

		if err != nil {
			log.Error().Msgf("get metaforoProposal %d error: %+v", metaforoThreadId, err)
			return err
		}

		updatedProposalRecord.State = int(model.ProposalStateDraft)
	} else {
		log.Error().Msgf("DB Record has no ProposalRecordId, create metaforo proposal")
		// DB Record has no ProposalRecordId, this is creating metaforo proposal action, which contains:
		// * Invoke metaforo.CreateProposal to create metaforo proposal record
		// * Save the metaforo proposal data back to DB record
		// This logic is invoked while changing proposal state form PendingSubmit to Draft

		// Update VoteRecords startTs and endTs to default delay, and generate vote form data
		err := db.Where(model.ProposalVoteRecord{ProposalID: updatedProposalRecord.ID}).Updates(model.ProposalVoteRecord{
			StartTs: time.Now().Add(internal.DefaultVoteStartDelay).UTC().Unix(),
			EndTs:   time.Now().Add(internal.DefaultVoteStartDelay + internal.DefaultVoteDuration).UTC().Unix(),
		}).Error
		if err != nil {
			log.Error().Msgf("update proposal vote record start / end ts error: %+v", err)
			return err
		}

		voteFormBytes, err := BuildMetaforoVoteFormDataBytes(updatedProposalRecord.VoteRecords)
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

		metaforoProposalResponse, err = metaforo.GetProposal(metaforoCreateProposalResponse.Thread.Id, internal.MetaforoGroupName)
		api.PrintStructAsJson(metaforoProposalResponse, "TTT: metaforo proposal after getting detail")

		if err != nil {
			log.Error().Msgf("get metaforoProposal %d error: %+v", metaforoCreateProposalResponse.Thread.Id, err)
			return err
		}

		// This branch only invoked while changing proposal state from PendingSubmit to Draft
		updatedProposalRecord.State = int(model.ProposalStateDraft)
	}

	updatedProposalRecord.ProposalRecordId = model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposalResponse.Thread.Id)
	if metaforoProposalResponse.Thread.EditHistory.Lists != nil && len(metaforoProposalResponse.Thread.EditHistory.Lists) > 0 {
		updatedProposalRecord.ArveaveHash = metaforoProposalResponse.Thread.EditHistory.Lists[0].Arweave
	}
	// TODO: Save vote info

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
