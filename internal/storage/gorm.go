package storage

import (
	"log"
	"os"
	"time"

	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gormDB *gorm.DB

func InitGormDB(dsn string) {
	// default logger config: https://github.com/go-gorm/gorm/blob/master/logger/logger.go#L74
	dbLogger := logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             20 * time.Millisecond,
		LogLevel:                  logger.Info,
		IgnoreRecordNotFoundError: false,
		Colorful:                  true,
	})

	var err error
	gormDB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: dbLogger})
	if err != nil {
		panic("failed to connect database")
	}

	// Migrate the schema
	err = gormDB.AutoMigrate(
		&model.User{},
		&model.UserNonce{},
		&model.UserAssetRecord{},
		&model.Project{},
		&model.ProjectBudget{},
		&model.Guild{},
		&model.GuildBudget{},
		&model.Application{},
		&model.ApplicationAuditLog{},
		&model.TreasuryAsset{},
		&model.TreasuryDetailedRecord{},
		&model.TreasuryAuditLog{},
	)
	if err != nil {
		panic("failed to migrate tables")
	}
}

func GetGormDB() *gorm.DB {
	return gormDB
}
