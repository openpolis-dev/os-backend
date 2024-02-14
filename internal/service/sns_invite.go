package service

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/shopspring/decimal"
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
	if r != nil {
		return ErrInvalidInviteCode
	}

	d, err := model.SnsInviteModel.FindInviteCodeByInviteCode(db, inviteCode)
	if err != nil {
		return err
	}
	if d == nil {
		return ErrAlreadyInvited
	}

	inviteRecord := &model.SnsInviteRecord{
		InviteCode:        inviteCode,
		InviteUserWallet:  d.UserWallet,
		InviteeUserWallet: inviteeUserWallet,
		SCRRewards:        decimal.NewFromInt(100),
	}
	err = model.SnsInviteModel.CreateSnsInviteRecord(db, inviteRecord)
	if err != nil {
		return err
	}

	return nil
}

func GetMySnsInviteRewards(db *gorm.DB, inviteUserWallet string) (count int, total decimal.Decimal, err error) {
	rows, err := model.SnsInviteModel.FindInviteRecordByInviteUserWallet(db, inviteUserWallet)
	if err != nil {
		return 0, decimal.Zero, err
	}

	for _, row := range rows {
		total = total.Add(row.SCRRewards)
	}
	return len(rows), total, nil
}
