package task_manager

import (
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

func VetoProposal(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("start create project task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	job.LastExecTs = model.GetCurrentUtcEpochSecond()
	job.LastExecResult = execResult
	job.LastExecutionFailed = jobFailed
	job.State = model.CronJobStateDone
	db.Updates(&job)
}
