package service

import (
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	wallet1 = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	wallet2 = "0x70997970C51812dc3A010C7d01b50e0d17dc79C8"
	wallet3 = "0x70997970C51812dc3A010C7d01b50e0d17dc1111"
	wallet4 = "0x70997970C51812dc3A010C7d01b50e0d17dc2222"
	wallet5 = "0x70997970C51812dc3A010C7d01b50e0d17dc3333"
)

var conn *gorm.DB

func TestMain(m *testing.M) {
	var err error
	conn, err = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags),
			logger.Config{
				SlowThreshold: 20 * time.Millisecond,
				LogLevel:      logger.Silent,
			},
		),
	})
	if err != nil {
		panic(err)
	}
	err = conn.AutoMigrate(
		&model.SnsInviteCode{},
		&model.SnsInviteRecord{},
		&model.Season{},
		&model.AppBundle{},
		&model.AppBundleAuditLog{},
		&model.Application{},
		&model.ApplicationAuditLog{},
	)
	if err != nil {
		panic(err)
	}

	m.Run()
}

func truncateTable(tableName string) {
	conn.Exec(fmt.Sprintf("TRUNCATE table %s", tableName))
}
