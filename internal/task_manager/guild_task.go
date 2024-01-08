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

type CloseGuildParam struct {
	GuildId uint `json:"guild_id"`
}

func CreateGuildTask(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("start create guild task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	var params CreateEntityParam
	err = json.Unmarshal([]byte(jobParams), &params)

	if err != nil {
		log.Warn().Msgf("create guild job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		tx := db.Begin()
		// create guild with sponsor
		guild := model.Guild{
			Name:     params.Name,
			Sponsors: []string{params.Applicant},
			Status:   model.ProjectStatusOpen,
			Creator:  common.FormatUserWallet(params.Applicant),
			CreateTs: model.GetCurrentUtcEpochSecond(),
			UpdateTs: model.GetCurrentUtcEpochSecond(),
		}
		err = model.GuildModel.CreateOrUpdate(tx, &guild)
		if err != nil {
			tx.Rollback()
			log.Warn().Msgf("create guild error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}

		err = tx.Commit().Error
		if err != nil {
			log.Warn().Msgf("commit create guild error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}

		// update permission
		enforcer := storage.GetEnforcer()
		policies := guild.GenerateCasbinPolicies()
		_, err = enforcer.AddPolicies(policies)
		if err != nil {
			log.Warn().Msgf("create guild error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}

		// add roles
		sponsorGroupingPolicies := [][]string{
			// g, 0xc13..1283 proj_sponsor_1
			{common.FormatUserWallet(params.Applicant), fmt.Sprintf("%s%d", internal.RoleGuildSponsorPrefix, guild.ID)},
		}

		_, err = enforcer.AddGroupingPolicies(sponsorGroupingPolicies)
		if err != nil {
			log.Warn().Msgf("create guild error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}
		err = enforcer.SavePolicy()
		if err != nil {
			log.Warn().Msgf("create guild error: %+v", err)
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

func CloseGuildTask(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("start close guild task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	var params CloseGuildParam
	err = json.Unmarshal([]byte(jobParams), &params)

	if err != nil {
		log.Warn().Msgf("close guild job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		err := db.Model(model.Guild{}).
			Where("id = ?", params.GuildId).
			Update("status", model.ProjectStatusClosed).Error
		if err != nil {
			log.Warn().Msgf("close guild error: %+v", err)
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
