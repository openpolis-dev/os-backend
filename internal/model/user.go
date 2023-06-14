package model

import (
	"strings"
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model

	ID             uint   `json:"id"`
	Wallet         string `json:"wallet"`
	Name           string `json:"name"`
	Avatar         string `json:"avatar"`
	Email          string `json:"email"`
	DiscordProfile string `json:"discordProfile"` // discord_profile
	TwitterProfile string `json:"twitterProfile"` // twitter_profile
	GoogleProfile  string `json:"GoogleProfile"`  // google_profile

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
	querySeg := db.Where("wallet = ?", strings.ToLower(wallet))
	return gormfind.Row[User](querySeg)
}

func (*userModel) List(db *gorm.DB, wallets []string) ([]*User, error) {
	querySeg := db.Where("wallet IN (?)", wallets)
	return gormfind.Rows[User](querySeg, nil)
}
