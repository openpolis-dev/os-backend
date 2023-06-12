package model

import (
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

type Project struct {
	gorm.Model

	Logo      string   `json:"logo"`
	Name      string   `json:"name"`
	Status    string   `json:"status"`
	Sponsors  []string `json:"sponsors"`
	Members   []string `json:"members"`
	Proposals []string `json:"proposals"`
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
	querySeg := db.Table("projects").Where("? = ANY(sponsors)", wallet).Or("? = ANY(members)", wallet)
	return gormfind.Rows[Project](querySeg, page)
}
