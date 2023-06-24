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

func (*guildModel) List(db *gorm.DB, status string, page *gormfind.Page) (data []*Guild, total int64, err error) {
	querySeg := db.Table("guilds")
	if status != "" {
		querySeg.Where("status = ?", status)
	}

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
