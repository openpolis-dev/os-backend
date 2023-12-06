package storage

import (
	"log"
	"os"
	"time"

	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gormDB *gorm.DB

// InitGormDB inits gorm database connector
func InitGormDB(dsn string, dbSchema string) {
	// default logger config: https://github.com/go-gorm/gorm/blob/master/logger/logger.go#L74
	dbLogger := logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             20 * time.Millisecond,
		LogLevel:                  logger.Info,
		IgnoreRecordNotFoundError: false,
		Colorful:                  true,
	})

	var err error
	switch dbSchema {
	case "mysql":
		gormDB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: dbLogger})
	case "postgres", "pg":
		gormDB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: dbLogger})
	default:
		panic("unsupported db schema")
	}
	if err != nil {
		panic("failed to connect database")
	}
}

// MigrateTables auto migrate models defined.
func MigrateTables() {
	// Migrate the schema
	err := gormDB.AutoMigrate(
		&model.User{},
		&model.UserNonce{},
		&model.UserAssetRecord{},
		&model.Project{},
		&model.ProjectBudget{},
		&model.Guild{},
		&model.GuildBudget{},
		&model.AppBundle{},
		&model.AppBundleAuditLog{},
		&model.Season{},
		&model.Application{},
		&model.ApplicationAuditLog{},
		&model.TreasuryAsset{},
		&model.TreasuryDetailedRecord{},
		&model.TreasuryAuditLog{},
		&model.Event{},
		&model.Push{},
	)
	if err != nil {
		panic("failed to migrate tables")
	}
}

// SeedDbRecords inits some const data records to database if not existing
// The seasons currently has initialized in the database so no seed is required here
func SeedDbRecords() {
}

func GetGormDB() *gorm.DB {
	return gormDB
}
