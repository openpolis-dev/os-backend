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
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type NewRewardDetail struct {
	TargetUserWallet string `json:"address"`
	Amount           string `json:"amount"`
	DetailedType     string `json:"issue"`
	Comment          string `json:"memo"`
	AssetInfo        struct {
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
		Name string `json:"name"`
	} `json:"assetInfo"`
}

type MotivationTaskParam struct {
	Applicant  string              `json:"applicant"`
	ProposalId string              `json:"proposal_id"`
	Records    []*MotivationDetail `json:"budgetList"`
}

type AutoTransferScrItem struct {
	ApplicationId uint   `json:"application_id"`
	TargetWallet  string `json:"target_wallet"`
	ScrAmount     string `json:"scr_amount"`
}

type AutoTransferScrParam struct {
	Applicant string                 `json:"applicant"`
	Items     []*AutoTransferScrItem `json:"items"`
}

type AutoTransferScrTaskResult struct {
	TxHash      string `json:"tx_hash"`
	TxStatus    string `json:"tx_status"`
	TxError     string `json:"tx_error"`
	TxTimestamp string `json:"tx_timestamp"`
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
					// TODO: Duplicated code *NewAppBundleAndApplication*
					// Create AppBundle
					appBundle := model.AppBundle{
						Applicant:    common.FormatUserWallet(params.Applicant),
						SeasonId:     currentSeason.ID,
						State:        model.ApplicationStateApproved,
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
										State:            model.ApplicationStateApproved,
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
							var appAuditLogs []*model.ApplicationAuditLog
							for _, appRcd := range appBundle.AppRecords {
								appAuditLogs = append(appAuditLogs, &model.ApplicationAuditLog{
									ApplicationID: appRcd.ID,
									LogTs:         model.GetCurrentUtcEpochSecond(),
									Operation:     model.AuditActionNew,
									Operator:      common.FormatUserWallet(params.Applicant),
									PreState:      "",
									PostState:     model.ApplicationStateOpen,
								})
								appAuditLogs = append(appAuditLogs, &model.ApplicationAuditLog{
									ApplicationID: appRcd.ID,
									LogTs:         model.GetCurrentUtcEpochSecond(),
									Operation:     model.AuditActionApprove,
									Operator:      common.FormatUserWallet(params.Applicant),
									PreState:      model.ApplicationStateOpen,
									PostState:     model.ApplicationStateApproved,
								})
							}

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

func AutoTransferSCR(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("enter auto transfer SCR task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	var params *AutoTransferScrParam
	err = json.Unmarshal([]byte(jobParams), &params)
	if err != nil {
		log.Warn().Msgf("parse auto transfer SCR params error: %+v", err)
		return
	}

	apiEndpoint, err := model.GetXferScrApi(db)
	if err != nil {
		log.Warn().Msgf("get xfer SCR service api base error: %+v", err)
		return
	}

	// TODO: Invoke SCR transfer service to transfer SCR
	resp, err := sdk.SendScr(apiEndpoint, jobParams, common.FormatUserWallet(params.Applicant))
	if err != nil {
		log.Error().Msgf("send SCR request error: %+v", err)
		return
	}

	log.Debug().Msgf("send SCR response: %s", string(resp))
	db.Model(&job).Update("last_exec_result", string(resp)).Update("state", model.CronJobStateDone)

	log.Debug().Msgf("exit auto transfer SCR task: %+v", job)
}
