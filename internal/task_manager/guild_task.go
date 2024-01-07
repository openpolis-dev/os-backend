package task_manager

import (
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

func CreateGuildTask(db *gorm.DB, job *model.CronJob, jobParams string) {}

func CloseGuildTask(db *gorm.DB, job *model.CronJob, jobParams string) {}
