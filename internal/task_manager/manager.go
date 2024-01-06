package task_manager

import (
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"gorm.io/gorm"
)

type TaskManager struct {
	DatabaseClient      *gorm.DB
	CheckIntervalSecond int

	Scheduler gocron.Scheduler

	RootJob gocron.Job
}

var taskMgr *TaskManager

var err error

func InitTaskManager(db *gorm.DB, checkIntervalSecond int) {
	if checkIntervalSecond == 0 {
		checkIntervalSecond = internal.DefaultTaskRunnerCheckIntervalSecond
	}
	taskMgr = &TaskManager{DatabaseClient: db, CheckIntervalSecond: checkIntervalSecond}

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
		gocron.DurationJob(time.Second*time.Duration(t.CheckIntervalSecond)),
		gocron.NewTask(t.ScanTaskPool))
	if err != nil {
		panic(err)
	}

	//t.Scheduler.Start()
}

func (t *TaskManager) StopRunner() {
	err := t.Scheduler.StopJobs()
	if err != nil {
		log.Error().Msgf("stop scheduler job error: %+v", err)
	}
}

func (t *TaskManager) ScanTaskPool() {
	log.Error().Msgf("scan task pool")
}
