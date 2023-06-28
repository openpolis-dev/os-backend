package model_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

var db *gorm.DB

const (
	aliceWallet = "0x_alice_wallet"
	bobWallet   = "0x_bob_wallet"
	carolWallet = "0x_carol_wallet"
	daveWallet  = "0x_dave_wallet"

	token1Type = model.BudgetTypeCredit
	token1Name = "TTT"
	token2Type = model.BudgetTypeToken
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
}

var openProject, pendingCloseProject, closedProject *model.Project

var _ = BeforeSuite(func() {
	db, _ = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
})

func TestModel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Model Suite")
}
