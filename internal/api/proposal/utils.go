package proposal

import (
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

const QueryMetaforoUserWithOsUserBaseSQL = `
select u.avatar as avatar_link, u.wallet as wallet, mu.* from users u inner join metaforo_users mu on u.wallet = mu.user_wallet; 

`

func GetMetaforoProposalByInternalId(db *gorm.DB, proposalIdStr string) (*model.Proposal, *metaforo.ProposalResponse, error) {
	proposalRcd, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		log.Error().Msgf("get db proposal id %s error: %+v", proposalIdStr, err)
		return nil, nil, err
	}

	proposalDetailRecord, err := metaforo.GetProposal(proposalRcd.GetMetaforoThreadId(), internal.MetaforoGroupName, 0)
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

	proposalRecordId := model.BuildProposalRecordIdFromMetaforoThreadId(proposalDetailRecord.Thread.Id)
	var histRecords []*model.Proposal
	err = db.Model(model.Proposal{}).
		Where("proposal_record_id = ?", proposalRecordId).
		Select("title").
		Order("create_ts desc").
		Find(&histRecords).Error
	if err != nil {
		log.Error().Msgf("fetch proposal history record error: %+v", err)
		return nil, nil, err
	}

	editHistory := lo.Map(histRecords, func(r *model.Proposal, _ int) *metaforo.PostEditHistoryRecord {
		return &metaforo.PostEditHistoryRecord{
			CreatedAt: time.Unix(r.CreateTs, 0),
			Title:     r.Title,
		}
	})

	proposalDetailRecord.Thread.EditHistory.Lists = editHistory
	proposalDetailRecord.Thread.EditHistory.Count = len(editHistory)

	return proposalRcd, proposalDetailRecord, nil
}
