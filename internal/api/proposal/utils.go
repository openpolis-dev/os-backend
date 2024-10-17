package proposal

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const ListProposalsSQL = `
SELECT p.id,
       p.title,
       lower(p.applicant) as applicant,
       u.avatar as applicant_avatar,
       pc.name  as category_name,
       p.create_ts,
       p.sip,
       p.version,
       p.state as state_id
FROM proposals p
         JOIN (SELECT proposal_record_id, MAX(version) AS max_version
               FROM proposals
               GROUP BY proposal_record_id) t2
              ON p.proposal_record_id = t2.proposal_record_id AND p.version = t2.max_version
         JOIN proposal_categories pc ON p.proposal_category_id = pc.id
         JOIN users u ON p.applicant = u.wallet`

const ListProposalsSQLForGettingCreatingProjectProposal = `
SELECT p.id,
       p.title,
       lower(p.applicant) as applicant,
       u.avatar as applicant_avatar,
       pc.name  as category_name,
       p.create_ts,
       p.sip,
       projects.status as project_status,
       p.version,
       p.state as state_id
FROM proposals p
         JOIN (SELECT proposal_record_id, MAX(version) AS max_version
               FROM proposals
               GROUP BY proposal_record_id) t2
              ON p.proposal_record_id = t2.proposal_record_id AND p.version = t2.max_version
         JOIN proposal_categories pc ON p.proposal_category_id = pc.id
         JOIN users u ON p.applicant = u.wallet
         JOIN projects ON projects.s_ip = p.sip::text`

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

func GetMetaforoProposalByInternalId(db *gorm.DB, proposalIdStr string, metaforoGroupName string, mfAccessToken string) (*model.Proposal, *metaforo.ProposalResponse, error) {
	osProposalRcd, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		log.Error().Msgf("get db proposal id %s error: %+v", proposalIdStr, err)
		return nil, nil, err
	}

	metaforoProposalResponse, err := metaforo.GetProposal(osProposalRcd.GetMetaforoThreadId(), metaforoGroupName, mfAccessToken, 0)
	if err != nil {
		log.Error().Msgf("get metaforo proposal error: %+v", err)
		return nil, nil, err
	}

	err = UpdateDbRecordsFromMetaforoProposalResponse(db, osProposalRcd.ID, metaforoProposalResponse)
	if err != nil {
		log.Error().Msgf("update db records from metaforoProposalResponse error: %+v", err)
	}

	return osProposalRcd, metaforoProposalResponse, nil
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

		var mfContent string
		switch reflect.TypeOf(metaforoComment.Content).Kind() {
		case reflect.Float64:
			mfContent = strconv.FormatFloat(metaforoComment.Content.(float64), 'f', -1, 64)
		default:
			mfContent = metaforoComment.Content.(string)
		}

		frontendCommentsRecords = append(frontendCommentsRecords, &FrontendProposalCommentRecord{
			MetaforoPostId:      metaforoComment.Id,
			Content:             mfContent,
			Wallet:              userWallet,
			Avatar:              userAvatar,
			ReplyMetaforoPostId: metaforoComment.ReplyPid,
			Deleted:             metaforoComment.DeletedBy != nil,
			Children:            childrenRecords,
			ProposalTitle:       proposalTitle,
			ProposalTs:          proposalTs,
			ProposalArweaveHash: proposalArweaveHash,
			CreatedTs:           metaforoComment.CreatedAt.UTC().Unix(),
			IsRejected:          dbComment.IsRejectComment,
		})
	}

	return frontendCommentsRecords, nil
}

// PopulateUserWalletFromMetaforoIds populates user wallet from metaforo user ids
// The function first check whether the user wallet is empty, if so it will try to get the user detail from metaforo API then save the wallet to db
// It will also create user record in db if not exist
func PopulateUserWalletFromMetaforoIds(db *gorm.DB, metaforoUserIds []int) (map[int]string, error) {
	// Verify the user are new record which haven't been created in our db
	var missingWalletMetaforoUser []*model.MetaforoUser
	err = db.Model(&model.MetaforoUser{}).Where("user_wallet = ?", "").Find(&missingWalletMetaforoUser).Error
	if err != nil {
		log.Error().Msgf("get missing wallet metaforo user error: %+v", err)
		return nil, err
	}

	mfUserIdProfile := make(map[int]*metaforo.UserDetailResponseForProfileAPI)
	for _, mfUser := range missingWalletMetaforoUser {
		mfUserData, err := metaforo.UserDetail(mfUser.MetaforoUserId)
		if err != nil {
			log.Error().Msgf("get metaforo user detail error: %+v", err)
			return nil, err
		}

		mfUserIdProfile[mfUserData.User.Id] = mfUserData
		time.Sleep(time.Millisecond * 100)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for userId, profileData := range mfUserIdProfile {
			metaforoUser := model.MetaforoUser{
				MetaforoUserId: userId,
				UserWallet:     common.FormatUserWallet(profileData.User.Web3PublicKey),
			}
			mfUserTx := tx.Where(&model.MetaforoUser{MetaforoUserId: userId}).Find(&metaforoUser)

			if mfUserTx.Error != nil {
				log.Error().Msgf("get metaforo user record error: %+v", mfUserTx.Error)
				return mfUserTx.Error
			} else if mfUserTx.RowsAffected == 0 {
				if err = tx.Create(&metaforoUser).Error; err != nil {
					log.Error().Msgf("create metaforo user record error: %+v", err)
					return err
				}
			} else if mfUserTx.RowsAffected == 1 {
				if err = tx.Updates(&metaforoUser).Error; err != nil {
					log.Error().Msgf("update metaforo user record error: %+v", err)
					return err
				}
			} else {
				err = fmt.Errorf("unexpected rows affected: %d", mfUserTx.RowsAffected)
				log.Error().Msgf(err.Error())
				return err
			}

			// Create user record if not existing
			userRecord := model.User{Wallet: metaforoUser.UserWallet}
			db.Clauses(clause.OnConflict{DoNothing: true}).Model(&model.User{}).Where(&userRecord).Assign(model.User{
				CreateTs: model.GetCurrentUtcEpochSecond(),
				UpdateTs: model.GetCurrentUtcEpochSecond(),
				Avatar:   profileData.User.PhotoUrl,
				Name:     profileData.User.Username,
			}).FirstOrCreate(&userRecord)
		}
		return nil
	})

	userIdWalletMap := make(map[int]string)
	mfUserRecords := make([]*model.MetaforoUser, 0)
	err = db.Model(&model.MetaforoUser{}).Where("metaforo_user_id in ?", metaforoUserIds).Find(&mfUserRecords).Error
	if err != nil {
		log.Error().Msgf("get metaforo user records error: %+v", err)
		return nil, err
	}

	for _, mfUser := range mfUserRecords {
		userIdWalletMap[mfUser.MetaforoUserId] = mfUser.UserWallet
	}

	return userIdWalletMap, nil
}
