package storage

import (
	"log"
	"os"
	"time"

	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

var gormDB *gorm.DB

// InitGormDB inits gorm database connector
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
func SeedDbRecords() {
	prjTz := time.FixedZone("UTF+8", int((8 * time.Hour).Seconds()))
	// Note: S3 end is changed since the testing for node calc is not finished yet
	seasons := []model.Season{
		{
			Name:    "S0",
			Idx:     0,
			StartAt: time.Date(2022, 9, 26, 0, 0, 0, 0, prjTz).Unix(),
			EndAt:   time.Date(2022, 11, 3, 0, 0, 0, 0, prjTz).Unix(),
		}, {
			Name:    "S1",
			Idx:     1,
			StartAt: time.Date(2022, 11, 3, 0, 0, 0, 0, prjTz).Unix(),
			EndAt:   time.Date(2023, 2, 26, 0, 0, 0, 0, prjTz).Unix(),
		}, {
			Name:    "S2",
			Idx:     2,
			StartAt: time.Date(2023, 2, 28, 0, 0, 0, 0, prjTz).Unix(),
			EndAt:   time.Date(2023, 6, 2, 0, 0, 0, 0, prjTz).Unix(),
		}, {
			Name:    "S3",
			Idx:     3,
			StartAt: time.Date(2023, 6, 3, 0, 0, 0, 0, prjTz).Unix(),
			EndAt:   time.Date(2023, 9, 5, 0, 0, 0, 0, prjTz).Unix(),
		}, {
			Name:    "S4",
			Idx:     4,
			StartAt: time.Date(2023, 9, 6, 0, 0, 0, 0, prjTz).Unix(),
			EndAt:   time.Date(2023, 12, 1, 0, 0, 0, 0, prjTz).Unix(),
			//}, {
			//	Name:    "S5",
			//	Idx:     5,
			//	StartAt: time.Date(2023, 11, 2, 0, 0, 0, 0, prjTz).Unix(),
			//	EndAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, prjTz).Unix(),
		},
	}
	// Do nothing on conflict
	err := gormDB.Clauses(clause.OnConflict{
		DoNothing: true,
	}).Create(&seasons).Error

	if err != nil {
		panic("failed to seed database")
	}
}

func GetGormDB() *gorm.DB {
	return gormDB
}
