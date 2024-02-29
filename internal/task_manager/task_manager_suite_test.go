package task_manager_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTaskManager(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("skip test without TEST_DATABASE_DSN")
	}

	RegisterFailHandler(Fail)
	RunSpecs(t, "TaskManager Suite")
}

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
	&model.SystemVariable{},
}

var db *gorm.DB
var _ = BeforeSuite(func() {
	dsn := os.Getenv("TEST_DATABASE_DSN")
	debugFlag := os.Getenv("TEST_DATABASE_DEBUG")
	logLv := logger.Error
	if debugFlag != "" {
		logLv = logger.Info
	}
	db, _ = storage.BuildGormClient("pg", dsn, logLv)
	storage.InitGormDB(dsn, "postgres")
	storage.InitCache()
})
