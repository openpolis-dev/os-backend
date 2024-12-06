package seeauth_inject

import (
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type UserNonce struct {
	ID        uint   `json:"id" gorm:"primaryKey"`
	Wallet    string `json:"wallet" gorm:"type:varchar(256);index"`
	Nonce     string `json:"nonce"`
	RefreshAt int64  `json:"refresh_at"`
}

type userNonceModel struct{}

var UserNonceModel userNonceModel

func (*userNonceModel) CreateOrUpdate(db *gorm.DB, userNonce *UserNonce) error {
	return db.Save(userNonce).Error
}

func (*userNonceModel) Detail(db *gorm.DB, wallet string) (*UserNonce, error) {
	querySeg := db.Where("wallet = ?", wallet)
	return gormfind.Row[UserNonce](querySeg)
}

func (*userNonceModel) RecentNonce(db *gorm.DB, wallet string, timeout int64) (*UserNonce, error) {
	now := time.Now().UnixMilli()
	querySeg := db.Where("wallet = ?", wallet).Where("refresh_at BETWEEN ? AND ?", now-timeout, now)
	return gormfind.Row[UserNonce](querySeg)
}
