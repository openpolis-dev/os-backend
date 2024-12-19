package seasons_inject

import (
	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"
)

type SeasonsService struct {
	// inject
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}
