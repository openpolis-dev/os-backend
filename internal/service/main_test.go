package service

import (
	"context"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	postgresmodules "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
	// ===>1 start postgres container
	ctx := context.Background()
	postgresContainer, err := postgresmodules.RunContainer(ctx,
		testcontainers.WithImage("docker.io/postgres:16.2-alpine"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(5*time.Second)),
	)
	if err != nil {
		panic(err)
	}

	// ===>2 connect to postgres
	dsn, _ := postgresContainer.ConnectionString(ctx) // eg. `postgres://postgres:postgres@localhost:58209/postgres?`
	conn, err = gorm.Open(postgres.Open(dsn))

	// ===>3 auto migrate
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

	// ===>4 terminate postgres container
	err = postgresContainer.Terminate(ctx)
	if err != nil {
		panic(err)
	}
}

func truncateTables() {
	conn.Exec("TRUNCATE table sns_invite_codes,sns_invite_records")
	conn.Exec("TRUNCATE table application_audit_logs,applications,app_bundle_audit_logs,app_bundles,seasons")
}
