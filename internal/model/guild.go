package model

import (
	"fmt"
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type Guild struct {
	ID        uint     `json:"id" gorm:"primaryKey"`
	Logo      string   `json:"logo"`
	Name      string   `json:"name"`
	Sponsors  []string `json:"sponsors" gorm:"serializer:json"`
	Members   []string `json:"members" gorm:"serializer:json"`
	Proposals []string `json:"proposals" gorm:"serializer:json"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type guildModel struct{}

var GuildModel guildModel

func (*guildModel) CreateOrUpdate(db *gorm.DB, proj *Guild) error {
	tx := db.Save(proj)
	return tx.Error
}

func (*guildModel) Detail(db *gorm.DB, id uint) (*Guild, error) {
	querySeg := db.Where("id = ?", id)
	return gormfind.Row[Guild](querySeg)
}

func (*guildModel) List(db *gorm.DB, page *gormfind.Page) (data []*Guild, total int64, err error) {
	querySeg := db.Table("guilds")

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}

	data, err = gormfind.Rows[Guild](querySeg, page)
	if err != nil {
		return
	}

	return data, total, nil
}

func (*guildModel) ListBySponsorOrMember(db *gorm.DB, wallet string, page *gormfind.Page) (data []*Guild, total int64, err error) {
	w := fmt.Sprintf("%%\"%s\"%%", wallet) // value is: `%"0x123"%`
	querySeg := db.Table("guilds").Where("sponsors LIKE ?", w).Or("members LIKE ?", w)

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}
	data, err = gormfind.Rows[Guild](querySeg, page)
	if err != nil {
		return
	}
	return data, total, nil
}

// SetBudget set budget record directly, but only total amount is allowed to set directly
func (*guildModel) SetBudget(db *gorm.DB, guildId uint, budgetType BudgetType, assertName string, totalAmount uint64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRecord, err := GuildBudgetModel.QueryByGuildIdAndBudgetType(tx, guildId, budgetType)
		if err != nil {
			return err
		}

		if budgetRecord == nil {
			budgetRecord = &GuildBudget{
				GuildID:      guildId,
				Name:         assertName,
				Type:         budgetType,
				TotalAmount:  totalAmount,
				RemainAmount: totalAmount,
			}
		} else {
			budgetRecord.TotalAmount = totalAmount
		}

		return GuildBudgetModel.Update(tx, budgetRecord)
	})
}

// TODO: Some budget related logics can be merged

func (*guildModel) WithdrawBudget(db *gorm.DB, guildId uint, budgetType BudgetType, tokenName string, tokenAmount uint64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := GuildBudgetModel.QueryByGuildIdAndBudgetType(tx, guildId, budgetType)
		if err != nil {
			return err
		}

		if budgetRcd == nil {
			return fmt.Errorf("guild %d has no budget record with asset %s", guildId, tokenName)
		}

		if budgetRcd.RemainAmount < tokenAmount {
			return fmt.Errorf("guild %d has insufficient budget record with asset %s", guildId, tokenName)
		}

		budgetRcd.RemainAmount -= tokenAmount
		return tx.Save(budgetRcd).Error
	})
}

// DepositBudget deposits budget back to guild, e.g. application for reward has been rejected
func (*guildModel) DepositBudget(db *gorm.DB, guildId uint, budgetType BudgetType, tokenName string, tokenAmount uint64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		budgetRcd, err := GuildBudgetModel.QueryByGuildIdAndBudgetType(tx, guildId, budgetType)
		if err != nil {
			return err
		}

		if budgetRcd == nil {
			return tx.Save(&GuildBudget{
				GuildID:      guildId,
				Name:         tokenName,
				Type:         budgetType,
				TotalAmount:  tokenAmount,
				RemainAmount: tokenAmount,
			}).Error
		} else {
			budgetRcd.RemainAmount += tokenAmount
			return tx.Save(budgetRcd).Error
		}
	})
}
