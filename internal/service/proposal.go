package service

import (
	"fmt"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
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

func SaveProposalRecordToDB(db *gorm.DB, reqData *proposal.CreateProposalData, userWallet string) (*model.Proposal, error) {
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
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, block := range reqData.ContentBlocks {
			if err := db.Model(&model.ProposalContentBlock{}).Create(&model.ProposalContentBlock{
				ProposalID: proposalRecord.ID,
				Title:      block.Title,
				Content:    block.Content,
				CreateTs:   time.Now().UTC().Unix(),
			}).Error; err != nil {
				log.Error().Msgf("create proposal block error: %+v, block data: %+v", err, block)
				log.Error().Msgf("create proposal block error: %+v", err)
				return err
			}
		}
		return nil
	}); err != nil {
		log.Error().Msgf("create proposal block error: %+v", err)
		return nil, err
	}

	// Create proposal components
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, componentData := range reqData.Components {
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
			if err := db.Create(&model.ProposalComponentRecord{
				CreateTs:    time.Now().UTC().Unix(),
				ComponentId: componentRecord.ID,
				ProposalID:  proposalRecord.ID,
				Data:        componentData.Data,
			}).Error; err != nil {
				log.Error().Msgf("create proposal component error: %+v", err)
				return err
			}
		}
		return nil
	}); err != nil {
		log.Error().Msgf("create proposal component error: %+v", err)
		return nil, err
	}

	return &proposalRecord, nil
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

	var metaforoProposal *metaforo.ProposalResponse
	updatedProposalrecord := proposalRecord
	if proposalRecord.ProposalRecordId != "" {
		// DB Record has ProposalRecordId, the action should be updating existing metaforo proposal
		// TODO: The metaforo ID of proposal can be extracted from proposalRecord.ProposalRecordId
		metaforoProposal, err = metaforo.UpdateProposal(metaforoAccessToken)
		if err != nil {
			log.Error().Msgf("update metaforoProposal error: %+v", err)
			return err
		}

		// Create a new model.Proposal record, and copy associated records to it, then bump up the version
		updatedProposalrecord.Version = proposalRecord.Version + 1
		updatedProposalrecord.ID = 0
		// TODO: Verify whether the record is new created in DB
		if err := db.Create(&updatedProposalrecord).Error; err != nil {
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
					ProposalID: updatedProposalrecord.ID,
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
					ProposalID:  updatedProposalrecord.ID,
					ComponentId: component.ComponentId,
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
			"TODO", proposalRecord.Title, nil, nil, nil,
		)
		if err != nil {
			log.Error().Msgf("update metaforoProposal error: %+v", err)
			return err
		}

		updatedProposalrecord.ProposalRecordId = fmt.Sprintf("metaforo:%d", metaforoProposal.Thread.Id)
		updatedProposalrecord.State = int(model.ProposalStateDraft)
		updatedProposalrecord.ArveaveHash = metaforoProposal.Thread.EditHistory.Lists[0].Arweave
	}

	// Save data backed from metaforo API response to DB
	if err := db.Save(proposalRecord).Error; err != nil {
		log.Error().Msgf("update proposal error: %+v", err)
		return err
	}

	return nil
}
