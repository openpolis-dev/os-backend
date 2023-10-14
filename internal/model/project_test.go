package model_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/model"
)

var _ = Describe("Project", func() {
	BeforeEach(func() {
		_ = db.AutoMigrate(tables...)

		db.Create(&model.User{Wallet: aliceWallet})
		db.Create(&model.User{Wallet: bobWallet})
		db.Create(&model.User{Wallet: carolWallet})
	})

	// After each `It` execution, drop tables
	AfterEach(func() {
		_ = db.Migrator().DropTable(tables...)
	})

	Describe("Validate project creation", func() {
		It("Can create project record successfully", func() {
			projectRecord := model.Project{
				Logo:     "logo",
				Name:     "test project",
				Status:   model.ProjectStatusOpen,
				Sponsors: []string{aliceWallet},
				Members:  []string{bobWallet},
			}
			err := model.ProjectModel.CreateOrUpdate(db, &projectRecord)
			Expect(err).To(BeNil())

			openPrjs, prjCnt, err := model.ProjectModel.List(db, string(model.ProjectStatusOpen), nil, false)
			Expect(err).To(BeNil())
			Expect(prjCnt).To(BeEquivalentTo(1))
			Expect(len(openPrjs)).To(Equal(1))

			pendingClosePrjs, prjCnt, err := model.ProjectModel.List(db, string(model.ProjectStatusPendingClose), nil, false)
			Expect(err).To(BeNil())
			Expect(prjCnt).To(BeEquivalentTo(0))
			Expect(len(pendingClosePrjs)).To(Equal(0))

			closedPrjs, prjCnt, err := model.ProjectModel.List(db, string(model.ProjectStatusClosed), nil, false)
			Expect(err).To(BeNil())
			Expect(prjCnt).To(BeEquivalentTo(0))
			Expect(len(closedPrjs)).To(Equal(0))

			alicePrjs, alicePrjCnt, err := model.ProjectModel.ListBySponsorOrMember(db, aliceWallet, nil)
			Expect(err).To(BeNil())
			Expect(alicePrjCnt).To(BeEquivalentTo(1))
			Expect(len(alicePrjs)).To(Equal(1))

			bobPrjs, bobPrjCnt, err := model.ProjectModel.ListBySponsorOrMember(db, bobWallet, nil)
			Expect(err).To(BeNil())
			Expect(bobPrjCnt).To(BeEquivalentTo(1))
			Expect(len(bobPrjs)).To(Equal(1))

			carolPrjs, carolPrjCnt, err := model.ProjectModel.ListBySponsorOrMember(db, carolWallet, nil)
			Expect(err).To(BeNil())
			Expect(carolPrjCnt).To(BeEquivalentTo(0))
			Expect(len(carolPrjs)).To(Equal(0))
		})
	})

	Describe("Budget related functions", func() {
		var prjRcd model.Project
		BeforeEach(func() {
			prjRcd = model.Project{
				Logo:     "logo",
				Name:     "test project",
				Status:   model.ProjectStatusOpen,
				Sponsors: []string{aliceWallet},
				Members:  []string{bobWallet},
			}
			_ = model.ProjectModel.CreateOrUpdate(db, &prjRcd)
		})

		It("can set budget correctly", func() {
			err := model.ProjectModel.SetBudget(db, prjRcd.ID, token1Type, token1Name, token1Budget)
			Expect(err).To(BeNil())

			budgetRecord, err := model.ProjectBudgetModel.QueryByProjectIdAndBudgetProps(db, prjRcd.ID, token1Type, token1Name)
			Expect(err).To(BeNil())
			Expect(budgetRecord.TotalAmount.Cmp(token1Budget)).To(Equal(0))
			Expect(budgetRecord.UsedAmount.Cmp(decimal.Zero)).To(Equal(0))
			Expect(budgetRecord.RemainAmount.Cmp(token1Budget)).To(Equal(0))
		})

		It("can withdraw budget correctly", func() {
			_ = model.ProjectModel.SetBudget(db, prjRcd.ID, token1Type, token1Name, token1Budget)

			err := model.ProjectModel.WithdrawBudget(db, prjRcd.ID, token1Type, token1Name, token1WithdrawAmount)
			Expect(err).To(BeNil())

			budgetRecord, err := model.ProjectBudgetModel.QueryByProjectIdAndBudgetProps(db, prjRcd.ID, token1Type, token1Name)
			Expect(err).To(BeNil())
			Expect(budgetRecord.TotalAmount.Cmp(token1Budget)).To(BeEquivalentTo(0))
			Expect(budgetRecord.UsedAmount.Cmp(token1WithdrawAmount)).To(BeEquivalentTo(0))
			Expect(budgetRecord.RemainAmount.Cmp(token1Budget.Sub(token1WithdrawAmount))).To(BeEquivalentTo(0))
		})
		It("update project budget record after deposit successfully", func() {
			_ = model.ProjectModel.SetBudget(db, prjRcd.ID, token1Type, token1Name, token1Budget)
			_ = model.ProjectModel.WithdrawBudget(db, prjRcd.ID, token1Type, token1Name, token1WithdrawAmount)

			err := model.ProjectModel.DepositBudget(db, prjRcd.ID, token1Type, token1Name, token1DepositAmount)
			Expect(err).To(BeNil())

			budgetRecord, err := model.ProjectBudgetModel.QueryByProjectIdAndBudgetProps(db, prjRcd.ID, token1Type, token1Name)
			Expect(err).To(BeNil())
			Expect(budgetRecord.TotalAmount.Cmp(token1Budget)).To(BeEquivalentTo(0))
			Expect(budgetRecord.UsedAmount.Cmp(token1WithdrawAmount.Sub(token1DepositAmount))).To(BeEquivalentTo(0))
			Expect(budgetRecord.RemainAmount.Cmp(token1Budget.Sub(token1WithdrawAmount).Add(token1DepositAmount))).To(BeEquivalentTo(0))
		})
		It("create budget record if deposit to non-existing asset", func() {
			err := model.ProjectModel.DepositBudget(db, prjRcd.ID, token2Type, token2Name, token1DepositAmount)
			Expect(err).To(BeNil())

			budgetRecord, err := model.ProjectBudgetModel.QueryByProjectIdAndBudgetProps(db, prjRcd.ID, token2Type, token2Name)
			Expect(err).To(BeNil())
			Expect(budgetRecord.TotalAmount.Cmp(token1DepositAmount)).To(BeEquivalentTo(0))
			Expect(budgetRecord.UsedAmount.Cmp(decimal.Zero)).To(BeEquivalentTo(0))
			Expect(budgetRecord.RemainAmount.Cmp(token1DepositAmount)).To(BeEquivalentTo(0))
		})

	})
})
