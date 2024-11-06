package data_srv

import (
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

var err error

const (
	getCurrentSeasonMintableUserVoteRecordsQuery = `SELECT * FROM proposal_user_vote_records
		WHERE proposal_id IN (SELECT id FROM proposals p WHERE vote_gate_id =
			(SELECT id FROM proposal_vote_gates WHERE season_id = ? AND name like '节点%')
			AND state in (6, 7, 9) AND p.version = (select max(version) from proposals p2 where p.proposal_record_id=p2.proposal_record_id));`
)

// GetSeasonVoteRecords returns a map of user wallet to their vote count of current season
// The source data is gathered from proposal_user_vote_records table
func GetSeasonVoteRecords(db *gorm.DB, seasonRcd *model.Season) (map[string]int, error) {
	var currentSeasonMintUserVoteRecords []*model.ProposalUserVoteRecord
	err = db.Raw(getCurrentSeasonMintableUserVoteRecordsQuery, seasonRcd.ID).Find(&currentSeasonMintUserVoteRecords).Error
	if err != nil {
		log.Error().Msgf("get current season mintable user vote records error: %+v", err)
		return nil, err
	}

	metaforoUserVoteCount := make(map[string]int)
	for _, record := range currentSeasonMintUserVoteRecords {
		metaforoUserVoteCount[common.FormatUserWallet(record.UserWallet)] += 1
	}
	return metaforoUserVoteCount, nil
}
