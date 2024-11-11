package task_manager

import (
	"encoding/json"
	"time"

	"github.com/aptible/supercronic/cronexpr"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/theseed-labs/os-backend/internal/storage"
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

	cfg := storage.GetConfig()

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
		querySql := `SELECT p1.* FROM proposals p1 INNER JOIN (
			SELECT proposal_record_id, MAX(version) AS max_version
			FROM proposals
			GROUP BY proposal_record_id
		) p2 ON p1.proposal_record_id = p2.proposal_record_id
		AND p1.version = p2.max_version
		AND p1.proposal_record_id != ''
 		AND p1.state IN ?`

		err := db.Raw(querySql, []model.ProposalState{
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
				metaforoProposalData, err := metaforo.GetProposal(metaforoThreadId, params.GroupName, cfg.MetaforoData.AccessToken, 0)
				if err != nil {
					log.Warn().Msgf("get metaforo proposal error: %+v", err)
					continue
				}

				// Start transaction to update db records
				if err = db.Transaction(func(tx *gorm.DB) error {
					err = proposal.UpdateDbRecordsFromMetaforoProposalResponse(tx, dbRcd.ID, metaforoProposalData)
					if err != nil {
						log.Warn().Msgf("update propsal with metaforo response error: %+v", err)
						return err
					}

					pollStatusChanged, err := proposal.UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(tx, dbRcd.ID, metaforoProposalData)
					if err != nil {
						log.Warn().Msgf("update propsal vote option records with metaforo response error: %+v", err)
						return err
					}

					if pollStatusChanged {
						if err = proposal.HandleProposalPollStatusChange(tx, dbRcd.ID, cfg.MetaforoData.GroupName); err != nil {
							log.Warn().Msgf("handle proposal poll status change error: %+v", err)
							return err
						}
					}
					return nil
				}); err != nil {
					log.Warn().Msgf("update proposal %d error: %+v, continue", dbRcd.ID, err)
					continue
				}
			}
		}

		time.Sleep(1 * time.Second)
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
