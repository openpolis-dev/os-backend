package model

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type GuildBudget struct {
	ID      uint `json:"id" gorm:"primaryKey"`
	GuildID uint `json:"guild_id"` // guild_id

	AssetName    string          `json:"asset_name"`
	TotalAmount  decimal.Decimal `json:"total_amount" sql:"type:decimal(20,8);"`  // total_amount = used_amount + remain_amount
	UsedAmount   decimal.Decimal `json:"used_amount" sql:"type:decimal(20,8);"`   // used_amount
	RemainAmount decimal.Decimal `json:"remain_amount" sql:"type:decimal(20,8);"` // remain_amount

	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
	CreateTs  int64     `json:"create_ts" gorm:"index"`
	UpdateTs  int64     `json:"update_ts" gorm:"index"`
}

type guildBudgetModel struct{}

var GuildBudgetModel guildBudgetModel

func (*guildBudgetModel) Create(db *gorm.DB, budgets []*GuildBudget) error {
	tx := db.Create(budgets)
	return tx.Error
}

func (*guildBudgetModel) Update(db *gorm.DB, budget *GuildBudget) error {
	return db.Save(budget).Error
}

func (*guildBudgetModel) Detail(db *gorm.DB, id uint) (*GuildBudget, error) {
	querySeg := db.Where("id = ?", id)
	return gormfind.Row[GuildBudget](querySeg)
}

func (*guildBudgetModel) ListByGuildId(db *gorm.DB, guildID uint) ([]*GuildBudget, error) {
	querySeg := db.Where("guild_id = ?", guildID)
	return QueryRows[GuildBudget](querySeg, nil)
}

func (*guildBudgetModel) QueryByGuildIdAndAssetName(db *gorm.DB, guildID uint, assetName string) (*GuildBudget, error) {
	querySeg := db.Where("guild_id = ?", guildID).Where("name = ?", assetName)
	return gormfind.Row[GuildBudget](querySeg)
}

func (*guildBudgetModel) WithdrawSingleAsset(db *gorm.DB, guildId uint, assetName string, amount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := GuildBudgetModel.QueryByGuildIdAndAssetName(tx, guildId, assetName)
		if err != nil {
			log.Error().Msgf("query guild %d budget %s error: %+v", guildId, assetName, err)
			return err
		}

		updateClause := map[string]any{
			"id": budgetRcd.ID,
		}

		if budgetRcd.RemainAmount.LessThan(amount) {
			err = fmt.Errorf("guild %d budget %s remain amount %s is less than request value %s", guildId, assetName, budgetRcd.RemainAmount.String(), amount.String())
			log.Error().Msgf(err.Error())
			return err
		} else {
			updateClause["used_amount"] = budgetRcd.UsedAmount.Add(amount)
			updateClause["remain_amount"] = budgetRcd.RemainAmount.Sub(amount)
		}

		return tx.Model(&budgetRcd).Updates(updateClause).Error
	})
}

func (*guildBudgetModel) DepositSingleAsset(db *gorm.DB, guildId uint, assetName string, amount decimal.Decimal) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := GuildBudgetModel.QueryByGuildIdAndAssetName(tx, guildId, assetName)
		if err != nil {
			log.Error().Msgf("query guild %d budget %s error: %+v", guildId, assetName, err)
			return err
		}

		updateClause := map[string]any{
			"id": budgetRcd.ID,
		}

		if budgetRcd.UsedAmount.LessThan(amount) {
			err = fmt.Errorf("guild %d budget %s used amount %s is less than request value %s", guildId, assetName, budgetRcd.UsedAmount.String(), amount.String())
			log.Error().Msgf(err.Error())
			return err
		} else {
			updateClause["used_amount"] = budgetRcd.UsedAmount.Sub(amount)
			updateClause["remain_amount"] = budgetRcd.RemainAmount.Add(amount)
		}

		return tx.Model(&budgetRcd).Updates(updateClause).Error
	})
}
