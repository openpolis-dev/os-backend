package application

import (
	"encoding/json"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/task_manager"
	"gorm.io/gorm"
)

const ScrTaskPendingTime = 24 * time.Hour

func createAutoTransferScrTask(db *gorm.DB, applications []model.Application) error {
	// Issue send SCR tasks
	scrApplications := lo.Filter(applications, func(app model.Application, _ int) bool {
		return app.Type == model.ApplicationNewReward && app.AssetName == "SCR"
	})

	if len(scrApplications) > 0 {
		return db.Transaction(func(tx *gorm.DB) error {
			// Group scr applications by applicant
			groupedScrApplications := lo.GroupBy(scrApplications, func(app model.Application) string {
				return app.Applicant
			})

			currentTs := time.Now().UTC()
			jobExecTs := currentTs.Add(ScrTaskPendingTime)

			for applicant, apps := range groupedScrApplications {
				jobParams := task_manager.AutoTransferScrParam{
					Applicant: applicant,
					Items: lo.Map(apps, func(app model.Application, _ int) *task_manager.AutoTransferScrItem {
						return &task_manager.AutoTransferScrItem{
							ApplicationId: app.ID,
							TargetWallet:  app.TargetUserWallet,
							ScrAmount:     app.AssetAmount.String(),
						}
					}),
				}

				jobParamsBytes, err := json.Marshal(jobParams)
				if err != nil {
					log.Error().Msgf("marshal auto transfer SCR job params error: %+v", err)
					return err
				}

				sendScrTask := &model.CronJob{
					CreateTs:       currentTs.Unix(),
					UpdateTs:       currentTs.Unix(),
					HandlerName:    internal.TaskAutoTransferSCR,
					LastExecTs:     0,
					NextExecTs:     jobExecTs.Unix(),
					JobParams:      string(jobParamsBytes),
					State:          model.CronJobStateActive,
					LastExecResult: "",
				}

				log.Debug().Msgf("create auto transfer SCR task: %+v", sendScrTask)
				err = tx.Create(&sendScrTask).Error
				if err != nil {
					log.Error().Msgf("create auto transfer SCR task error: %+v", err)
					return err
				}
			}

			return nil
		})
	}

	return nil
}
