package model

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type GuildBudget struct {
	ID           uint            `json:"id" gorm:"primaryKey"`
	GuildID      uint            `json:"guild_id"` // guild_id
	Name         string          `json:"name"`
	Type         BudgetType      `json:"type"`                                   // budget type, credit or token
	TotalAmount  decimal.Decimal `json:"total_amount" sql:"type:decimal(20,8);"` // total_amount
	UsedAmount   decimal.Decimal `json:"used_amount" sql:"type:decimal(20,8);"`  // used_amount
	RemainAmount decimal.Decimal `json:"remain_amount sql:"type:decimal(20,8);"` // remain_amount
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
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
	return gormfind.Rows[GuildBudget](querySeg, nil)
}

func (*guildBudgetModel) QueryByGuildIdAndBudgetType(db *gorm.DB, guildID uint, budgetType BudgetType) (*GuildBudget, error) {
	querySeg := db.Where("guild_id = ?", guildID).Where("type = ?", budgetType)
	return gormfind.Row[GuildBudget](querySeg)
}
