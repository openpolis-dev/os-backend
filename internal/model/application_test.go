package model_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/model"
)

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

var openProject, pendingCloseProject, closedProject *model.Project

var tables = []any{
	&model.Application{},
	&model.ApplicationAuditLog{},
	&model.Project{},
	&model.ProjectBudget{},
	&model.User{},
	&model.UserAssetRecord{},
}

var _ = Describe("Application", func() {

	// Before each `It` execution, create the table and init project data
	BeforeEach(func() {
		_ = db.AutoMigrate(tables...)
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
		_ = db.Migrator().DropTable(tables...)
	})

	Describe("Invoking NewApplicationRecord function", func() {
		When("to create close project application", func() {
			It("should create application record and related audit log if project is open and change project to pending_close state", func() {
				_ = model.NewApplicationRecord(db, &model.Application{
					Type:       model.MustParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
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
					Type:       model.MustParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "project",
					EntityId:   pendingCloseProject.ID,
				})).ToNot(BeNil())

				Expect(model.NewApplicationRecord(db, &model.Application{
					Type:       model.MustParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "project",
					EntityId:   closedProject.ID,
				})).ToNot(BeNil())
			})
			It("should return error if entity type is guild", func() {
				Expect(model.NewApplicationRecord(db, &model.Application{
					Type:       model.MustParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "guild",
					EntityId:   42, // A fake ID
				})).ToNot(BeNil())
			})
			// TODO: Test with guild records
		})

		When("to create new reward application", func() {
			It("should create application record and related audit log if project is in open state", func() {
				_ = model.NewApplicationRecord(db, &model.Application{
					Type:       model.MustParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
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
					Type:       model.MustParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "project",
					EntityId:   pendingCloseProject.ID,
				})).NotTo(BeNil())
				Expect(model.NewApplicationRecord(db, &model.Application{
					Type:       model.MustParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
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
					Type:       model.MustParseApplicationType("close_project"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "project",
					EntityId:   openProject.ID,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)
			})

			It("should change project to closed state if project is in pending_close status and create completed audit logs", func() {
				preLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(preLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateOpen))

				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")
				Expect(err).To(BeNil())

				project, _ := model.ProjectModel.Detail(db, openProject.ID)
				// The project changes to closed state
				Expect(project.Status).To(BeEquivalentTo(model.ProjectStatusClosed))

				postLatestAuditLog, _ := app.GetLatestAuditLog(db)

				// A new audit log with correct state change has been inserted
				Expect(postLatestAuditLog.ID).NotTo(BeEquivalentTo(preLatestAuditLog.ID))
				Expect(postLatestAuditLog.Operator).To(BeEquivalentTo(carolWallet))
				Expect(postLatestAuditLog.Operation).To(BeEquivalentTo(model.AuditActionApprove))
				Expect(postLatestAuditLog.PreState).To(BeEquivalentTo(model.ApplicationStateOpen))
				Expect(postLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateCompleted))

				// The application should be changed to completed now
				Expect(app.State).To(BeEquivalentTo(model.ApplicationStateCompleted))
			})
			It("should return error if project is not in pending_close status", func() {
				db.Model(&openProject).Update("status", model.ApplicationStateOpen)
				Expect(model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")).NotTo(BeNil())
			})
		})
		When("to approve new reward application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:       model.MustParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "project",
					EntityId:   openProject.ID,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)

				// Set budget for project
				_ = model.ProjectModel.SetBudget(db, openProject.ID, token1Type, token1Name, 100)
				_ = model.ProjectModel.SetBudget(db, openProject.ID, token2Type, token2Name, 200)

				// For new_reward application, detailed data is required for reward detail
				detailedData := model.NewRewardApplicationDetailedData{
					token1Type: {
						ApplicationID:    app.ID,
						TargetUserWallet: daveWallet,
						AssetType:        token1Type,
						AssetName:        token1Name,
						Amount:           10,
					},
				}
				detailedDataByte, _ := json.Marshal(detailedData)
				app.DetailedData = detailedDataByte
				db.Save(&app)
			})
			It("should prepare the project and application ready", func() {
				Expect(app.State).To(BeEquivalentTo(model.ApplicationStateOpen))
				Expect(openProject.Status).To(BeEquivalentTo(model.ProjectStatusOpen))

				budgetRcds, _ := model.ProjectBudgetModel.ListByProjectId(db, openProject.ID)
				Expect(len(budgetRcds)).To(BeEquivalentTo(2))

				tokenNameAmountList := lo.Map(budgetRcds, func(r *model.ProjectBudget, _ int) map[string]uint64 {
					return map[string]uint64{r.Name: r.TotalAmount}
				})
				Expect(tokenNameAmountList).To(ConsistOf([]map[string]uint64{{token1Name: 100}, {token2Name: 200}}))
			})
			It("should update application status to approved and create new audit log", func() {
				preLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(preLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateOpen))
				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")

				project, _ := model.ProjectModel.Detail(db, openProject.ID)
				Expect(project.Status).To(BeEquivalentTo(model.ProjectStatusOpen))

				postLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(app.State).To(BeEquivalentTo(model.ApplicationStateApproved))

				Expect(postLatestAuditLog.Operator).To(BeEquivalentTo(carolWallet))
				Expect(postLatestAuditLog.Operation).To(BeEquivalentTo(model.AuditActionApprove))
				Expect(postLatestAuditLog.PreState).To(BeEquivalentTo(model.ApplicationStateOpen))
				Expect(postLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateApproved))
			})
			It("should return error if project is not in open status", func() {
				openProject.Status = model.ProjectStatusPendingClose
				db.Save(&openProject)

				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")
				Expect(err).NotTo(BeNil())
			})
			It("should not change project budget record", func() {
				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")
				budgetRcds, _ := model.ProjectBudgetModel.ListByProjectId(db, openProject.ID)
				Expect(len(budgetRcds)).To(Equal(2))
				tokenNameAmountList := lo.Map(budgetRcds, func(r *model.ProjectBudget, _ int) map[string]uint64 {
					return map[string]uint64{r.Name: r.TotalAmount}
				})
				Expect(tokenNameAmountList).To(ConsistOf([]map[string]uint64{{token1Name: 100}, {token2Name: 200}}))
			})
		})
		When("to reject new reward application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:       model.MustParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "project",
					EntityId:   openProject.ID,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)

				// Set budget for project
				_ = model.ProjectModel.SetBudget(db, openProject.ID, token1Type, token1Name, 100)
				_ = model.ProjectModel.SetBudget(db, openProject.ID, token2Type, token2Name, 200)

				// For new_reward application, detailed data is required for reward detail
				detailedData := model.NewRewardApplicationDetailedData{
					token1Type: {
						ApplicationID:    app.ID,
						TargetUserWallet: daveWallet,
						AssetType:        token1Type,
						AssetName:        token1Name,
						Amount:           10,
					},
				}
				detailedDataByte, _ := json.Marshal(detailedData)
				app.DetailedData = detailedDataByte
				db.Save(&app)
			})
			It("should update application state to rejected", func() {
				preLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(preLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateOpen))
				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionReject, "test reason")
				Expect(err).To(BeNil())

				project, _ := model.ProjectModel.Detail(db, openProject.ID)
				Expect(project.Status).To(BeEquivalentTo(model.ProjectStatusOpen))

				Expect(app.State).To(BeEquivalentTo(model.ApplicationStateRejected))
				Expect(app.RejectReason).To(BeEquivalentTo("test reason"))

				postLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(postLatestAuditLog.Operator).To(BeEquivalentTo(carolWallet))
				Expect(postLatestAuditLog.Operation).To(BeEquivalentTo(model.AuditActionReject))
				Expect(postLatestAuditLog.PreState).To(BeEquivalentTo(model.ApplicationStateOpen))
				Expect(postLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateRejected))
			})
			It("should return error if application state is not open", func() {
				db.Model(&app).Update("state", model.ApplicationStateApproved)
				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionReject, "test reason")
				Expect(err).NotTo(BeNil())
			})
		})
		When("to process new reward application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:       model.MustParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "project",
					EntityId:   openProject.ID,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)

				// Set budget for project
				_ = model.ProjectModel.SetBudget(db, openProject.ID, token1Type, token1Name, 100)
				_ = model.ProjectModel.SetBudget(db, openProject.ID, token2Type, token2Name, 200)

				// For new_reward application, detailed data is required for reward detail
				detailedData := model.NewRewardApplicationDetailedData{
					token1Type: {
						ApplicationID:    app.ID,
						TargetUserWallet: daveWallet,
						AssetType:        token1Type,
						AssetName:        token1Name,
						Amount:           10,
					},
				}
				detailedDataByte, _ := json.Marshal(detailedData)
				app.DetailedData = detailedDataByte

				db.Save(&app)

				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")
			})
			It("should update application status to processing and create new audit log", func() {
				preLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(preLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateApproved))

				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionProcess, "")
				Expect(err).To(BeNil())

				project, _ := model.ProjectModel.Detail(db, openProject.ID)
				Expect(project.Status).To(BeEquivalentTo(model.ProjectStatusOpen))

				Expect(app.State).To(BeEquivalentTo(model.ApplicationStateProcessing))

				postLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(postLatestAuditLog.Operator).To(BeEquivalentTo(carolWallet))
				Expect(postLatestAuditLog.Operation).To(BeEquivalentTo(model.AuditActionProcess))
				Expect(postLatestAuditLog.PreState).To(BeEquivalentTo(model.ApplicationStateApproved))
				Expect(postLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateProcessing))
			})
			It("should update project budget remain amount", func() {
				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionProcess, "")
				Expect(err).To(BeNil())

				budgetRecords, _ := model.ProjectBudgetModel.ListByProjectId(db, openProject.ID)
				for _, r := range budgetRecords {
					if r.Type == token1Type {
						Expect(r.RemainAmount).To(Equal(r.TotalAmount - 10)) // 100-10
					} else if r.Type == token2Type {
						Expect(r.RemainAmount).To(Equal(r.TotalAmount))
					}
				}
			})
			It("should add to processing amount of user asset record", func() {
				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionProcess, "")
				Expect(err).To(BeNil())

				userAssetRcd, err := model.UserAssetRecordModel.FindWithUserWalletAndAssetType(db, daveWallet, token1Type)
				Expect(err).To(BeNil())

				Expect(len(userAssetRcd)).To(Equal(1))
				Expect(userAssetRcd[0].DealtAmount).To(BeEquivalentTo(0))
				Expect(userAssetRcd[0].ProcessingAmount).To(BeEquivalentTo(10))
			})
		})
		When("to complete new reward application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:       model.MustParseApplicationType("new_reward"),
					Applicant:  aliceWallet,
					State:      model.ApplicationStateOpen,
					EntityType: "project",
					EntityId:   openProject.ID,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)

				// Set budget for project
				_ = model.ProjectModel.SetBudget(db, openProject.ID, token1Type, token1Name, 100)
				_ = model.ProjectModel.SetBudget(db, openProject.ID, token2Type, token2Name, 200)

				// For new_reward application, detailed data is required for reward detail
				detailedData := model.NewRewardApplicationDetailedData{
					token1Type: {
						ApplicationID:    app.ID,
						TargetUserWallet: daveWallet,
						AssetType:        token1Type,
						AssetName:        token1Name,
						Amount:           10,
					},
				}
				detailedDataByte, _ := json.Marshal(detailedData)
				app.DetailedData = detailedDataByte
				db.Save(&app)

				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "")
				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionProcess, "")
			})
			It("should add to processing amount of user asset record", func() {
				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionComplete, "")
				Expect(err).To(BeNil())

				userAssetRcd, err := model.UserAssetRecordModel.FindWithUserWalletAndAssetType(db, daveWallet, token1Type)
				Expect(err).To(BeNil())

				Expect(len(userAssetRcd)).To(Equal(1))
				Expect(userAssetRcd[0].DealtAmount).To(BeEquivalentTo(10))
				Expect(userAssetRcd[0].ProcessingAmount).To(BeEquivalentTo(0))
			})
		})
	})
})
