package model_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal/model"
)

const (
	aliceWallet = "0x2866e6b2aa58942261f126530b069951e7b271d2"
	bobWallet   = "0xe6ade4161bb5294a9ec25b6f75d86fa167e800ce"
	carolWallet = "0x2866e6b2aa58942261f126530b069951e7b871d2"
)

var openProject, pendingCloseProject, closedProject *model.Project

var _ = Describe("Application", func() {

	// Before each `It` execution, create the table and init project data
	BeforeEach(func() {
		_ = db.AutoMigrate(&model.Application{}, &model.Project{}, &model.ApplicationAuditLog{}, &model.User{})
		openProject = &model.Project{Name: "Open Project", Status: model.ProjectStatusOpen}
		pendingCloseProject = &model.Project{Name: "Open Project", Status: model.ProjectStatusPendingClose}
		closedProject = &model.Project{Name: "Open Project", Status: model.ProjectStatusClosed}
		db.Create(&[]*model.Project{openProject, pendingCloseProject, closedProject})

		db.Create(&model.User{Wallet: aliceWallet})
		db.Create(&model.User{Wallet: bobWallet})
		db.Create(&model.User{Wallet: carolWallet})
	})

	// After each `It` execution, drop tables
	AfterEach(func() {
		_ = db.Migrator().DropTable(model.ApplicationAuditLog{}, model.Application{}, model.Project{}, model.User{})
	})

	Describe("Invoking NewApplicationRecord function", func() {
		When("to create close project application", func() {
			It("should create application record and related audit log if project is open and change project to pending_close state", func() {
				_ = model.NewApplicationRecord(db, &model.Application{
					Type:       model.ParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "project",
					EntityId:   openProject.ID,
				})

				var applications []*model.Application
				db.Find(&model.Application{Type: model.ApplicationCloseProject}).Find(&applications)
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

				updatedProject, _ := model.ProjectModel.Detail(db, openProject.ID)
				Expect(updatedProject.Status).To(BeEquivalentTo(model.ProjectStatusPendingClose))
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

		When("to create new reward application", func() {
			It("should create application record and related audit log if project is in open state", func() {
				_ = model.NewApplicationRecord(db, &model.Application{
					Type:       model.ParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "project",
					EntityId:   openProject.ID,
				})

				var applications []*model.Application
				db.Find(&model.Application{Type: model.ApplicationNewReward}).Find(&applications)
				Expect(len(applications)).To(BeEquivalentTo(1))
				Expect(applications[0].Applicant).To(Equal(aliceWallet))
				Expect(applications[0].Type).To(Equal(model.ApplicationNewReward))
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
					Type:       model.ParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "project",
					EntityId:   pendingCloseProject.ID,
				})).NotTo(BeNil())
				Expect(model.NewApplicationRecord(db, &model.Application{
					Type:       model.ParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "project",
					EntityId:   closedProject.ID,
				})).NotTo(BeNil())
			})
		})
	})

	Describe("Invoking AuditApplication function", func() {
		When("to approve close project application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:       model.ParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      "open",
					EntityType: "project",
					EntityId:   openProject.ID,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)
			})

			It("should change project to closed state if project is in pending_close status and create related audit logs", func() {
				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")

				project, _ := model.ProjectModel.Detail(db, openProject.ID)
				Expect(project.Status).To(BeEquivalentTo(model.ProjectStatusClosed))

				// TODO: Check audit logs
			})
			It("should return error if project is not in pending_close status", func() {
				db.Model(&openProject).Update("status", "open")
				p, _ := model.ProjectModel.Detail(db, openProject.ID)
				fmt.Printf("prj: %+v\n", p)
				Expect(model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")).NotTo(BeNil())
			})
		})
	})
})
