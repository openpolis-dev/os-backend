package model

import (
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type User struct {
	ID             uint   `json:"id" gorm:"primaryKey"`
	Wallet         string `json:"wallet"`
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Email          string `json:"email"`
	Wechat         string `json:"wechat"`
	DiscordProfile string `json:"discord_profile"`
	TwitterProfile string `json:"twitter_profile"`
	GoogleProfile  string `json:"google_profile"`

	Mirror string `json:"mirror"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type userModel struct{}

var UserModel userModel

func (*userModel) CreateOrUpdate(db *gorm.DB, user *User) error {
	tx := db.Save(user)
	return tx.Error
}

func (*userModel) Detail(db *gorm.DB, wallet string) (*User, error) {
	querySeg := db.Where("wallet = ?", wallet)
	return gormfind.Row[User](querySeg)
}

func (*userModel) List(db *gorm.DB, wallets []string) ([]*User, error) {
	querySeg := db.Where("wallet IN (?)", wallets)
	return gormfind.Rows[User](querySeg, nil)
}
