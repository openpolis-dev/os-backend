package proposal

import (
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

const QueryMetaforoUserWithOsUserBaseSQL = `
SELECT u.wallet            AS wallet,
       mu.metaforo_user_id AS metaforo_user_id,
       u.id                AS os_user_id,
       u.avatar            AS os_avatar,
       u.name              AS os_username
FROM users u
         INNER JOIN metaforo_users mu ON u.wallet = mu.user_wallet`

const QueryProposalWithJointUserBaseSQL = `
SELECT p.title AS title,
u.wallet            AS wallet,
u.name AS os_username,
p.create_ts AS create_ts
FROM users u INNER JOIN proposals p ON u.wallet = p.applicant`

type JointMetaforoAndOsUser struct {
	Wallet string `json:"wallet"`

	MetaforoUserID int `json:"metaforo_user_id"`

	OsUserID   int    `json:"os_user_id"`
	OsAvatar   string `json:"os_avatar"`
	OsUserName string `json:"os_user_name"`
}

func GetMetaforoProposalByInternalId(db *gorm.DB, proposalIdStr string) (*model.Proposal, *metaforo.ProposalResponse, error) {
	osProposalRcd, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		log.Error().Msgf("get db proposal id %s error: %+v", proposalIdStr, err)
		return nil, nil, err
	}

	metaforoProposalRcd, err := metaforo.GetProposal(osProposalRcd.GetMetaforoThreadId(), internal.MetaforoGroupName, "", 0)
	if err != nil {
		log.Error().Msgf("get metaforo proposal error: %+v", err)
		return nil, nil, err
	}

	return osProposalRcd, metaforoProposalRcd, nil
}

func GetLocalEditHistoriesWithOsUserData(db *gorm.DB, proposalRecordId string) ([]*FrontendProposalEditHistoryRecord, error) {
	// Get proposal records with same RecordId
	var localHistoryRecords []*FrontendProposalEditHistoryRecord
	querySql := QueryProposalWithJointUserBaseSQL + " WHERE proposal_record_id = ? ORDER BY create_ts desc"
	err := db.Raw(querySql, proposalRecordId).Find(&localHistoryRecords).Error
	if err != nil {
		log.Error().Msgf("fetch history proposal record error: %+v", err)
		return nil, err
	}

	return localHistoryRecords, nil
}

func GetOsUserFromMetaforoUserId(db *gorm.DB, metaforoUserIds []int) ([]*JointMetaforoAndOsUser, error) {
	var records []*JointMetaforoAndOsUser
	err := db.Raw(QueryMetaforoUserWithOsUserBaseSQL+" WHERE mu.metaforo_user_id in ?", metaforoUserIds).Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}
