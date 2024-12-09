package applications_inject

import "github.com/theseed-labs/os-backend/internal/task_manager"

type AuditRequestBody struct {
	Message string `json:"message"`
}

type ApplicantListResponse struct {
	Applicant string
	Name      string
}

type AutoXferTaskResponse struct {
	ID        int   `json:"id"`
	CreateTs  int64 `json:"create_ts"`
	UpdateTs  int64 `json:"update_ts"`
	ExecuteTs int64 `json:"execute_ts"`

	State string `json:"state"`

	TransactionItems []*task_manager.AutoTransferScrItem `json:"transaction_items"`

	IsFinished bool `json:"is_finished"`

	TransactionHash string `json:"transaction_hash"`
}

type AutoXferTaskListQueryParams struct {
	Page      int    `form:"page"`
	Size      int    `form:"size"`
	SortField string `form:"sort_field"`
	SortOrder string `form:"sort_order"`

	State string `form:"state"`
}
