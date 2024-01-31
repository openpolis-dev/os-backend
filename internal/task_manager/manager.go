package task_manager

import (
	"errors"
	"fmt"
	"time"

	"github.com/aptible/supercronic/cronexpr"
	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type TaskManager struct {
	DatabaseClient      *gorm.DB
	CheckIntervalSecond int
	CheckDuration       time.Duration

	Scheduler gocron.Scheduler

	RootJob gocron.Job

	TaskChannel chan *model.CronJob

	AppConfig *config.Config
}

var taskMgr *TaskManager

var err error

func InitTaskManager(db *gorm.DB, checkIntervalSecond int, cfg *config.Config) {
	if checkIntervalSecond == 0 {
		checkIntervalSecond = internal.DefaultTaskRunnerCheckIntervalSecond
	}
	taskMgr = &TaskManager{
		DatabaseClient:      db,
		CheckIntervalSecond: checkIntervalSecond,
		CheckDuration:       time.Duration(checkIntervalSecond) * time.Second,
		TaskChannel:         make(chan *model.CronJob),
		AppConfig:           cfg,
	}

	log.Error().Msgf("Next run time from now: %s", cronexpr.MustParse(internal.TaskRefreshVotingProposalVoteInfoCronExpr).Next(time.Now()))

	// Init built-in tasks
	// - RefreshProposalVoteState
	refreshVoteStateJob := &model.CronJob{
		HandlerName: internal.TaskRefreshVotingProposalVoteInfo,
	}
	if err := db.Where(&refreshVoteStateJob).Assign(model.CronJob{
		State:      model.CronJobStateActive,
		CronExp:    internal.TaskRefreshVotingProposalVoteInfoCronExpr,
		CreateTs:   time.Now().UTC().Unix(),
		NextExecTs: cronexpr.MustParse(internal.TaskRefreshVotingProposalVoteInfoCronExpr).Next(time.Now()).UTC().Unix(),
		JobParams:  fmt.Sprintf(`{"group_name": "%s"}`, cfg.MetaforoData.GroupName),
	}).FirstOrCreate(&refreshVoteStateJob).Error; err != nil {
		panic(err)
	}
}

func GetTaskManager() *TaskManager {
	return taskMgr
}

func (t *TaskManager) StartRunner() {
	t.Scheduler, err = gocron.NewScheduler()
	if err != nil {
		panic(err)
	}

	t.RootJob, err = t.Scheduler.NewJob(
		gocron.DurationJob(t.CheckDuration),
		gocron.NewTask(t.ScanTaskPool))
	if err != nil {
		panic(err)
	}

	// Repeat refresh RefreshVoteStateJob every minute
	_, err = t.Scheduler.NewJob(
		gocron.DurationJob(time.Minute),
		gocron.NewTask(t.ActivateRefreshVoteStateJobIfRequired))
	if err != nil {
		panic(err)
	}

	go t.TaskDispatcher()

	t.Scheduler.Start()
}

func (t *TaskManager) StopRunner() {
	err := t.Scheduler.StopJobs()
	if err != nil {
		log.Error().Msgf("stop scheduler job error: %+v", err)
	}
}

// ScanTaskPool function scans the cron_job tables, list out all tasks which state is activate and next run time is in current time window, then dispatch it to task dispatcher with channel
func (t *TaskManager) ScanTaskPool() {
	var tasksShouldBeExecuted []*model.CronJob
	startTime := time.Now().UTC()
	endTime := time.Now().Add(t.CheckDuration).UTC()

	err := t.DatabaseClient.Model(&model.CronJob{}).
		Where("state = ? AND ((next_exec_ts >= ? AND next_exec_ts < ?) OR last_exec_ts=0)",
			model.CronJobStateActive, startTime.Unix(), endTime.Unix()).Find(&tasksShouldBeExecuted).Error

	if err != nil {
		log.Error().Msgf("scan task pool error: %+v", err)
		return
	}

	for _, job := range tasksShouldBeExecuted {
		log.Debug().Msgf("dispatched job: %+v", job)
		t.TaskChannel <- job
	}
}

func (t *TaskManager) ActivateRefreshVoteStateJobIfRequired() error {
	log.Debug().Msgf("activate refresh vote state job")
	refreshVoteStateJob := &model.CronJob{
		HandlerName: internal.TaskRefreshVotingProposalVoteInfo,
		State:       model.CronJobStateActive,
		CronExp:     internal.TaskRefreshVotingProposalVoteInfoCronExpr,
		CreateTs:    time.Now().UTC().Unix(),
		NextExecTs:  cronexpr.MustParse(internal.TaskRefreshVotingProposalVoteInfoCronExpr).Next(time.Now()).UTC().Unix(),
		JobParams:   fmt.Sprintf(`{"group_name": "%s"}`, t.AppConfig.MetaforoData.GroupName),
	}

	if err := t.DatabaseClient.Model(&refreshVoteStateJob).Where("handler_name = ?", internal.TaskRefreshVotingProposalVoteInfo).Updates(&refreshVoteStateJob).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return t.DatabaseClient.Create(&refreshVoteStateJob).Error
		} else {
			return err
		}
	}
	return nil
}

func (t *TaskManager) TaskDispatcher() {
	log.Debug().Msgf("task dispatcher started")
	for {
		task := <-t.TaskChannel
		switch task.HandlerName {
		case internal.TaskRefreshVotingProposalVoteInfo:
			go RefreshVotingProposalInfoJob(t.DatabaseClient, task, task.JobParams)
		case internal.TaskVetoedProposal:
			log.Debug().Msgf("veto proposal task")
			go CreateVetoProposalTask(t.DatabaseClient, task, task.JobParams)
		case internal.TaskNewMotivationReward:
			log.Debug().Msgf("motivation task")
			go CreateAppBundleTaskFromMotivationComponent(t.DatabaseClient, task, task.JobParams, task.VoteType, task.VoteResult)
		case internal.TaskCloseGuild:
		case internal.TaskRewardNewApplication:
		case internal.TaskCreateGuild:
		case internal.TaskCloseProject:
		case internal.TaskCreateProject:
		default:
			// Handle unknown task
			log.Warn().Msgf("unknown task name: %s task detail: %+v", task.HandlerName, task)
			t.MarkTaskAsTerminated(task)
		}
	}
}

func (t *TaskManager) MarkTaskAsTerminated(task *model.CronJob) {
	task.State = model.CronJobStateTerminated
	task.LastExecTs = model.GetCurrentUtcEpochSecond()
	task.UpdateTs = model.GetCurrentUtcEpochSecond()
	t.DatabaseClient.Updates(task)
}
