package model

import (
	"time"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type Push struct {
	ID            uint   `json:"id" gorm:"primaryKey"`
	CreatorWallet string `json:"creator_wallet" gorm:"type:varchar(256)"`

	// TODO support multi language
	Title   string `json:"title"`
	Content string `json:"content"`

	// TODO support multi type, custom(JumpURL), xx(yy,zz)
	JumpURL string `json:"jump_url"`

	// TODO: Verify whether this file need to be changed to epoch second
	PushDate time.Time `json:"push_date"`
	Status   int       `json:"status"`

	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`

	CreateTs int64 `json:"create_ts" gorm:"index"`
	UpdateTs int64 `json:"update_ts" gorm:"index"`
}

type pushModel struct{}

var PushModel pushModel

func (*pushModel) CreateOrUpdate(db *gorm.DB, push *Push) error {
	return db.Save(push).Error
}

func (*pushModel) Detail(db *gorm.DB, id uint) (*Push, error) {
	querySeg := db.Where("id = ?", id)
	return gormfind.Row[Push](querySeg)
}

func (*pushModel) List(db *gorm.DB, status *int, page *gormfind.Page) (data []*Push, total int64, err error) {
	querySeg := db.Table("pushes")
	if status != nil {
		querySeg.Where("status = ?", status)
	}

	total, err = gormfind.Count(querySeg)
	if err != nil {
		return
	}

	data, err = QueryRows[Push](querySeg, page)
	if err != nil {
		return
	}

	return data, total, nil
}
