package publicity_inject

import (
	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"
)

type PublicityService struct {
	// inject
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}
