package db_agent_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDbAgent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DbAgent Suite")
}

var db *gorm.DB
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
