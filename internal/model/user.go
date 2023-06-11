package model

import (
	"strings"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model

	Wallet    string `json:"wallet"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	DiscordID string `json:"discordID"` // discord_id
	TwitterID string `json:"twitterID"` // twitter_id
}

type userModel struct{}

var UserModel userModel

func (*userModel) CreateOrUpdate(db *gorm.DB, user *User) error {
	tx := db.Save(user)
	return tx.Error
}

func (*userModel) Detail(db *gorm.DB, wallet string) (*User, error) {
	querySeg := db.Where("wallet = ?", strings.ToLower(wallet))
	return gormfind.Row[User](querySeg)
}

func (*userModel) List(db *gorm.DB, wallets []string) ([]*User, error) {
	querySeg := db.Where("wallet IN (?)", wallets)
	return gormfind.Rows[User](querySeg, nil)
}
