package task_manager

import (
	"encoding/json"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/model"
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
	if err != nil {
		log.Warn().Msgf("veto proposal job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		// Veto proposal
		// 1. Mark cronjob related to the specified proposal to terminated
		// 2. Mark the proposal to vetoed state
		var proposalTasks []*model.CronJob
		err := db.Raw(queryCronJobRecordFromProposalIdSQL, params.BeVetoedProposalInfo.Id).Find(&proposalTasks).Error
		if err != nil {
			log.Warn().Msgf("fetch be vetoed proposal data error: %+v", err)
			execResult = err.Error()
			jobFailed = true
		} else {
			for _, r := range proposalTasks {
				r.State = model.CronJobStateTerminated
				r.UpdateTs = model.GetCurrentUtcEpochSecond()
				db.Updates(r)
			}
			db.Updates(&proposalTasks)

			err = db.Model(&model.Proposal{}).Where("id = ?", params.BeVetoedProposalInfo.Id).Update("state", model.ProposalStateVetoed).Error
			if err != nil {
				log.Warn().Msgf("fetch be vetoed proposal data error: %+v", err)
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

	vetoProposalId := strings.Replace(params.VetoProposalId, "os-", "", -1)

	if jobFailed {
		db.Model(&model.Proposal{}).Where("id = ?", vetoProposalId).Update("state", model.ProposalStateExecutionFailed)
	} else {
		db.Model(&model.Proposal{}).Where("id = ?", vetoProposalId).Update("state", model.ProposalStateExecuted)
	}
}

func UpdateProposalSateTask(db *gorm.DB, job *model.CronJob, jobParams string) {
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
		proposal := model.Proposal{ID: params.ProposalId}
		log.Debug().Msgf("update proposal %d from state %d to %d", proposal.ID, proposal.State, params.State)
		proposal.State = params.State
		err = db.Updates(&proposal).Error
		if err != nil {
			log.Warn().Msgf("update proposal state error: %+v", err)
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
