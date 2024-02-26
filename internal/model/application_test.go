package model_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/theseed-labs/os-backend/internal/model"
)

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

				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "", nil, nil)
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
		})
		When("to approve new reward application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:             model.MustParseApplicationType("new_reward"),
					Applicant:        aliceWallet,
					State:            model.ApplicationStateOpen,
					EntityType:       "project",
					EntityId:         openProject.ID,
					TargetUserWallet: daveWallet,
					AssetName:        token1Name,
					AssetAmount:      token1RewardAmount,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)

				db.Save(&app)
			})

			It("should update application status to approved and create new audit log", func() {
				preLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(preLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateOpen))
				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "", nil, nil)

				project, _ := model.ProjectModel.Detail(db, openProject.ID)
				Expect(project.Status).To(BeEquivalentTo(model.ProjectStatusOpen))

				postLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(app.State).To(BeEquivalentTo(model.ApplicationStateApproved))

				Expect(postLatestAuditLog.Operator).To(BeEquivalentTo(carolWallet))
				Expect(postLatestAuditLog.Operation).To(BeEquivalentTo(model.AuditActionApprove))
				Expect(postLatestAuditLog.PreState).To(BeEquivalentTo(model.ApplicationStateOpen))
				Expect(postLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateApproved))
			})
		})
		When("to reject new reward application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:             model.MustParseApplicationType("new_reward"),
					Applicant:        aliceWallet,
					State:            model.ApplicationStateOpen,
					EntityType:       "project",
					EntityId:         openProject.ID,
					TargetUserWallet: daveWallet,
					AssetName:        token1Name,
					AssetAmount:      token1RewardAmount,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)
			})
			It("should update application state to rejected", func() {
				preLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(preLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateOpen))
				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionReject, "test reason", nil, nil)
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
				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionReject, "test reason", nil, nil)
				Expect(err).NotTo(BeNil())
			})
		})
		When("to process new reward application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:             model.MustParseApplicationType("new_reward"),
					Applicant:        aliceWallet,
					State:            model.ApplicationStateOpen,
					EntityType:       "project",
					EntityId:         openProject.ID,
					TargetUserWallet: daveWallet,
					AssetName:        token1Name,
					AssetAmount:      token1RewardAmount,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)
				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "", nil, nil)
			})
			It("should update application status to processing and create new audit log", func() {
				preLatestAuditLog, _ := app.GetLatestAuditLog(db)
				Expect(preLatestAuditLog.PostState).To(BeEquivalentTo(model.ApplicationStateApproved))

				err := model.AuditApplication(db, carolWallet, &app, model.AuditActionProcess, "", nil, nil)
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

			// Comment this case out since user assets amount is not using now
			It("should add to processing amount of user asset record", func() {
				//err := model.AuditApplication(db, carolWallet, &app, model.AuditActionProcess, "", nil, nil)
				//Expect(err).To(BeNil())
				//
				//userAssetRcd, err := model.UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, daveWallet, token1Name)
				//Expect(err).To(BeNil())
				//
				//Expect(len(userAssetRcd)).To(Equal(1))
				//Expect(userAssetRcd[0].DealtAmount.Cmp(decimal.Zero)).To(Equal(0))
				//Expect(userAssetRcd[0].ProcessingAmount.Cmp(token1RewardAmount)).To(Equal(0))
			})
		})
		When("to complete new reward application", func() {
			var app model.Application

			BeforeEach(func() {
				app = model.Application{
					Type:             model.MustParseApplicationType("new_reward"),
					Applicant:        aliceWallet,
					State:            model.ApplicationStateOpen,
					EntityType:       "project",
					EntityId:         openProject.ID,
					TargetUserWallet: daveWallet,
					AssetName:        token1Name,
					AssetAmount:      token1RewardAmount,
				}

				// Create correct application before testing
				_ = model.NewApplicationRecord(db, &app)

				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionApprove, "", nil, nil)
				_ = model.AuditApplication(db, carolWallet, &app, model.AuditActionProcess, "", nil, nil)
			})
			// Comment this case out since user assets amount is not using now
			It("should add to processing amount of user asset record", func() {
				//err := model.AuditApplication(db, carolWallet, &app, model.AuditActionComplete, "", nil, nil)
				//Expect(err).To(BeNil())
				//
				//userAssetRcd, err := model.UserAssetRecordModel.FindWithUserWalletAndAssetProps(db, daveWallet, token1Name)
				//Expect(err).To(BeNil())
				//
				//Expect(len(userAssetRcd)).To(Equal(1))
				//Expect(userAssetRcd[0].DealtAmount).To(Equal(token1RewardAmount))
				//Expect(userAssetRcd[0].ProcessingAmount.Cmp(decimal.Zero)).To(Equal(0))
			})
		})
	})
})
