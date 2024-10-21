package data_srv

import (
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

const (
	getCurrentSeasonMintableUserVoteRecordsQuery = `select * from proposal_user_vote_records where proposal_id in (
		select proposals.id from proposals where proposals.proposal_template_id in (
			select DISTINCT (m2mpvg.proposal_template_id) from m2m_proposal_voting_gates m2mpvg
			left join proposal_templates pt on m2mpvg.proposal_template_id = pt.id
			left join proposal_vote_gates pvg on m2mpvg.proposal_vote_gate_id = pvg.id where pvg.name like '节点%')
	)`
)

func CalcMetaforoRewards(db *gorm.DB, currentSeason *model.Season) (map[string]decimal.Decimal, error) {
	// Get mintable user vote records for current season
	// Note:  the current season for now is collecting by voting gate record, will be changed to proposa season id after data populated
	var currentSeasonMintableUserVoteRecords []*model.ProposalUserVoteRecord
	err := db.Raw(getCurrentSeasonMintableUserVoteRecordsQuery, currentSeason.ID).Find(&currentSeasonMintableUserVoteRecords).Error
	if err != nil {
		log.Error().Msgf("get current season mintable user vote records error: %+v", err)
		return nil, err
	}

	var metaforoUserVoteCount map[string]int
	for _, record := range currentSeasonMintableUserVoteRecords {
		metaforoUserVoteCount[common.FormatUserWallet(record.UserWallet)] += 1
	}

	return nil, nil
}
