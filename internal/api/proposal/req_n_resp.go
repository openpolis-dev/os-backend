package proposal

import "github.com/theseed-labs/os-backend/internal/api/component"

///////////////////////
// Request data definitions
///////////////////////

type ListQueryParams struct {
	Page      int    `form:"page"`
	Size      int    `form:"size"`
	SortField string `form:"sort_field"`
	SortOrder string `form:"sort_order"`
	Status    string `form:"status"`
}

// ComponentRequestData represents a component request, which contains component id and associated data
// This record will be saved to database's ProposalComponentRecord table
// If the ID field has no value, this is a new component should be created,
// while if it has value, this is an existing component should be updated.
type ComponentRequestData struct {
	ID          uint   `json:"id"`
	ComponentId uint   `json:"component_id"`
	Data        string `json:"data"`
}

type CreateProposalData struct {
	Title      string `json:"title"`
	Background string `json:"background"`
	Content    string `json:"content"`
	Components []*ComponentRequestData
}

///////////////////////
// Response data definitions
///////////////////////

type FrontendProposalDetailRecord struct {
	Title      string `json:"title"`
	Background string `json:"background"`
	Content    string `json:"content"`
	State      string `json:"state"`
	Components []*component.ComponentInstance

	// Some user information
	Applicant string `json:"applicant"`
	Reviewer  string `json:"reviewer"`

	// Vote data
	IsApproved bool `json:"is_approved"`

	// Timestamps
	CreateTs int64 `json:"create_ts"`
	UpdateTs int64 `json:"update_ts"`
}
