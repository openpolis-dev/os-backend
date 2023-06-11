package model

import (
	"strings"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model

	Wallet   string `json:"wallet"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Avatar   string `json:"avatar"`
}

type userModel struct{}

var UserModel userModel

func (*userModel) CreateOrUpdate(db *gorm.DB, user *User) error {
	tx := db.Save(user)
	return tx.Error
}

func (*userModel) User(db *gorm.DB, wallet string) (*User, error) {
	querySeg := db.Where("wallet = ?", strings.ToLower(wallet))
	return gormfind.Row[User](querySeg)
}
