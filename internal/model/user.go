package model

import (
	"errors"
	"time"

	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/sdk"
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
	GithubProfile  string `json:"github_profile"`

	Mirror string `json:"mirror"`

	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`

	CreateTs int64 `json:"create_ts" gorm:"index"`
	UpdateTs int64 `json:"update_ts" gorm:"index"`

	Assets []*UserAssetRecord `json:"assets" gorm:"foreignKey:UserWallet;references:Wallet"`
}

type userModel struct{}

var UserModel userModel

func (*userModel) CreateOrUpdate(db *gorm.DB, user *User) error {
	user.UpdateTs = GetCurrentUtcEpochSecond()
	user.UpdatedAt = time.Now().In(internal.ProjectTimezone)

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
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	if user != nil {
		return user.Name, nil
	} else {
		return "", nil
	}
}

func (u *User) BuildSppUpdateProfilePayload() *sdk.SppUpdateProfileRequest {
	sppReq := &sdk.SppUpdateProfileRequest{
		Nickname:       u.Name,
		Bio:            u.Bio,
		Avatar:         u.Avatar,
		Email:          u.Email,
		SocialAccounts: []sdk.ProfileSocialAccount{},
	}

	if u.Wechat != "" {
		sppReq.SocialAccounts = append(sppReq.SocialAccounts, sdk.ProfileSocialAccount{
			Network:  "wechat",
			Identity: u.Wechat,
			Verified: false,
		})
	}
	if u.DiscordProfile != "" {
		sppReq.SocialAccounts = append(sppReq.SocialAccounts, sdk.ProfileSocialAccount{
			Network:  "discord",
			Identity: u.DiscordProfile,
			Verified: false,
		})
	}

	if u.TwitterProfile != "" {
		sppReq.SocialAccounts = append(sppReq.SocialAccounts, sdk.ProfileSocialAccount{
			Network:  "twitter",
			Identity: u.TwitterProfile,
			Verified: false,
		})
	}
	if u.Mirror != "" {
		sppReq.SocialAccounts = append(sppReq.SocialAccounts, sdk.ProfileSocialAccount{
			Network:  "mirror",
			Identity: u.Mirror,
			Verified: false,
		})
	}

	if u.GoogleProfile != "" && u.Email == "" {
		u.Email = u.GoogleProfile
	}

	if u.GithubProfile != "" {
		sppReq.SocialAccounts = append(sppReq.SocialAccounts, sdk.ProfileSocialAccount{
			Network:  "github",
			Identity: u.GithubProfile,
			Verified: false,
		})
	}

	return sppReq
}
