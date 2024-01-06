package task_manager

import (
	"encoding/json"
	"time"

	"github.com/aptible/supercronic/cronexpr"
	"github.com/rs/zerolog/log"
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
	var params RefreshVotingProposalVoteInfoJobParams
	err := json.Unmarshal([]byte(jobParams), &params)
	if err != nil {
		log.Warn().Msgf("refresh voting proposal vote info job params error: %+v", err)
		if err = updateJobExecutionInfo(db, job); err != nil {
			log.Warn().Msgf("update cron job error: %+v", err)
		}
	} else {
		var proposals []*model.Proposal
		err = db.Model(&model.Proposal{}).
			Where("state = ?", model.ProposalStateVoting).
			Select("id, proposal_record_id").
			Find(&proposals).Error
		if err != nil {
			log.Warn().Msgf("refresh proposal list error: %+v", err)
			return
		}

		for _, proposal := range proposals {
			metaforoThreadId := proposal.GetMetaforoThreadId()
			metaforoPropsalData, err := metaforo.GetProposal(metaforoThreadId, params.GroupName, "", 0)
			if err != nil {
				log.Warn().Msgf("get metaforo proposal error: %+v", err)
			}

			// 从vote信息中更新vote数据，并检查是否需要更改状态

		}

		// Calculate next time after execution done
		if err = updateJobExecutionInfo(db, job); err != nil {
			log.Warn().Msgf("update cron job error: %+v", err)
		}
	}

}

func updateJobExecutionInfo(db *gorm.DB, job *model.CronJob) error {
	nextTime := cronexpr.MustParse(job.CronExp).Next(time.Now()).UTC()
	return db.Model(&job).Updates(model.CronJob{
		LastExecTs:     time.Now().UTC().Unix(),
		LastExecResult: "",
		NextExecTs:     nextTime.Unix(),
	}).Error
}
