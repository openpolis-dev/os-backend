package model

type CronJobState int

const (
	CronJobStateActive CronJobState = iota
	CronJobStatePaused
	CronJobStateTerminated
	CronJobStateRunning
)

// CronJob saves info for scheduler tasks.
// If the task is executed once, only the NextExecTs will be set.
// If the task will be executed repeatedly, the CronExp field will be set
// There will be a runner that queries the CronJob table and execute the task.
// The detailed execution logic will be described in runner code.
type CronJob struct {
	ID       int `gorm:"primaryKey"`
	CreateTs int64
	UpdateTs int64 `gorm:"index"`

	HandlerName string // Handler name, used to launch correct function while being dispatched

	// Scheduler info
	CronExp    string // Expression of cron job, the format syntax is `s m h dom mon dow`
	LastExecTs int64  `gorm:"index"`
	NextExecTs int64  `gorm:"index"`

	JobParams string       // Job parameters saves params used for job in json string format
	State     CronJobState // Active, Paused, Terminated

	LastExecResult string // Saves last execution result, used for debug
}
