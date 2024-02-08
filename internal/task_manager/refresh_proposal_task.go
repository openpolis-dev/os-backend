package task_manager

import (
	"encoding/json"
	"time"

	"github.com/aptible/supercronic/cronexpr"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

type RefreshVotingProposalVoteInfoJobParams struct {
	GroupName string `json:"group_name"`
}

// RefreshVotingProposalInfoJob refresh the info of proposal in voting state.
// Tasks in this job contains:
// - Check whether the voting has closed, if yes, verify the result and update the proposal state
// - Update voter's data in vote record
// - Save execution result and Set next execution timestamp
func RefreshVotingProposalInfoJob(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("refresh voting proposal info job: %+v", job)
	// Clear NextExecTs to avoid launch again while the job is running
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	execResult := ""
	jobFailed := false

	var params RefreshVotingProposalVoteInfoJobParams
	err = json.Unmarshal([]byte(jobParams), &params)

	if err != nil {
		log.Warn().Msgf("refresh voting proposal vote info job params error: %+v", err)
		execResult = err.Error()
		jobFailed = true
	} else {
		var proposals []*model.Proposal
		querySql := `select distinct on (proposal_record_id) proposals.* FROM "proposals" WHERE state IN ?`
		err = db.Raw(querySql, []model.ProposalState{
			model.ProposalStateVoting,
			model.ProposalStateApproved,
			model.ProposalStateDraft,
		}).Find(&proposals).Error
		if err != nil {
			log.Warn().Msgf("get proposal list error: %+v", err)
			jobFailed = true
		} else {
			for _, dbRcd := range proposals {
				metaforoThreadId := dbRcd.GetMetaforoThreadId()
				metaforoProposalData, err := metaforo.GetProposal(metaforoThreadId, params.GroupName, "", 0)
				if err != nil {
					log.Warn().Msgf("get metaforo proposal error: %+v", err)
					jobFailed = true
					execResult = err.Error()
					continue
				}

				err = proposal.UpdateDbRecordsFromMetaforoProposalResponse(db, dbRcd, metaforoProposalData)
				if err != nil {
					log.Warn().Msgf("update propsal with metaforo response error: %+v", err)
					jobFailed = true
					execResult = err.Error()
					continue
				}
				time.Sleep(1 * time.Second)
			}
		}
	}
	// Calculate next time after execution done
	if err = updateJobExecutionInfoForNextRun(db, job, execResult, jobFailed); err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
	}
	log.Debug().Msgf("updated job running info: %+v", job)
	log.Debug().Msgf("refresh voting proposal info job done: %+v", job)
}

func updateJobExecutionInfoForNextRun(db *gorm.DB, job *model.CronJob, execResult string, jobFailed bool) error {
	log.Debug().Msgf("prepare to update running info of job: %+v", job)
	job.LastExecTs = time.Now().UTC().Unix()
	job.LastExecResult = execResult
	job.NextExecTs = cronexpr.MustParse(job.CronExp).Next(time.Now()).UTC().Unix()
	job.State = model.CronJobStateActive
	job.LastExecutionFailed = jobFailed
	return db.Save(&job).Error
}
