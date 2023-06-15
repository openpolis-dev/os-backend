package model_test

import (
	"github.com/glebarez/sqlite"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

const (
	aliceWallet = "0x2866E6B2aA58942261F126530b069951e7b271D2"
	bobWallet   = "0xe6ade4161Bb5294A9Ec25B6F75D86Fa167e800Ce"
	carolWallet = "0x2866E6B2aA58942261F126530b069951e7b271D2"
)

var _ = Describe("Application", func() {
	var db *gorm.DB
	var openProject, pendingCloseProject, closedProject *model.Project

	// TODO: Verify whether BeforeEach is meet the record init requirements
	BeforeEach(func() {
		db, _ = gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
		_ = db.AutoMigrate(&model.Application{}, &model.Project{}, &model.ApplicationAuditLog{})
		openProject = &model.Project{Name: "Open Project", Status: model.ProjectStatusOpen}
		pendingCloseProject = &model.Project{Name: "Open Project", Status: model.ProjectStatusPendingClose}
		closedProject = &model.Project{Name: "Open Project", Status: model.ProjectStatusClosed}
		db.Save(openProject)
		db.Save(pendingCloseProject)
		db.Save(closedProject)
	})

	Describe("NewApplicationRecord function", func() {
		Context("close project application can be created on open project", func() {
			It("should create application record and related audit log if project is open", func() {
				_ = model.NewApplicationRecord(db, &model.Application{
					Type:       model.ParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "project",
					EntityId:   openProject.ID,
				})

				var applications []*model.Application
				db.Find(&model.Application{}).Find(&applications)
				Expect(len(applications)).To(BeEquivalentTo(1))
				Expect(applications[0].Applicant).To(Equal(aliceWallet))
				Expect(applications[0].Type).To(Equal(model.ApplicationCloseProject))
				Expect(applications[0].State).To(Equal(model.ApplicationStateOpen))
				Expect(applications[0].EntityType).To(Equal("project"))
				Expect(applications[0].EntityId).To(Equal(openProject.ID))

				auditLogs, _ := applications[0].ListAuditLogs(db)
				Expect(len(auditLogs)).To(BeEquivalentTo(1))
				Expect(auditLogs[0].ApplicationID).To(Equal(applications[0].ID))
				Expect(auditLogs[0].Operator).To(Equal(aliceWallet))
				Expect(auditLogs[0].Operation).To(Equal(model.AuditActionNew))
				Expect(auditLogs[0].PostState).To(Equal(model.ApplicationStateOpen))
			})

			It("should return error if project is not in open state", func() {
				Expect(model.NewApplicationRecord(db, &model.Application{
					Type:       model.ParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "project",
					EntityId:   pendingCloseProject.ID,
				})).ToNot(BeNil())

				Expect(model.NewApplicationRecord(db, &model.Application{
					Type:       model.ParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "project",
					EntityId:   closedProject.ID,
				})).ToNot(BeNil())
			})
			It("should return error if entity type is guild", func() {
				Expect(model.NewApplicationRecord(db, &model.Application{
					Type:       model.ParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "guild",
					EntityId:   42, // A fake ID
				})).ToNot(BeNil())
			})
			// TODO: Test with guild records
		})

		//Context("new reward application", func() {
		//	It("should create application record and related audit log if project is open", func() {
		//		appData := model.Application{
		//			Type:       model.ParseApplicationType("new_reward"),
		//			Applicant:  aliceWallet,
		//			State:      "open",
		//			EntityType: "project",
		//			EntityId:   openProject.ID,
		//		}
		//
		//		_ = model.NewApplicationRecord(db, &appData)
		//
		//		var applications []*model.Application
		//		db.Find(&model.Application{}).Find(&applications)
		//		Expect(len(applications)).To(BeEquivalentTo(1))
		//		Expect(applications[0].Applicant).To(Equal(aliceWallet))
		//		Expect(applications[0].Type).To(Equal(model.ApplicationCloseProject))
		//		Expect(applications[0].State).To(Equal(model.ApplicationStateOpen))
		//		Expect(applications[0].EntityType).To(Equal("project"))
		//		Expect(applications[0].EntityId).To(Equal(openProject.ID))
		//
		//		auditLogs, _ := applications[0].ListAuditLogs(db)
		//		Expect(len(auditLogs)).To(BeEquivalentTo(1))
		//		Expect(auditLogs[0].ApplicationID).To(Equal(applications[0].ID))
		//		Expect(auditLogs[0].Operator).To(Equal(aliceWallet))
		//		Expect(auditLogs[0].Operation).To(Equal(model.AuditActionNew))
		//		Expect(auditLogs[0].PostState).To(Equal(model.ApplicationStateOpen))
		//	})
		//})
	})

	Describe("Invoking AuditApplication function", func() {
	})
})
