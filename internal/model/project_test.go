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

		It("Can set budget correctly", func() {
		})
	})
})
