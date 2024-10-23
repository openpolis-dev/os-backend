package data_srv

import (
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

const (
	getCurrentSeasonMintableUserVoteRecordsQuery = `select * from proposal_user_vote_records where proposal_id in (
		select proposals.id from proposals where proposals.proposal_template_id in (
			select DISTINCT (m2mpvg.proposal_template_id) from m2m_proposal_voting_gates m2mpvg
			left join proposal_templates pt on m2mpvg.proposal_template_id = pt.id
			left join proposal_vote_gates pvg on m2mpvg.proposal_vote_gate_id = pvg.id where pvg.name like '节点%') and season_id = ?
	)`
)

// getSeasonVoteRecords returns a map of user wallet to their vote count of current season
// The source data is gathered from proposal_user_vote_records table
func getSeasonVoteRecords(db *gorm.DB, currentSeason *model.Season) (map[string]int, error) {
	var currentSeasonMintUserVoteRecords []*model.ProposalUserVoteRecord
	err := db.Raw(getCurrentSeasonMintableUserVoteRecordsQuery, currentSeason.ID).Find(&currentSeasonMintUserVoteRecords).Error
	if err != nil {
		log.Error().Msgf("get current season mintable user vote records error: %+v", err)
		return nil, err
	}

	var metaforoUserVoteCount map[string]int
	for _, record := range currentSeasonMintUserVoteRecords {
		metaforoUserVoteCount[common.FormatUserWallet(record.UserWallet)] += 1
	}
	return metaforoUserVoteCount, nil
}
