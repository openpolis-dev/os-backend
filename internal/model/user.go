package model

import (
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type User struct {
	ID             uint   `json:"id" gorm:"primaryKey"`
	Wallet         string `json:"wallet" gorm:"type:varchar(256);uniqueIndex"`
	Name           string `json:"name"`
	Bio            string `json:"bio"`
	Avatar         string `json:"avatar"`
	Email          string `json:"email"`
	Wechat         string `json:"wechat"`
	DiscordProfile string `json:"discord_profile"`
	TwitterProfile string `json:"twitter_profile"`
	GoogleProfile  string `json:"google_profile"`

	Mirror string `json:"mirror"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Assets []*UserAssetRecord `json:"assets" gorm:"foreignKey:UserWallet;references:Wallet"`
}

type userModel struct{}

var UserModel userModel

func (*userModel) CreateOrUpdate(db *gorm.DB, user *User) error {
	return db.Save(user).Error
}

func (*userModel) Detail(db *gorm.DB, wallet string) (*User, error) {
	querySeg := db.Preload("Assets").Where("wallet = ?", wallet)
	return gormfind.Row[User](querySeg)
}

func (*userModel) List(db *gorm.DB, wallets []string) ([]*User, error) {
	querySeg := db.Preload("Assets").Where("wallet IN (?)", wallets)
	return gormfind.Rows[User](querySeg, nil)
}

// TryGetUsername try to get username of passed in wallet address, and return "" if no user record found
func (*userModel) TryGetUsername(db *gorm.DB, wallet string) (string, error) {
	querySeg := db.Where("wallet = ?", wallet)
	user, err := gormfind.Row[User](querySeg)
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", err
	}

	if user != nil {
		return user.Name, nil
	} else {
		return "", nil
	}
}
