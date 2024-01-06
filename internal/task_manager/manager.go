package task_manager

import (
	"time"

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
		Where("next_run_ts >= ? AND next_run_ts < ?", startTime.Unix(), endTime.Unix()).Find(&tasksShouldBeExecuted).Error

	if err != nil {
		log.Error().Msgf("scan task pool error: %+v", err)
		return
	}

	for _, job := range tasksShouldBeExecuted {
		log.Debug().Msgf("dispatched job: %+v", job)
		t.TaskChannel <- job
	}

	log.Error().Msgf("TTT: tasks: %+v", tasksShouldBeExecuted)
}

func (t *TaskManager) TaskDispatcher() {
	// Please help to generate a code segment that get tasks from channel and process with task name
	for {
		task := <-t.TaskChannel
		switch task.HandlerName {
		case internal.TaskRefreshVotingProposalVoteInfo:
			// Process task1
		default:
			log.Warn().Msgf("unknown task name: %s task detail: %+v", task.HandlerName, task)
			// Handle unknown task
		}
	}
}
