package model

import (
	"errors"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type (
	SnsInviteCode struct {
		gorm.Model
		UserWallet string `json:"user_wallet" gorm:"type:varchar(256)"`
		InviteCode string `json:"invite_code" gorm:"uniqueIndex"` // must unique
	}

	SnsInviteRecord struct {
		gorm.Model
		InviteCode        string          `json:"invite_code"`
		InviteUserWallet  string          `json:"invite_user_wallet" gorm:"type:varchar(256)"`
		InviteeUserWallet string          `json:"invitee_user_wallet" gorm:"type:varchar(256)"`
		Verified          bool            `json:"verified"`
		SCRRewards        decimal.Decimal `json:"scr_rewards"`
	}
)

const (
	TableSnsInviteCode   = "sns_invite_codes"
	TableSnsInviteRecord = "sns_invite_records"
)

type snsInviteModel struct{}

var SnsInviteModel snsInviteModel

func (*snsInviteModel) CreateSnsInviteCode(db *gorm.DB, snsInviteCode *SnsInviteCode) error {
	return db.Create(snsInviteCode).Error
}

func (*snsInviteModel) FindInviteCodeByUserWallet(db *gorm.DB, userWallet string) (snsInviteCode *SnsInviteCode, err error) {
	err = db.Table(TableSnsInviteCode).Where("user_wallet = ?", userWallet).First(&snsInviteCode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return
}

func (*snsInviteModel) FindInviteCodeByInviteCode(db *gorm.DB, inviteCode string) (snsInviteCode *SnsInviteCode, err error) {
	err = db.Table(TableSnsInviteCode).Where("invite_code = ?", inviteCode).First(&snsInviteCode).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return
}

// ------ ------ ------ ------ ------ ------ ------ ------ ------

func (*snsInviteModel) CreateSnsInviteRecord(db *gorm.DB, snsInviteRecord *SnsInviteRecord) error {
	return db.Create(snsInviteRecord).Error
}

func (*snsInviteModel) FindInviteRecordByInviteUserWallet(db *gorm.DB, inviteUserWallet string) (rows []*SnsInviteRecord, err error) {
	err = db.Table(TableSnsInviteRecord).Where("invite_user_wallet = ?", inviteUserWallet).Find(&rows).Error
	return
}

func (*snsInviteModel) FindInviteRecordByInviteUserWalletAndVerified(db *gorm.DB, inviteUserWallet string) (rows []*SnsInviteRecord, err error) {
	err = db.Table(TableSnsInviteRecord).Where("invite_user_wallet = ? AND verified = ?", inviteUserWallet, true).Find(&rows).Error
	return
}

func (*snsInviteModel) FindInviteRecordByInviteeUserWallet(db *gorm.DB, inviteeUserWallet string) (row *SnsInviteRecord, err error) {
	err = db.Table(TableSnsInviteRecord).Where("invitee_user_wallet = ?", inviteeUserWallet).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return
}

// ------ ------ ------

func (*snsInviteModel) FindInviteRecordOfUnVerified(db *gorm.DB) (rows []*SnsInviteRecord, err error) {
	err = db.Table(TableSnsInviteRecord).Where("verified = ?", false).Find(&rows).Error
	return
}

func (*snsInviteModel) UpdateInviteRecordVerified(db *gorm.DB, inviteeUserWallet string) error {
	return db.Table(TableSnsInviteRecord).Where("invitee_user_wallet = ?", inviteeUserWallet).Update("verified", true).Error
}
