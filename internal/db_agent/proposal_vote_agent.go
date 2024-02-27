package db_agent

import (
	"errors"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

// UpsertProposalVoteRecord upserts a proposal vote record into DB
// Here is the logic for the function
// 1. Try to create the ProposalVoteRecord, if on conflict with proposalId and voteType, then do nothing
// 2. If the effected rows is 1, then it is a new record, and we need to create all associated ProposalVoteOptionRecord
// 3. If the effected rows is 0, need to validate whether VoteOptionRecord has metaforo ID.
//   - If yes, return error since the data has been uploaded to metaforo and is not updatable
//   - If no, remove all associated ProposalVoteOptionRecord, and create new records from passed in params
//
// Note: this function is only used for proposal haven't submitted to metaforo
func UpsertProposalVoteRecord(db *gorm.DB, proposalId uint, voteType int, voteOptions []*model.ProposalVoteOptionRecord) (*model.ProposalVoteRecord, error) {
	var err error

	if voteType == model.ProposalVoteTypeNone {
		log.Debug().Msgf("skip upsert proposal vote record for none vote type")
		return nil, nil
	}

	proposalVoteRecord := model.ProposalVoteRecord{
		ProposalID: proposalId,
		VoteType:   voteType,
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		createVoteRecordTx := tx.Where(proposalVoteRecord).FirstOrCreate(&proposalVoteRecord)

		if createVoteRecordTx.Error != nil {
			log.Error().Msgf("upsert proposal vote record error: %+v", createVoteRecordTx.Error)
			return createVoteRecordTx.Error
		} else {
			if createVoteRecordTx.RowsAffected == 1 {
				log.Debug().Msgf("new proposal vote record created: %+v, creating associated vote option records", proposalVoteRecord)
				voteOptions = lo.Map(voteOptions, func(r *model.ProposalVoteOptionRecord, _ int) *model.ProposalVoteOptionRecord {
					r.ProposalVoteRecordId = proposalVoteRecord.ID
					return r
				})
				log.Debug().Msgf("vote options to be created: %+v", voteOptions)
				return tx.Create(&voteOptions).Error
			} else if createVoteRecordTx.RowsAffected == 0 {
				log.Debug().Msgf("proposal vote record already exists: %+v, checking whether vote option records have metaforo ID", proposalVoteRecord)
				var existingVoteRecords []*model.ProposalVoteOptionRecord

				if err = tx.Model(&proposalVoteRecord).Association("Options").Find(&existingVoteRecords); err != nil {
					log.Error().Msgf("fetch vote option records error: %+v", err)
					return err
				}

				if len(existingVoteRecords) > 0 && existingVoteRecords[0].MetaforoID != 0 {
					log.Warn().Msgf("vote option records has been uploaded to metaforo, cannot update")
					return nil
				}

				if err = tx.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_vote_record_id = ?", proposalVoteRecord.ID).Delete(&model.ProposalVoteOptionRecord{}).Error; err != nil {
					log.Error().Msgf("delete vote option records error: %+v", err)
					return err
				}

				voteOptions = lo.Map(voteOptions, func(r *model.ProposalVoteOptionRecord, _ int) *model.ProposalVoteOptionRecord {
					r.ProposalVoteRecordId = proposalVoteRecord.ID
					return r
				})
				log.Debug().Msgf("vote options to be created: %+v", voteOptions)
				return tx.Create(&voteOptions).Error
			} else {
				log.Error().Msgf("upsert proposal vote record error, unexpected effected rows: %d", createVoteRecordTx.RowsAffected)
				return errors.New("unexpected effected rows")
			}
		}
	})

	if err != nil {
		return nil, err
	} else {
		return &proposalVoteRecord, nil
	}
}

func GetProposalVoteRecord(db *gorm.DB, proposalId uint) ([]*model.ProposalVoteRecord, error) {
	var err error

	var proposalVoteRecord []*model.ProposalVoteRecord
	if err = db.Where("proposal_id = ?", proposalId).Find(&proposalVoteRecord).Error; err != nil {
		log.Error().Msgf("fetch proposal vote record error: %+v", err)
		return nil, err
	}
	return proposalVoteRecord, nil
}

// GenerateMetaforoVoteOptions fetches vote options from DB for specified proposal and generates the vote options for metaforo
func GenerateMetaforoVoteOptions(proposalId uint) ([]*metaforo.VoteOption, error) {
	agent := GetDbAgent()
	var err error

	var osVoteOptions []*model.ProposalVoteOptionRecord
	if err = agent.db.Model(&model.ProposalVoteOptionRecord{}).Where("proposal_id = ?", proposalId).Find(&osVoteOptions).Error; err != nil {
		log.Error().Msgf("fetch vote option records error: %+v", err)
		return nil, err
	}

	mfVoteOpts := lo.Map(osVoteOptions, func(r *model.ProposalVoteOptionRecord, idx int) *metaforo.VoteOption {
		return &metaforo.VoteOption{Text: r.Text, Type: idx}
	})

	log.Debug().Msgf("metaforo vote options to be created: %+v", mfVoteOpts)
	return mfVoteOpts, nil
}
