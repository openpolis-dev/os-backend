package proposal

import (
	"errors"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
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
p.create_ts AS create_ts,
p.arweave_hash as arweave
FROM users u INNER JOIN proposals p ON u.wallet = p.applicant`

type JointMetaforoAndOsUser struct {
	Wallet string `json:"wallet"`

	MetaforoUserID int `json:"metaforo_user_id"`

	OsUserID   int    `json:"os_user_id"`
	OsAvatar   string `json:"os_avatar"`
	OsUserName string `json:"os_user_name"`
}

const QueryComponentActionNameBaseSQL = `
select pcr.id as proposal_component_record_id,
       pcr.data as component_params,
       approve_pca.command as approve_action_name,
       reject_pca.command  as reject_action_name
from proposal_component_records pcr
         join proposal_components pc on pcr.component_id = pc.id
         join proposal_component_actions approve_pca on pc.approve_action_id = approve_pca.id
         join proposal_component_actions reject_pca on pc.reject_action_id = reject_pca.id`

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

	err = UpdateDbRecordsFromMetaforoProposalResponse(db, osProposalRcd, metaforoProposalRcd)
	if err != nil {
		log.Error().Msgf("update db records from metaforoProposalResponse error: %+v", err)
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

func GetProposalCommentsWithOsUserData(db *gorm.DB, metaforoComments []metaforo.PostData) ([]*FrontendProposalCommentRecord, error) {
	var err error
	var frontendCommentsRecords []*FrontendProposalCommentRecord

	for _, metaforoComment := range metaforoComments {
		userWallet := ""
		if len(metaforoComment.User.Web3PublicKeys) > 0 {
			userWallet = common.FormatUserWallet(metaforoComment.User.Web3PublicKeys[0].Address)
		}

		proposalTitle := ""
		proposalTs := int64(0)
		proposalArweaveHash := ""
		dbComment := model.ProposalComment{MetaforoCommentId: metaforoComment.Id}
		err = db.Model(model.ProposalComment{}).Joins("Proposal").Where(dbComment).First(&dbComment).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				log.Warn().Msgf("porposal comment with metaforo id %d not found, metaforo resposne: %+v", metaforoComment.Id, metaforoComment)
			} else {
				return nil, err
			}
		} else {
			proposalTitle = dbComment.Proposal.Title
			proposalTs = dbComment.Proposal.CreateTs
			proposalArweaveHash = dbComment.Proposal.ArweaveHash
		}

		userRecords, err := GetOsUserFromMetaforoUserId(db, []int{metaforoComment.UserId})
		if err != nil {
			log.Error().Msgf("get user info by wallet %s error: %+v", dbComment.AuthorWallet, err)
			return nil, err
		}

		userAvatar := ""
		if len(userRecords) > 0 {
			userAvatar = userRecords[0].OsAvatar
		}

		var childrenRecords []*FrontendProposalCommentRecord
		if metaforoComment.ChildrenCount > 0 {
			childrenRecords, err = GetProposalCommentsWithOsUserData(db, metaforoComment.Children.Posts)
			if err != nil {
				log.Error().Msgf("convert children comments error: %+v", err)
				return nil, err
			}
		}

		frontendCommentsRecords = append(frontendCommentsRecords, &FrontendProposalCommentRecord{
			MetaforoPostId:      metaforoComment.Id,
			Content:             metaforoComment.Content,
			Wallet:              userWallet,
			Avatar:              userAvatar,
			ReplyMetaforoPostId: metaforoComment.ReplyPid,
			Deleted:             metaforoComment.Deleted == 1,
			Children:            childrenRecords,
			ProposalTitle:       proposalTitle,
			ProposalTs:          proposalTs,
			ProposalArweaveHash: proposalArweaveHash,
			CreatedTs:           metaforoComment.CreatedAt.UTC().Unix(),
		})
	}

	return frontendCommentsRecords, nil
}
