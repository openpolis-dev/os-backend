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

// RefreshVotingProposalVoteInfoJob refresh the vote info of proposal in voting state.
// Tasks in this job contains:
// - Check whether the voting has closed, if yes, verify the result and update the proposal state
// - Update voter's data in vote record
// - Save execution result and Set next execution timestamp
func RefreshVotingProposalVoteInfoJob(db *gorm.DB, job *model.CronJob, jobParams string) {
	log.Debug().Msgf("refresh voting proposal vote info job: %+v", job)
	// Clear NextExecTs to avoid launch again while the job is running
	err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	if err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
		return
	}

	var params RefreshVotingProposalVoteInfoJobParams
	err = json.Unmarshal([]byte(jobParams), &params)
	execResult := ""
	if err != nil {
		log.Warn().Msgf("refresh voting proposal vote info job params error: %+v", err)
		execResult = err.Error()

	} else {
		var proposals []*model.Proposal
		err = db.Model(&model.Proposal{}).
			Where("state = ?", model.ProposalStateVoting).
			Distinct("proposal_record_id").
			Select("id, proposal_record_id").
			Find(&proposals).Error
		if err != nil {
			log.Warn().Msgf("refresh proposal list error: %+v", err)
			return
		}

		for _, dbRcd := range proposals {
			metaforoThreadId := dbRcd.GetMetaforoThreadId()
			metaforoPropsalData, err := metaforo.GetProposal(metaforoThreadId, params.GroupName, "", 0)
			if err != nil {
				log.Warn().Msgf("get metaforo proposal error: %+v", err)
			}

			err = proposal.UpdateDbRecordsFromMetaforoProposalResponse(db, dbRcd, metaforoPropsalData)
			if err != nil {
				log.Warn().Msgf("get metaforo proposal error: %+v", err)
			}
		}
	}
	// Calculate next time after execution done
	if err = updateJobExecutionInfoForNextRun(db, job, execResult); err != nil {
		log.Warn().Msgf("update cron job error: %+v", err)
	}
	log.Debug().Msgf("updated job running info: %+v", job)
	log.Debug().Msgf("refresh voting proposal vote info job done: %+v", job)
}

func updateJobExecutionInfoForNextRun(db *gorm.DB, job *model.CronJob, execResult string) error {
	log.Debug().Msgf("prepare to update running info of job: %+v", job)
	job.LastExecTs = time.Now().UTC().Unix()
	job.LastExecResult = execResult
	job.NextExecTs = cronexpr.MustParse(job.CronExp).Next(time.Now()).UTC().Unix()
	job.State = model.CronJobStateActive
	return db.Save(&job).Error
}

func Foobar() {
	log.Error().Msgf("TTTasdfasdfasfsad fsa")
}
