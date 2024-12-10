package appbundles_inject

import (
	"time"

	"github.com/theseed-labs/os-backend/internal/model"
)

type AppBundleResponseRecord struct {
	ID         uint                               `json:"id"`
	SeasonName string                             `json:"season_name"`
	Records    []*model.FrontendApplicationRecord `json:"records"`

	Entity struct {
		Id   uint   `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"entity"`
	Applicant string                 `json:"applicant"`
	ApplyTime time.Time              `json:"apply_time"`
	ApplyTs   int64                  `json:"apply_ts"`
	Reviewer  string                 `json:"reviewer"`
	Comment   string                 `json:"comment"`
	State     model.ApplicationState `json:"state"`
	Assets    []struct {
		Name   string `json:"name"`
		Amount string `json:"amount"`
	} `json:"assets"`
}

type ListAvailableProjectAndGuildResp struct {
	Guilds             []*model.Guild              `json:"guilds"`
	Projects           []*model.Project            `json:"projects"`
	CommonBudgetSource []*model.CommonBudgetSource `json:"common_budget_source"`
}
