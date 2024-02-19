package db_agent

import (
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

type DbAgent struct {
	db *gorm.DB
}

var dbAgent *DbAgent

func GetDbAgent() *DbAgent {
	if dbAgent == nil {
		dbAgent = &DbAgent{
			db: storage.GetGormDB(),
		}
	}

	return dbAgent
}
