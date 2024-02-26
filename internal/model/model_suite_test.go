package model_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

const (
	aliceWallet = "0xB5238ed5a631895024d5dD91EbC4361b59276455"
	bobWallet   = "0x74d0398ad57D0Ccd484f9dd83bDa094b0708be3d"
	carolWallet = "0x5D6d5701e836108a0f3898f5E672dB30Aec5695a"
	daveWallet  = "0x55b8D71B7D7b267C4dD97AE36F1ACA7377784758"

	token1Name = "TTT"
	token2Name = "AAT"
	token3Name = "42T"
)

var tables = []any{
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
	&model.Season{},
}

var (
	token1Budget            = decimal.NewFromInt(100)
	token2Budget            = decimal.NewFromInt(200)
	token1RewardAmount, _   = decimal.NewFromString("9.9")
	token1WithdrawAmount, _ = decimal.NewFromString("42.24")
	token1DepositAmount, _  = decimal.NewFromString("24.42")
)

var openProject, pendingCloseProject, closedProject *model.Project

var _ = BeforeSuite(func() {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	debugFlag := os.Getenv("TEST_DATABASE_DEBUG")
	if dsn == "" {
		dsn = "postgres://localhost:5432/os_backend_auto_test?sslmode=disable"
	}
	logLv := logger.Error
	if debugFlag != "" {
		logLv = logger.Info
	}
	db, _ = storage.BuildGormClient("pg", dsn, logLv)
	storage.InitCache()
})

func TestModel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Model Suite")
}
