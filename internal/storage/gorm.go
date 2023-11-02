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
func SeedDbRecords(db *gorm.DB) error {
	prjTz := time.FixedZone("UTF+8", int((8 * time.Hour).Seconds()))
	seasons := []model.Season{
		{
			Name:    "S0",
			StartAt: time.Date(1970, 1, 1, 0, 0, 0, 0, prjTz),
			EndAt:   time.Date(2023, 2, 28, 0, 0, 0, 0, prjTz),
		}, {
			Name:    "S1",
			StartAt: time.Date(2023, 3, 1, 0, 0, 0, 0, prjTz),
			EndAt:   time.Date(2023, 5, 30, 0, 0, 0, 0, prjTz),
		}, {
			Name:    "S2",
			StartAt: time.Date(2023, 6, 1, 0, 0, 0, 0, prjTz),
			EndAt:   time.Date(2023, 8, 31, 0, 0, 0, 0, prjTz),
		}, {
			Name:    "S3",
			StartAt: time.Date(2023, 9, 1, 0, 0, 0, 0, prjTz),
			EndAt:   time.Date(2023, 11, 30, 0, 0, 0, 0, prjTz),
		}, {
			Name:    "S4",
			StartAt: time.Date(2023, 12, 1, 0, 0, 0, 0, prjTz),
			EndAt:   time.Date(2024, 2, 28, 0, 0, 0, 0, prjTz),
		},
	}
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(&seasons).Error

	if err != nil {
		return err
	}
	return nil
}

func GetGormDB() *gorm.DB {
	return gormDB
}
