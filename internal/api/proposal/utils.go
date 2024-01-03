package proposal

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

const QueryMetaforoUserWithOsUserBaseSQL = `
select u.avatar as avatar_link, u.wallet as wallet, mu.* from users u inner join metaforo_users mu on u.wallet = mu.user_wallet
`

func GetMetaforoProposalByInternalId(db *gorm.DB, proposalIdStr string) (*model.Proposal, *metaforo.ProposalResponse, error) {
	proposalRcd, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		log.Error().Msgf("get db proposal id %s error: %+v", proposalIdStr, err)
		return nil, nil, err
	}

	proposalDetailRecord, err := metaforo.GetProposal(proposalRcd.GetMetaforoThreadId(), internal.MetaforoGroupName, "", 0)
	if err != nil {
		log.Error().Msgf("get metaforo proposal error: %+v", err)
		return nil, nil, err
	}

	var userdata model.User
	err = db.Model(model.User{}).Where("wallet = ?", proposalRcd.Applicant).First(&userdata).Error
	if err != nil {
		log.Error().Msgf("fetch user data error: %+v", err)
		return nil, nil, err
	}

	editHistory, err := GetLocalEditHistories(db, proposalDetailRecord)
	if err != nil {
		log.Error().Msgf("fetch local history record error: %+v", err)
		return nil, nil, err
	}

	proposalDetailRecord.Thread.EditHistory.Lists = editHistory
	proposalDetailRecord.Thread.EditHistory.Count = len(editHistory)

	return proposalRcd, proposalDetailRecord, nil
}

func GetLocalEditHistories(db *gorm.DB, metaforoProposal *metaforo.ProposalResponse) ([]*metaforo.PostEditHistoryRecord, error) {
	proposalRecordId := model.BuildProposalRecordIdFromMetaforoThreadId(metaforoProposal.Thread.Id)
	var histRecords []*model.Proposal
	err := db.Model(model.Proposal{}).
		Where("proposal_record_id = ?", proposalRecordId).
		Select("title").
		Order("create_ts desc").
		Find(&histRecords).Error
	if err != nil {
		log.Error().Msgf("fetch proposal history record error: %+v", err)
		return nil, err
	}

	editHistory := lo.Map(histRecords, func(r *model.Proposal, _ int) *metaforo.PostEditHistoryRecord {
		return &metaforo.PostEditHistoryRecord{
			CreatedAt: time.Unix(r.CreateTs, 0),
			Title:     r.Title,
		}
	})

	return editHistory, nil
}

func GetOsUserFromMetaforoUserId(db *gorm.DB, metaforoUserIds []int) ([]*model.User, error) {
	var records map[string]any
	err := db.Raw(QueryMetaforoUserWithOsUserBaseSQL+" WHERE mu.metaforo_user_id in ?", metaforoUserIds).Find(&records).Error
	if err != nil {
		return nil, err
	}
	log.Error().Msgf("Found users: %d", len(records))
	api.PrintStructAsJson(records, "")
	return nil, nil
}
