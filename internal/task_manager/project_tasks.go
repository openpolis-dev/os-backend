package task_manager

import (
	"encoding/json"
	"fmt"
	"strings"

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
	ProposalId  string `json:"proposal_id"`
	Budget      string `json:"budget"`
	Deliverable string `json:"deliverable"`
	PlanTime    string `json:"plan_time"`
	Sip         int    `json:"sip"`
}

type CloseProjectParam struct {
	ProjectInfo struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	} `json:"project_info"`
}

type UpdateProjectOwnerParam struct {
	NewAdminWallet string `json:"admin_wallet"`
	ProjectInfo    struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	} `json:"common_project"`
}

// CreateProjectTask does not change proposal state since creating project template contains 3 components and does not
// follow the component - action model like other templates. For other tasks, update the proposal state based on job execution result.
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
			Name:      params.ProjectName,
			Status:    model.ProjectStatusOpen,
			Proposals: []string{params.ProposalId},
			Sponsors:  []string{params.Applicant},
			Creator:   common.FormatUserWallet(params.Applicant),
			SIP:       fmt.Sprintf("%d", params.Sip),
			PlanTime:  params.PlanTime,
			Budgets:   params.Budget,

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
		policies := model.GenerateCasbinPoliciesForProject(proj.ID)
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
		err := db.Model(model.Project{}).Where("id = ?", params.ProjectInfo.Id).Update("status", model.ProjectStatusClosed).Error
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

// UpdateProjectOwner sets the first sponsor of the project to given data
func UpdateProjectOwner(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("start update project owner task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	var params UpdateProjectOwnerParam
	err = json.Unmarshal([]byte(jobParams), &params)

	if err != nil {
		log.Warn().Msgf("close project job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		err = db.Transaction(func(tx *gorm.DB) error {
			var existingSponsors string
			err := tx.Model(model.Project{}).Where("id = ?", params.ProjectInfo.Id).Pluck("sponsors", &existingSponsors).Error
			if err != nil {
				log.Warn().Msgf("get project with id %d error: %+v", params.ProjectInfo.Id, err)
				return err
			}

			var sponsorsList []string
			if existingSponsors != "" {
				err = json.Unmarshal([]byte(existingSponsors), &sponsorsList)
				if err != nil {
					log.Warn().Msgf("unmarshal sponsors error: %+v", err)
					return err
				}

				if len(sponsorsList) > 0 {
					sponsorsList[0] = params.NewAdminWallet
				} else {
					sponsorsList = append(sponsorsList, params.NewAdminWallet)
				}
			} else {
				log.Warn().Msgf("project with id %d has no sponsor", params.ProjectInfo.Id)
				sponsorsList = append(sponsorsList, params.NewAdminWallet)
			}

			sponsorsListStr := fmt.Sprintf("[\"%s\"]", strings.Join(sponsorsList, "\",\""))
			err = tx.Model(model.Project{}).Where("id = ?", params.ProjectInfo.Id).
				Update("sponsors", sponsorsListStr).
				Update("contant_way", "").
				Error
			if err != nil {
				return err
			}
			return nil
		})

		if err != nil {
			log.Warn().Msgf("update project owner error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		}
	}

	job.LastExecTs = model.GetCurrentUtcEpochSecond()
	job.LastExecResult = execResult
	job.LastExecutionFailed = jobFailed
	job.State = model.CronJobStateDone
	db.Updates(&job)

	if jobFailed {
		db.Model(&model.Proposal{}).Where("id = ?", job.ProposalId).Update("state", model.ProposalStateExecutionFailed)
	} else {
		db.Model(&model.Proposal{}).Where("id = ?", job.ProposalId).Update("state", model.ProposalStateExecuted)
	}
}
