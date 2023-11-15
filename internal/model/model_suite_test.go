package model_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

var db *gorm.DB

const (
	aliceWallet = "0x_alice_wallet"
	bobWallet   = "0x_bob_wallet"
	carolWallet = "0x_carol_wallet"
	daveWallet  = "0x_dave_wallet"

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
	db, _ = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
})

func TestModel(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Model Suite")
}
