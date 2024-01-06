package model

// CronJob saves info for scheduler tasks.
// If the task is executed once, only the NextRunTs will be set.
// If the task will be executed repeatedly, the CronExp field will be set
// There will be a runner that queries the CronJob table and execute the task.
// The detailed execution logic will be described in runner code.
type CronJob struct {
	ID       int `gorm:"primaryKey"`
	Name     string
	CreateTs int64
	UpdateTs int64 `gorm:"index"`

	// Scheduler info
	CronExp   string // Expression of cron job, the format syntax is `s m h dom mon dow`
	LastRunTs int64  `gorm:"index"`
	NextRunTs int64  `gorm:"index"`

	Command string // Command name for this job, the detailed processing logic is defined in code
	Status  string // Active, Paused, Terminated

	LastRunResult string
}
