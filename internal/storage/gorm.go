package storage

import (
	"fmt"
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

func BuildGormClient(dbSchema string, dsn string, logLevel logger.LogLevel) (*gorm.DB, error) {
	// default logger config: https://github.com/go-gorm/gorm/blob/master/logger/logger.go#L74
	dbLogger := logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		SlowThreshold:             20 * time.Millisecond,
		LogLevel:                  logLevel,
		IgnoreRecordNotFoundError: false,
		Colorful:                  true,
	})

	switch dbSchema {
	case "mysql":
		return gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: dbLogger})
	case "postgres", "pg":
		return gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: dbLogger})
	default:
		return nil, fmt.Errorf("unsupported db schema: %s, dsn: %s", dbSchema, dsn)
	}
}

// InitGormDB inits gorm database connector
func InitGormDB(dsn string, dbSchema string) {
	var err error
	gormDB, err = BuildGormClient(dbSchema, dsn, logger.Info)
	if err != nil {
		panic(fmt.Errorf("init gorm connection error: %+v", err))
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
