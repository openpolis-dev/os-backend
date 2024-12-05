package user_inject

import (
	"errors"
	"fmt"
	"time"

	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
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

type UserAssetRecord struct {
	ID               uint            `json:"id" gorm:"primaryKey"`
	UserWallet       string          `json:"user_wallet" gorm:"type:varchar(256)"`
	AssetName        string          `json:"asset_name" gorm:"type:varchar(64)"`          // asset name
	DealtAmount      decimal.Decimal `json:"dealt_amount" sql:"type:decimal(20,8);"`      // amount of asset that already dealt
	ProcessingAmount decimal.Decimal `json:"processing_amount" sql:"type:decimal(20,8);"` // amount of asset that still need confirmation
	CreatedAt        time.Time       `json:"-"`
	UpdatedAt        time.Time       `json:"-"`
	CreateTs         int64           `json:"create_ts" gorm:"index"`
	UpdateTs         int64           `json:"update_ts" gorm:"index"`
}

type userAssetRecordModel struct{}

var UserAssetRecordModel userAssetRecordModel

func (*userAssetRecordModel) FindWithUserWalletAndAssetProps(db *gorm.DB, userWallet string, assetName string) ([]*UserAssetRecord, error) {
	formattedUserWallet := common.FormatUserWallet(userWallet)

	// Create user record if not existing
	var r User
	userRslt := db.Where(User{Wallet: formattedUserWallet}).Attrs(User{
		CreatedAt: time.Now().In(internal.ProjectTimezone),
		UpdatedAt: time.Now().In(internal.ProjectTimezone),
		CreateTs:  model.GetCurrentUtcEpochSecond(),
		UpdateTs:  model.GetCurrentUtcEpochSecond(),
	}).FirstOrInit(&r)
	if userRslt.Error != nil {
		return nil, userRslt.Error
	} else if userRslt.RowsAffected == 0 {
		err := db.Save(&r).Error
		if err != nil {
			return nil, userRslt.Error
		}
	}

	querySeg := db.Where(&UserAssetRecord{UserWallet: formattedUserWallet, AssetName: assetName})
	return model.QueryRows[UserAssetRecord](querySeg, nil)
}

func (*userAssetRecordModel) CreateOrUpdate(db *gorm.DB, userWallet string, assetName string, processingAmount, dealtAmount decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, userWallet, assetName)
	if err != nil {
		return err
	}

	if len(assetRecords) > 1 {
		return fmt.Errorf("user %s has more than one record for asset %s, please contract admin", userWallet, assetName)
	}

	if len(assetRecords) == 0 {
		return db.Save(&UserAssetRecord{
			UserWallet:       common.FormatUserWallet(userWallet),
			AssetName:        assetName,
			DealtAmount:      dealtAmount,
			ProcessingAmount: processingAmount,
		}).Error
	} else {
		assetRecords[0].DealtAmount = assetRecords[0].DealtAmount.Add(dealtAmount)
		assetRecords[0].ProcessingAmount = assetRecords[0].ProcessingAmount.Add(processingAmount)
		return db.Save(assetRecords).Error
	}
}

// Rollback extracts processing and dealt amount from records
func (*userAssetRecordModel) Rollback(db *gorm.DB, userWallet string, assetName string, processingAmount, dealtAmount decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, common.FormatUserWallet(userWallet), assetName)
	if err != nil {
		return err
	}

	if (len(assetRecords) != 1) || (assetRecords[0].ProcessingAmount.Cmp(processingAmount) == 1) || (assetRecords[0].DealtAmount.Cmp(dealtAmount) == 1) {
		return fmt.Errorf("user %s has invalid record for asset %s, please contract admin", userWallet, assetName)
	}

	assetRecords[0].DealtAmount = assetRecords[0].DealtAmount.Sub(dealtAmount)
	assetRecords[0].ProcessingAmount = assetRecords[0].ProcessingAmount.Sub(processingAmount)

	return db.Save(assetRecords).Error
}

func (*userAssetRecordModel) CompleteAssetTransaction(db *gorm.DB, userWallet string, assetName string, amountToBeDealt decimal.Decimal) error {
	assetRecords, err := UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, common.FormatUserWallet(userWallet), assetName)
	if err != nil {
		return err
	}

	if (len(assetRecords) != 1) || (assetRecords[0].ProcessingAmount.Cmp(amountToBeDealt) == -1) {
		return fmt.Errorf("user %s has invalid record for asset %s, please contract admin", common.FormatUserWallet(userWallet), assetName)
	}

	assetRecords[0].DealtAmount = assetRecords[0].DealtAmount.Add(amountToBeDealt)
	assetRecords[0].ProcessingAmount = assetRecords[0].ProcessingAmount.Sub(amountToBeDealt)

	return db.Save(assetRecords).Error
}

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
	user.UpdateTs = model.GetCurrentUtcEpochSecond()
	user.UpdatedAt = time.Now().In(internal.ProjectTimezone)
	user.Wallet = common.FormatUserWallet(user.Wallet)

	return db.Save(user).Error
}

func (*userModel) Detail(db *gorm.DB, wallet string) (*User, error) {
	querySeg := db.Preload("Assets").Where("wallet = ?", common.FormatUserWallet(wallet))
	return gormfind.Row[User](querySeg)
}

func (*userModel) List(db *gorm.DB, wallets []string) ([]*User, error) {
	querySeg := db.Preload("Assets").Where("wallet IN (?)", lo.Map(wallets, func(wallet string, _ int) interface{} {
		return common.FormatUserWallet(wallet)
	}))
	return model.QueryRows[User](querySeg, nil)
}

// TryGetUsername try to get username of passed in wallet address, and return "" if no user record found
func (*userModel) TryGetUsername(db *gorm.DB, wallet string) (string, error) {
	querySeg := db.Where("wallet = ?", common.FormatUserWallet(wallet))
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
