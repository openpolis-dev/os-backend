package task_manager

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

const queryCronJobRecordFromProposalIdSQL = `select
	cj.*,
	p.id as proposal_id
from
	cron_jobs cj
inner join proposal_component_records pcr on	cj.proposal_component_record_id = pcr.id
inner join proposals p on pcr.proposal_id = p.id
where p.id = ?`

type VoteProposalParams struct {
	Applicant            string `json:"applicant"`
	VetoProposalId       string `json:"proposal_id"`
	BeVetoedProposalInfo struct {
		Applicant            string `json:"applicant"`
		ApplicantAvatar      string `json:"applicant_avatar"`
		CreateTs             int    `json:"create_ts"`
		Id                   int    `json:"id"`
		Name                 string `json:"name"`
		ProposalCategoryName string `json:"proposal_category_name"`
		ProposalState        string `json:"proposal_state"`
	} `json:"proposal_info"`
}

type UpdateProposalStateParams struct {
	ProposalId uint `json:"proposal_id"`
	State      int  `json:"state"`
}

type AssociateProposalParams struct {
	Relate     string `json:"relate"`
	ProposalId uint   `json:"proposal_id"`
}

func CreateVetoProposalTask(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("start veto proposal task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	var params VoteProposalParams
	err = json.Unmarshal([]byte(jobParams), &params)

	vetoProposalId := strings.Replace(params.VetoProposalId, "os-", "", -1)

	if err != nil {
		log.Warn().Msgf("veto proposal job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		// Veto proposal
		// 1. Mark cronjob related to the specified proposal to terminated
		// 2. Mark the proposal to vetoed state
		// 3. If proposal has related applications and app_bundles, mark them as rejected
		//   - The auto transfer scr tasks will be cancelled while processing cron job table so no need extra logic
		var proposalTasks []*model.CronJob
		if err := db.Transaction(func(tx *gorm.DB) error {
			err = tx.Raw(queryCronJobRecordFromProposalIdSQL, params.BeVetoedProposalInfo.Id).Find(&proposalTasks).Error
			if err != nil {
				log.Warn().Msgf("fetch be vetoed proposal data error: %+v", err)
				execResult = err.Error()
				jobFailed = true
				return err
			}

			for _, r := range proposalTasks {
				r.State = model.CronJobStateTerminated
				r.UpdateTs = model.GetCurrentUtcEpochSecond()
				tx.Model(&model.CronJob{}).Updates(r)
			}
			tx.Updates(&proposalTasks)

			err = tx.Model(&model.Proposal{}).Where("id = ?", params.BeVetoedProposalInfo.Id).Update("state", model.ProposalStateVetoed).Error
			if err != nil {
				log.Warn().Msgf("fetch be vetoed proposal data error: %+v", err)
				execResult = err.Error()
				jobFailed = true
				return err
			}

			log.Debug().Msgf("clear all cron job for proposal %d be vetoed", params.BeVetoedProposalInfo.Id)
			if err = db.Model(&model.CronJob{}).Where(&model.CronJob{ProposalId: uint(params.BeVetoedProposalInfo.Id)}).Delete(&model.CronJob{}).Error; err != nil {
				log.Error().Msgf("delete cronjob error while vetoing proposal: %+v", err)
				return err
			}

			// Mark project associated to proposal be vetoed to close_failed
			var dbProposalRcd model.Proposal
			if err = tx.Find(&dbProposalRcd, params.BeVetoedProposalInfo.Id).Error; err != nil {
				log.Warn().Msgf("fetch veto proposal data error: %+v", err)
				execResult = err.Error()
				jobFailed = true
				return err
			}
			proposalIsForClosingProject, project, err := proposal.IsProposalIsForClosingProject(tx, dbProposalRcd.ID)
			if err != nil {
				log.Warn().Msgf("check proposal is closing project error: %+v", err)
				execResult = err.Error()
				jobFailed = true
				return err
			}

			log.Debug().Msgf("proposal %d is closing project proposal? %+v, related project: %+v", dbProposalRcd.ID, proposalIsForClosingProject, project)

			if proposalIsForClosingProject {
				log.Debug().Msgf("proposal %d is for closing project %v", dbProposalRcd.ID, project)
				if err = tx.Model(&project).Update("status", model.ProjectStatusCloseFailed).Error; err != nil {
					log.Warn().Msgf("update project status to close_failed error: %+v", err)
					execResult = err.Error()
					jobFailed = true
					return err
				}

				// Reject app_bundle and applications associated to this project
				var appBundle *model.AppBundle
				if err = tx.Model(&appBundle).Where(&model.AppBundle{EntityType: "project", EntityId: project.ID}).Update("state = ", model.ApplicationStateRejected).Error; err != nil {
					log.Warn().Msgf("reject app_bundle for project %d error: %+v", project.ID, err)
					execResult = err.Error()
					jobFailed = true
					return err
				}

				if err = tx.Model(&appBundle).Where(&model.Application{EntityType: "project", EntityId: project.ID}).Update("state = ", model.ApplicationStateRejected).Error; err != nil {
					log.Warn().Msgf("reject applications for project %d error: %+v", project.ID, err)
					execResult = err.Error()
					jobFailed = true
					return err
				}
			}

			return nil
		}); err != nil {
			log.Error().Msgf("update vetoed proposal error")
			jobFailed = true
		}
	}
	job.LastExecTs = model.GetCurrentUtcEpochSecond()
	job.LastExecResult = execResult
	job.LastExecutionFailed = jobFailed
	job.State = model.CronJobStateDone
	db.Updates(&job)

	if jobFailed {
		db.Model(&model.Proposal{}).Where("id = ?", vetoProposalId).Update("state", model.ProposalStateExecutionFailed)
	} else {
		db.Model(&model.Proposal{}).Where("id = ?", vetoProposalId).Update("state", model.ProposalStateExecuted)
	}
}

func UpdateProposalStateTask(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("start updating proposal state task: %+v", job)
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	var params UpdateProposalStateParams
	err = json.Unmarshal([]byte(jobParams), &params)
	if err != nil {
		log.Warn().Msgf("parse update proposal job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		dbProposalRcd := model.Proposal{ID: params.ProposalId}
		log.Debug().Msgf("prepare to update dbProposalRcd %d from state %d to %d", dbProposalRcd.ID, dbProposalRcd.State, params.State)
		if dbProposalRcd.IsInFinState() {
			err := fmt.Errorf("dbProposalRcd %d is already in final state %d", dbProposalRcd.ID, dbProposalRcd.State)
			log.Warn().Msgf(err.Error())
			execResult = err.Error()
			jobFailed = true
		} else {
			_, err = proposal.UpdateProposalStateAndLaunchStateChangeActions(db, nil, fmt.Sprintf("%d", dbProposalRcd.ID), model.ProposalState(params.State), storage.GetConfig())
			if err != nil {
				log.Warn().Msgf("update dbProposalRcd state error: %+v", err)
				execResult = err.Error()
				jobFailed = true
			}
		}
	}
	job.LastExecTs = model.GetCurrentUtcEpochSecond()
	job.LastExecResult = execResult
	job.LastExecutionFailed = jobFailed
	job.State = model.CronJobStateDone
	db.Updates(&job)
}
