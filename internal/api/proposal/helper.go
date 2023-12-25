package proposal

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
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
			if err := db.Save(dbProposalRcd).Error; err != nil {
				log.Error().Msgf("create proposal error: %+v", err)
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
			// Proposal has already published on chain, create new record and bump up version
			newProposalRecord := model.Proposal{
				CreateTs:           time.Now().UTC().Unix(),
				Title:              reqData.Title,
				Applicant:          common.FormatUserWallet(userWallet),
				ProposalCategoryID: reqData.ProposalCategoryId,
				Version:            dbProposalRcd.Version + 1,
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

			return &newProposalRecord, nil
		}
	} else {
		// Init proposal record to get ID
		proposalRecord := model.Proposal{
			CreateTs:           time.Now().UTC().Unix(),
			Title:              reqData.Title,
			Applicant:          common.FormatUserWallet(userWallet),
			ProposalCategoryID: reqData.ProposalCategoryId,
			Version:            1,
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
func SaveProposalToMetaforo(db *gorm.DB, proposalRecord *model.Proposal, metaforoAccessToken string) error {
	// TODO: Build metaforo content from proposal title and content blocks
	var err error

	var proposalCategory *model.ProposalCategory
	err = db.Where(&model.ProposalCategory{ID: proposalRecord.ProposalCategoryID}).First(&proposalCategory).Error
	if err != nil {
		log.Error().Msgf("get proposal category error: %+v", err)
		return err
	}

	var contentBlocks []*model.ProposalContentBlock
	err = db.Where(&model.ProposalContentBlock{ProposalID: proposalRecord.ID}).Find(&contentBlocks).Error
	if err != nil {
		log.Error().Msgf("get proposal category error: %+v", err)
		return err
	}

	metaforoContent := ""
	for _, block := range contentBlocks {
		metaforoContent += "# " + block.Title + "\n\n" + block.Content + "\n\n"
	}

	var metaforoProposal *metaforo.ProposalResponse
	updatedProposalRecord := proposalRecord
	if proposalRecord.ProposalRecordId != "" {
		panic(errors.New("not implemented"))
		// DB Record has ProposalRecordId, the action should be updating existing metaforo proposal
		// TODO: The metaforo ID of proposal can be extracted from proposalRecord.ProposalRecordId
		metaforoProposal, err = metaforo.UpdateProposal(metaforoAccessToken)
		if err != nil {
			log.Error().Msgf("update metaforoProposal error: %+v", err)
			return err
		}

		// Create a new model.Proposal record, and copy associated records to it, then bump up the version
		updatedProposalRecord.Version = proposalRecord.Version + 1
		updatedProposalRecord.ID = 0
		// TODO: Verify whether the record is new created in DB
		if err := db.Create(&updatedProposalRecord).Error; err != nil {
			log.Error().Msgf("bump proposal version error: %+v", err)
			return err
		}

		// Duplicate associated content blocks and components
		if err := db.Transaction(func(tx *gorm.DB) error {
			// Content blocks
			var contentBlocks []model.ProposalContentBlock
			if err := tx.Where(&model.ProposalContentBlock{ProposalID: proposalRecord.ID}).Find(&contentBlocks).Error; err != nil {
				log.Error().Msgf("get proposal blocks error: %+v", err)
				return err
			}
			for _, contentBlock := range contentBlocks {
				if err := tx.Create(&model.ProposalContentBlock{
					ProposalID: updatedProposalRecord.ID,
					Title:      contentBlock.Title,
					Content:    contentBlock.Content,
					CreateTs:   time.Now().UTC().Unix(),
				}).Error; err != nil {
					log.Error().Msgf("create proposal block error: %+v", err)
					return err
				}
			}

			// Components
			var proposalComponents []model.ProposalComponentRecord
			if err := tx.Where(&model.ProposalComponentRecord{ProposalID: proposalRecord.ID}).Find(&proposalComponents).Error; err != nil {
				log.Error().Msgf("get proposal components error: %+v", err)
				return err
			}
			for _, component := range proposalComponents {
				if err := tx.Create(&model.ProposalComponentRecord{
					ProposalID:  updatedProposalRecord.ID,
					ComponentID: component.ComponentID,
					Data:        component.Data,
				}).Error; err != nil {
					log.Error().Msgf("create proposal component error: %+v", err)
					return err
				}
			}
			return nil
		}); err != nil {
			log.Error().Msgf("create proposal component error: %+v", err)
			return err
		}
	} else {
		var err error
		metaforoProposal, err = metaforo.CreateProposal(
			metaforoAccessToken,
			internal.MetaforoGroupName,
			fmt.Sprintf("%d", proposalCategory.MetaforoId),
			proposalRecord.Title,
			metaforoContent, nil, nil,
		)
		if err != nil {
			log.Error().Msgf("update metaforoProposal error: %+v", err)
			return err
		}

		updatedProposalRecord.ProposalRecordId = fmt.Sprintf("metaforo:%d", metaforoProposal.Thread.Id)
		updatedProposalRecord.State = int(model.ProposalStateDraft)
		if metaforoProposal.Thread.EditHistory.Lists != nil && len(metaforoProposal.Thread.EditHistory.Lists) > 0 {
			updatedProposalRecord.ArveaveHash = metaforoProposal.Thread.EditHistory.Lists[0].Arweave
		}
	}

	// Save data backed from metaforo API response to DB
	if err := db.Save(proposalRecord).Error; err != nil {
		log.Error().Msgf("update proposal error: %+v", err)
		return err
	}

	return nil
}
