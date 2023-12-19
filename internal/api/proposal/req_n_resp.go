package proposal

import "github.com/theseed-labs/os-backend/internal/api/component"

///////////////////////
// Request data definitions
///////////////////////

type QueryParams struct {
	Page       int    `form:"page"`
	Size       int    `form:"size"`
	SortField  string `form:"sort_field"`
	SortOrder  string `form:"sort_order"`
	State      string `form:"state"`
	CategoryId uint   `form:"category_id"`
}

// ComponentRequestData represents a component request, which contains component name and associated data
// This record will be saved to database's ProposalComponentRecord table
// The Name filed is used to identify which component this
// If the ID field has no value, this is a new component should be created,
// while if it has value, this is an existing component should be updated.
type ComponentRequestData struct {
	ID         uint   `json:"id"`
	AutoAction string `json:"auto_action"`
	Name       uint   `json:"component_name"`
	Data       string `json:"data"`
}

type CreateProposalData struct {
	ProposalId          string `json:"proposal_id"`
	Title               string `json:"title"`
	Background          string `json:"background"`
	Content             string `json:"content"`
	Components          map[string]*ComponentRequestData
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

///////////////////////
// Response data definitions
///////////////////////

type FrontendProposalListRecord struct {
	ID           uint   `json:"id"`
	Title        string `json:"title"`
	Applicant    string `json:"applicant"`
	CategoryName string `json:"category_name"`
	State        string `json:"state"`
	CreateTs     int64  `json:"create_ts"`
	PollState    string `json:"poll_state"`
}

type FrontendProposalDetailRecord struct {
	ID         uint   `json:"id"`
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
