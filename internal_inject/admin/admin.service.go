package admin

import (
	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"
)

type AdminService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}
