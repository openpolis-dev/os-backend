package push_inject

import (
	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"
)

type PushService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}
