package model

import (
	"fmt"
	"strings"

	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type Project struct {
	gorm.Model

	Logo      string   `json:"logo"`
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Sponsors  []string `json:"sponsors" gorm:"serializer:json"`
	Members   []string `json:"members" gorm:"serializer:json"`
	Proposals []string `json:"proposals" gorm:"serializer:json"`
}

type projectModel struct{}

var ProjectModel projectModel

func (*projectModel) CreateOrUpdate(db *gorm.DB, proj *Project) error {
	tx := db.Save(proj)
	return tx.Error
}

func (*projectModel) Detail(db *gorm.DB, id uint) (*Project, error) {
	querySeg := db.Where("id = ?", id)
	return gormfind.Row[Project](querySeg)
}

func (*projectModel) List(db *gorm.DB, status string, page *gormfind.Page) ([]*Project, error) {
	querySeg := db.Table("projects")
	if status != "" {
		querySeg.Where("status = ?", status)
	}
	return gormfind.Rows[Project](querySeg, page)
}

func (*projectModel) ListBySponsorOrMember(db *gorm.DB, wallet string, page *gormfind.Page) ([]*Project, error) {
	w := fmt.Sprintf("%%\"%s\"%%", strings.ToLower(wallet)) // value is: `%"0x123"%`
	querySeg := db.Table("projects").Where("sponsors LIKE ?", w).Or("members LIKE ?", w)
	return gormfind.Rows[Project](querySeg, page)
}
