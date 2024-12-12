package snsinvite_inject

import (
	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"
)

type SnsInviteService struct {
	// inject

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}
