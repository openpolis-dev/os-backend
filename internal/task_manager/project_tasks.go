package task_manager

import (
	"encoding/json"
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

type CreateProjectParam struct {
	ProjectName string `json:"project_name"`
	Applicant   string `json:"applicant"`
}

type CloseProjectParam struct {
	ProjectId uint `json:"project_id"`
}

func CreateProjectTask(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("start create project task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	var params CreateProjectParam
	err = json.Unmarshal([]byte(jobParams), &params)

	if err != nil {
		log.Warn().Msgf("create project job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		tx := db.Begin()
		// create project with sponsor
		proj := model.Project{
			Name:     params.ProjectName,
			Status:   model.ProjectStatusOpen,
			Sponsors: []string{params.Applicant},
			Creator:  common.FormatUserWallet(params.Applicant),
			CreateTs: model.GetCurrentUtcEpochSecond(),
			UpdateTs: model.GetCurrentUtcEpochSecond(),
		}
		err = model.ProjectModel.CreateOrUpdate(tx, &proj)
		if err != nil {
			tx.Rollback()
			log.Warn().Msgf("create project error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}

		err = tx.Commit().Error
		if err != nil {
			log.Warn().Msgf("commit create project error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}

		// update permission
		enforcer := storage.GetEnforcer()
		policies := proj.GenerateCasbinPolicies()
		_, err = enforcer.AddPolicies(policies)
		if err != nil {
			log.Warn().Msgf("create project error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}

		// add roles
		sponsorGroupingPolicies := [][]string{
			// g, 0xc13..1283 proj_sponsor_1
			{common.FormatUserWallet(params.Applicant), fmt.Sprintf("%s%d", internal.RoleProjSponsorPrefix, proj.ID)},
		}

		_, err = enforcer.AddGroupingPolicies(sponsorGroupingPolicies)
		if err != nil {
			log.Warn().Msgf("create project error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}
		err = enforcer.SavePolicy()
		if err != nil {
			log.Warn().Msgf("create project error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}
	}

	job.LastExecTs = model.GetCurrentUtcEpochSecond()
	job.LastExecResult = execResult
	job.LastExecutionFailed = jobFailed
	job.State = model.CronJobStateDone
	db.Updates(&job)
}

func CloseProjectTask(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("start close project task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	var params CloseProjectParam
	err = json.Unmarshal([]byte(jobParams), &params)

	if err != nil {
		log.Warn().Msgf("close project job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		err := db.Model(model.Project{}).Where("id = ?", params.ProjectId).Update("status", model.ProjectStatusClosed).Error
		if err != nil {
			log.Warn().Msgf("commit create project error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}

		// TODO: update permission
	}

	job.LastExecTs = model.GetCurrentUtcEpochSecond()
	job.LastExecResult = execResult
	job.LastExecutionFailed = jobFailed
	job.State = model.CronJobStateDone
	db.Updates(&job)
}
