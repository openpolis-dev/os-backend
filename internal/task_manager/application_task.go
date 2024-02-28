package task_manager

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type NewRewardDetail struct {
	TargetUserWallet string `json:"address"`
	Amount           string `json:"amount"`
	DetailedType     string `json:"issue"`
	Comment          string `json:"memo"`
	AssetInfo        struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	} `json:"type"`
}

type NewRewardTaskParam struct {
	Applicant  string `json:"applicant"`
	EntityInfo struct {
		Id   uint   `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"budget"`
	Description string             `json:"description"`
	ProposalId  string             `json:"proposal_id"`
	Records     []*NewRewardDetail `json:"receiverList"`
}

type MotivationDetail struct {
	TargetUserWallet string `json:"address"`
	Amount           string `json:"amount"`
	Comment          string `json:"description"`
	AssetInfo        struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	} `json:"assetInfo"`
}

type MotivationTaskParam struct {
	Applicant  string              `json:"applicant"`
	ProposalId string              `json:"proposal_id"`
	Records    []*MotivationDetail `json:"budgetList"`
}

// TODO: This for old new_reward component, which is similar with motivation component. It is not using for now.

func CreateAppBundleTaskFromNewRewardComponent(db *gorm.DB, job *model.CronJob, jobParams string, voteType int, voteResult string) {
}

func CreateAppBundleTaskFromMotivationComponent(db *gorm.DB, job *model.CronJob, jobParams string, voteType int, voteResult string) {
	log.Debug().Msgf("enter create app bundle from motivation component task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	currentSeason, err := model.GetCurrentSeason(db)
	if err != nil {
		log.Warn().Msgf("get current season error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		var params MotivationTaskParam
		err = json.Unmarshal([]byte(jobParams), &params)
		if err != nil {
			log.Warn().Msgf("create app bundle job params error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		} else {
			ratio, _ := decimal.NewFromString("1")
			if (voteType == model.ProposalVoteTypeNumericAvg) || (voteType == model.ProposalVoteTypeNumericSingle) {
				ratio, err = decimal.NewFromString(voteResult)
				if err != nil {
					log.Error().Msgf("parse value %s to decimal error: %+v", voteResult, err)
				}
			}

			if ratio.IsZero() {
				execResult = "vote result is zero, no need to create applications"
				jobFailed = false
			} else {
				proposalId := strings.Replace(params.ProposalId, "os-", "", -1)
				var proposalDbRcd *model.Proposal
				db.First(&proposalDbRcd, proposalId)

				prjDbRcd := model.Project{
					SIP: fmt.Sprintf("%d", proposalDbRcd.Sip),
				}
				db.Model(&prjDbRcd).Where(&prjDbRcd).First(&prjDbRcd)

				if prjDbRcd.ID == 0 {
					log.Debug().Msgf("no project found with sip %d, check whether this is cityhall evaluation proposal.", proposalDbRcd.Sip)
					var cityHallEvaluationTempId uint
					db.Model(&model.ProposalTemplate{}).Where("name = ?", internal.CityHallEvaluationTemplateName).Pluck("id", &cityHallEvaluationTempId)
					if *proposalDbRcd.ProposalTemplateID == cityHallEvaluationTempId {
						cityHallRcd, err := model.GetCityHallProject(db)
						if err != nil {
							log.Warn().Msgf("get cityhall project error: %+v", err)
							execResult = err.Error()
							jobFailed = true
						} else {
							log.Debug().Msgf("cityhall project found: %+v", cityHallRcd)
							prjDbRcd = *cityHallRcd
						}
					}
				}

				if !jobFailed {
					// Limit only one motivation component record for one proposal
					existingAppBundle := model.AppBundle{
						EntityType: "project",
						EntityId:   prjDbRcd.ID,
					}

					var appBundleRecordsCount int64
					db.Model(&existingAppBundle).Where(&existingAppBundle).Count(&appBundleRecordsCount)
					log.Debug().Msgf("Found %d records for app bundle", appBundleRecordsCount)
					if appBundleRecordsCount > 0 {
						log.Error().Msgf("appliation bundle with sip %s is already exist", prjDbRcd.SIP)
						execResult = fmt.Sprintf("app bundle for project %d is existing", prjDbRcd.ID)
						jobFailed = true
					}

					// TODO: Duplicated code *NewAppBundleAndApplication*
					// Create AppBundle
					appBundle := model.AppBundle{
						Applicant:    common.FormatUserWallet(params.Applicant),
						SeasonId:     currentSeason.ID,
						State:        model.ApplicationStateOpen,
						ShadowRecord: false,
						CreateTs:     model.GetCurrentUtcEpochSecond(),
						UpdateTs:     model.GetCurrentUtcEpochSecond(),
						Type:         "NEW_REWARD",
						EntityType:   "project",
						EntityId:     prjDbRcd.ID,
					}
					err = db.Model(model.AppBundle{}).Create(&appBundle).Error
					if err != nil {
						log.Error().Msgf("Create app bundle records error: %+v", err)
						execResult = err.Error()
						jobFailed = true
					} else {
						// Create Applications inside the bundle
						err = db.Transaction(func(tx *gorm.DB) error {
							appBundle.AppRecords = lo.Map(params.Records, func(appDetail *MotivationDetail, index int) *model.Application {
								assetAmount, err := decimal.NewFromString(appDetail.Amount)
								if err != nil {
									log.Warn().Msgf("parse asset amount error: %+v", err)
									execResult = err.Error()
									jobFailed = true
									return nil
								} else {
									return &model.Application{
										Type:             model.ApplicationNewReward,
										Applicant:        common.FormatUserWallet(params.Applicant),
										State:            model.ApplicationStateOpen,
										CreatedAt:        time.Now().In(internal.ProjectTimezone),
										UpdatedAt:        time.Now().In(internal.ProjectTimezone),
										CreateTs:         model.GetCurrentUtcEpochSecond(),
										UpdateTs:         model.GetCurrentUtcEpochSecond(),
										Comment:          appDetail.Comment,
										TargetUserWallet: appDetail.TargetUserWallet,
										AssetName:        appDetail.AssetInfo.Name,
										AssetAmount:      assetAmount.Mul(ratio),
										EntityType:       appBundle.EntityType,
										EntityId:         appBundle.EntityId,
										SeasonId:         currentSeason.ID,
									}
								}
							})

							err = tx.Save(&appBundle).Error
							if err != nil {
								log.Error().Msgf("update app_bundle record error: %+v", err)
								return err
							}

							// Create application audit logs
							appAuditLogs := lo.Map(appBundle.AppRecords, func(app *model.Application, _ int) *model.ApplicationAuditLog {
								return &model.ApplicationAuditLog{
									ApplicationID: app.ID,
									LogTs:         model.GetCurrentUtcEpochSecond(),
									Operation:     model.AuditActionNew,
									Operator:      common.FormatUserWallet(params.Applicant),
									PreState:      "",
									PostState:     model.ApplicationStateOpen,
								}
							})
							err = tx.Model(model.ApplicationAuditLog{}).Create(&appAuditLogs).Error
							if err != nil {
								log.Error().Msgf("Create application audit log records error: %+v", err)
								return err
							}

							return tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
								AppBundleId: appBundle.ID,
								AppBundle:   appBundle,
								LogTs:       model.GetCurrentUtcEpochSecond(),
								Operation:   model.AuditActionNew,
								Operator:    common.FormatUserWallet(params.Applicant),
								PreState:    "",
								PostState:   model.ApplicationStateOpen,
								ExtraData:   "",
							}).Error
						})

						if err != nil {
							log.Error().Msgf("Transaction error: %+v", err)
							execResult = err.Error()
							jobFailed = true
						}
					}
				}
			}
		}

		proposalId := strings.Replace(params.ProposalId, "os-", "", -1)

		var proposal model.Proposal
		if err = db.Find(&proposal, proposalId).Error; err != nil {
			log.Error().Msgf("get proposal %s error: %+v", proposalId, err)
			execResult = err.Error()
			jobFailed = true
		} else {
			// Get project database record and update project status based on proposal execution result
			prjDbRcd := model.Project{
				SIP: fmt.Sprintf("%d", proposal.Sip),
			}

			if err = db.Transaction(func(tx *gorm.DB) error {
				if jobFailed {
					tx.Model(&model.Proposal{}).Where("id = ?", proposalId).Update("state", model.ProposalStateExecutionFailed)
					tx.Model(&prjDbRcd).Where(&prjDbRcd).Update("status", model.ProjectStatusCloseFailed)
				} else {
					var pDbRcd model.Proposal
					if err := tx.Model(&model.Proposal{}).Where("id = ?", proposalId).Find(&pDbRcd).Error; err != nil {
						log.Error().Msgf("find proposal error: %+v", err)
						jobFailed = true
						execResult = err.Error()
						tx.Model(&prjDbRcd).Where(&prjDbRcd).Update("status", model.ProjectStatusCloseFailed)
					} else {
						if err := tx.Model(&prjDbRcd).Where(&prjDbRcd).
							Update("status", model.ProjectStatusClosed).
							Update("over_link", fmt.Sprintf("/proposal/thread/%d", proposal.ID)).Error; err != nil {
							log.Error().Msgf("close project error: %+v", err)
							jobFailed = true
							execResult = err.Error()
							tx.Model(&prjDbRcd).Where(&prjDbRcd).Update("status", model.ProjectStatusCloseFailed)
						} else {
							tx.Model(&model.Proposal{}).Where("id = ?", proposalId).Update("state", model.ProposalStateExecuted)
						}
					}
				}
				return nil
			}); err != nil {
				log.Error().Msgf("update proposa or project state error: %+v", err)
				jobFailed = true
			}
		}
	}

	job.LastExecTs = model.GetCurrentUtcEpochSecond()
	job.LastExecResult = execResult
	job.LastExecutionFailed = jobFailed
	job.State = model.CronJobStateDone
	err = db.Updates(&job).Error
	if err != nil {
		log.Error().Msgf("Update cron job error: %+v", err)
	}

	log.Error().Msgf("TTT: exit create app bundle from motivation component task: %+v", job)
}
