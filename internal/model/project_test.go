package model_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

			openPrjs, prjCnt, err := model.ProjectModel.List(db, string(model.ProjectStatusOpen), nil)
			Expect(err).To(BeNil())
			Expect(prjCnt).To(BeEquivalentTo(1))
			Expect(len(openPrjs)).To(Equal(1))

			pendingClosePrjs, prjCnt, err := model.ProjectModel.List(db, string(model.ProjectStatusPendingClose), nil)
			Expect(err).To(BeNil())
			Expect(prjCnt).To(BeEquivalentTo(0))
			Expect(len(pendingClosePrjs)).To(Equal(0))

			closedPrjs, prjCnt, err := model.ProjectModel.List(db, string(model.ProjectStatusClosed), nil)
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
			err := model.ProjectModel.SetBudget(db, prjRcd.ID, token1Type, token1Name, 100)
			Expect(err).To(BeNil())

			budgetRecord, err := model.ProjectBudgetModel.QueryByProjectIdAndBudgetProps(db, prjRcd.ID, token1Type, token1Name)
			Expect(err).To(BeNil())
			Expect(budgetRecord.TotalAmount).To(BeEquivalentTo(100))
			Expect(budgetRecord.RemainAmount).To(BeEquivalentTo(100))
		})

		It("can withdraw budget correctly", func() {
			_ = model.ProjectModel.SetBudget(db, prjRcd.ID, token1Type, token1Name, 100)

			err := model.ProjectModel.WithdrawBudget(db, prjRcd.ID, token1Type, token1Name, 50)
			Expect(err).To(BeNil())

			budgetRecord, err := model.ProjectBudgetModel.QueryByProjectIdAndBudgetProps(db, prjRcd.ID, token1Type, token1Name)
			Expect(err).To(BeNil())
			Expect(budgetRecord.TotalAmount).To(BeEquivalentTo(100))
			Expect(budgetRecord.RemainAmount).To(BeEquivalentTo(50))
		})
		It("update project budget record after deposit successfully", func() {
			_ = model.ProjectModel.SetBudget(db, prjRcd.ID, token1Type, token1Name, 100)
			_ = model.ProjectModel.WithdrawBudget(db, prjRcd.ID, token1Type, token1Name, 50)

			err := model.ProjectModel.DepositBudget(db, prjRcd.ID, token1Type, token1Name, 20)
			Expect(err).To(BeNil())

			budgetRecord, err := model.ProjectBudgetModel.QueryByProjectIdAndBudgetProps(db, prjRcd.ID, token1Type, token1Name)
			Expect(err).To(BeNil())
			Expect(budgetRecord.TotalAmount).To(BeEquivalentTo(100))
			Expect(budgetRecord.RemainAmount).To(BeEquivalentTo(70))
		})
		It("create budget record if deposit to non-existing asset", func() {
			err := model.ProjectModel.DepositBudget(db, prjRcd.ID, token2Type, token2Name, 20)
			Expect(err).To(BeNil())

			budgetRecord, err := model.ProjectBudgetModel.QueryByProjectIdAndBudgetProps(db, prjRcd.ID, token2Type, token2Name)
			Expect(err).To(BeNil())
			Expect(budgetRecord.TotalAmount).To(BeEquivalentTo(20))
			Expect(budgetRecord.RemainAmount).To(BeEquivalentTo(20))
		})

	})
})
