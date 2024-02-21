package service

import (
	"time"

	"github.com/Taoist-Labs/sns-go"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

func GetMySnsInviteCode(db *gorm.DB, userWallet string) (string, error) {
	inviteCode, err := model.SnsInviteModel.FindInviteCodeByUserWallet(db, userWallet)
	if err != nil {
		return "", err
	}
	if inviteCode == nil {
		code, err := gonanoid.New(14)
		if err != nil {
			return "", err
		}

		inviteCode = &model.SnsInviteCode{
			UserWallet: userWallet,
			InviteCode: code,
		}
		err = model.SnsInviteModel.CreateSnsInviteCode(db, inviteCode)
		if err != nil {
			return "", err
		}
	}

	return inviteCode.InviteCode, nil
}

// SnsInvitedBy function of `inviteeUserWallet` invited by someone with `inviteCode`
func SnsInvitedBy(db *gorm.DB, inviteCode, inviteeUserWallet string) error {
	r, err := model.SnsInviteModel.FindInviteRecordByInviteeUserWallet(db, inviteeUserWallet)
	if err != nil {
		return err
	}
	// record exists and has verified
	if r != nil && r.Verified {
		return ErrAlreadyInvited
	}

	d, err := model.SnsInviteModel.FindInviteCodeByInviteCode(db, inviteCode)
	if err != nil {
		return err
	}
	if d == nil {
		return ErrInvalidInviteCode
	}

	if r == nil {
		r = &model.SnsInviteRecord{
			InviteCode:        inviteCode,
			InviteUserWallet:  d.UserWallet,
			InviteeUserWallet: inviteeUserWallet,
			Verified:          false,
			SCRRewards:        decimal.NewFromInt(100),
		}
	} else {
		r.InviteCode = inviteCode
		r.InviteUserWallet = d.UserWallet
	}

	err = model.SnsInviteModel.CreateOrUpdateSnsInviteRecord(db, r)
	if err != nil {
		return err
	}

	return nil
}

func GetMySnsInviteRewards(db *gorm.DB, inviteUserWallet string) (count int, total decimal.Decimal, err error) {
	rows, err := model.SnsInviteModel.FindInviteRecordByInviteUserWalletAndVerified(db, inviteUserWallet)
	if err != nil {
		return 0, decimal.Zero, err
	}

	for _, row := range rows {
		total = total.Add(row.SCRRewards)
	}
	return len(rows), total, nil
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------

// CheckAndUpdateUnverifiedSnsInvite check and update unverified SNS invite
// TODO define a cron task to execute this function
func CheckAndUpdateUnverifiedSnsInvite(db *gorm.DB) error {
	rows, err := model.SnsInviteModel.FindInviteRecordOfUnVerified(db)
	if err != nil {
		return err
	}
	log.Debug().Msgf("-- CheckAndUpdateUnverifiedSnsInvite -- Rows: %d", len(rows))

	// get current season
	currentSeason, err := model.GetCurrentSeason(db)

	for i, row := range rows {
		n := sns.Name(row.InviteeUserWallet)
		log.Debug().Msgf(" -- -- [%d] InviteeUserWallet: %s, SNS: %s", i, row.InviteeUserWallet, n)
		if n != "" {
			inviteeUserWallet := common.FormatUserWallet(row.InviteeUserWallet)
			err = db.Transaction(func(tx *gorm.DB) error {
				// 1 update invite record's 'verified' to true
				err = model.SnsInviteModel.UpdateInviteRecordVerified(tx, row.InviteeUserWallet)
				if err != nil {
					return err
				}

				// 2 create application
				// save app bundle
				appBundle := model.AppBundle{
					Applicant:    inviteeUserWallet,
					EntityType:   "guild", // TODO use config file
					EntityId:     2,       // TODO
					SeasonId:     currentSeason.ID,
					State:        model.ApplicationStateApproved,
					ShadowRecord: false,
					CreateTs:     model.GetCurrentUtcEpochSecond(),
					UpdateTs:     model.GetCurrentUtcEpochSecond(),
					Type:         "NEW_REWARD",
				}
				if err = tx.Model(model.AppBundle{}).Create(&appBundle).Error; err != nil {
					return err
				}
				// save applications
				appBundle.AppRecords = []*model.Application{
					{
						Type:             model.ApplicationNewReward,
						Applicant:        "0x4564d5a8Bb409272F1FB4ae4c8b45fC0eaFd709D", // 申请人 // TODO
						State:            model.ApplicationStateApproved,
						CreatedAt:        time.Now().In(internal.ProjectTimezone),
						UpdatedAt:        time.Now().In(internal.ProjectTimezone),
						CreateTs:         model.GetCurrentUtcEpochSecond(),
						UpdateTs:         model.GetCurrentUtcEpochSecond(),
						DetailedType:     "邀请 SNS",             // 事项
						Comment:          "SNS invite rewards", // 备注
						AssetName:        "SCR",
						AssetAmount:      row.SCRRewards,
						TargetUserWallet: common.FormatUserWallet(row.InviteUserWallet),
						EntityType:       appBundle.EntityType,
						EntityId:         appBundle.EntityId,
						SeasonId:         currentSeason.ID,
						BundleId:         appBundle.ID,
					},
				}
				if err = tx.Save(&appBundle).Error; err != nil {
					return err
				}
				// save application audit log
				if err = tx.Model(model.ApplicationAuditLog{}).Create(&model.ApplicationAuditLog{
					ApplicationID: appBundle.AppRecords[0].ID,
					LogTs:         model.GetCurrentUtcEpochSecond(),
					Operation:     model.AuditActionNew,
					Operator:      inviteeUserWallet,
					PreState:      "",
					PostState:     model.ApplicationStateApproved,
				}).Error; err != nil {
					return err
				}
				// save app bundle audit log
				if err = tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
					AppBundleId: appBundle.ID,
					AppBundle:   appBundle,
					LogTs:       model.GetCurrentUtcEpochSecond(),
					Operation:   model.AuditActionNew,
					Operator:    inviteeUserWallet,
					PreState:    "",
					PostState:   model.ApplicationStateApproved,
					ExtraData:   "",
				}).Error; err != nil {
					return err
				}

				// TODO 3 send application to QuickAccounting
				// !! implements this after this branch merged

				return nil
			})

			if err != nil {
				log.Error().Msgf(" -- -- [%d] UpdateInviteRecordVerified error: %s", i, err)
			} else {
				log.Debug().Msgf(" -- -- [%d] UpdateInviteRecordVerified success", i)
			}
		}
	}

	return nil
}
