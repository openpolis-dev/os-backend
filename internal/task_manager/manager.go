package task_manager

import (
	"fmt"
	"time"

	"github.com/aptible/supercronic/cronexpr"
	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
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
}

var taskMgr *TaskManager

var err error

func InitTaskManager(db *gorm.DB, checkIntervalSecond int) {
	if checkIntervalSecond == 0 {
		checkIntervalSecond = internal.DefaultTaskRunnerCheckIntervalSecond
	}
	taskMgr = &TaskManager{
		DatabaseClient:      db,
		CheckIntervalSecond: checkIntervalSecond,
		CheckDuration:       time.Duration(checkIntervalSecond) * time.Second,
		TaskChannel:         make(chan *model.CronJob),
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
		JobParams:  fmt.Sprintf(`{"group_name": "%s"}`, internal.MetaforoGroupName),
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
		Where("state = ?", model.CronJobStateActive).
		Where("(next_exec_ts >= ? AND next_exec_ts < ?) OR last_exec_ts=0", startTime.Unix(), endTime.Unix()).Find(&tasksShouldBeExecuted).Error

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
	}
	if err := t.DatabaseClient.Where(&refreshVoteStateJob).Updates(model.CronJob{
		State:      model.CronJobStateActive,
		CronExp:    internal.TaskRefreshVotingProposalVoteInfoCronExpr,
		CreateTs:   time.Now().UTC().Unix(),
		NextExecTs: cronexpr.MustParse(internal.TaskRefreshVotingProposalVoteInfoCronExpr).Next(time.Now()).UTC().Unix(),
		JobParams:  fmt.Sprintf(`{"group_name": "%s"}`, internal.MetaforoGroupName),
	}).FirstOrCreate(&refreshVoteStateJob).Error; err != nil {
		log.Error().Msgf("activate refresh vote state job error: %+v", err)
		return err
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
		case internal.TaskCreateProject:
			//go CreateProjectTask(t.DatabaseClient, task, task.JobParams)
			task.State = model.CronJobStateDone
			t.DatabaseClient.Updates(task)
			log.Debug().Msgf("create project")
		case internal.TaskCloseProject:
			go CloseProjectTask(t.DatabaseClient, task, task.JobParams)
		case internal.TaskCreateGuild:
			//go CreateGuildTask(t.DatabaseClient, task, task.JobParams)
			task.State = model.CronJobStateDone
			t.DatabaseClient.Updates(task)
			log.Debug().Msgf("create guild")
		case internal.TaskCloseGuild:
			go CloseGuildTask(t.DatabaseClient, task, task.JobParams)
		case internal.TaskRewardNewApplication:
			task.State = model.CronJobStateDone
			t.DatabaseClient.Updates(task)
			log.Debug().Msgf("reward new application")
		default:
			log.Warn().Msgf("unknown task name: %s task detail: %+v", task.HandlerName, task)
			// Handle unknown task
		}
	}
}
