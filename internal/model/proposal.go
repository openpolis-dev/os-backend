package model

type ProposalState int

var ProposalStateName = []string{
	"draft", "withdrawn", "voting", "passed", "failed", "rejected",
}

const (
	ProposalStateDraft ProposalState = iota
	ProposalStateWithdrawn
	ProposalStateVoting
	ProposalStatePassed
	ProposalStateFailed
	ProposalStateRejected
)

var ProposalStateIdNameMapping = map[string]ProposalState{
	"draft":     ProposalStateDraft,
	"withdrawn": ProposalStateWithdrawn,
	"voting":    ProposalStateVoting,
	"passed":    ProposalStatePassed,
	"failed":    ProposalStateFailed,
	"rejected":  ProposalStateRejected,
}

type Proposal struct {
	ID uint `gorm:"primaryKey"`

	// Only CreateTs for Proposal record since proposal is non-editable
	// Each proposal will be a new record in DB, and the ProposalId field will be used to identify the proposal.
	// While querying, the proposal with max Version will be returned in API,
	// and the historical versions can only be returned in detailed request
	CreateTs int64 `gorm:"index"`

	// int format state, refer ProposalState type const for name and value mapping
	State int `gorm:"index"`

	Title string

	ContentBlocks []*ProposalBlocks

	Components []*ProposalComponentRecord

	ProposalCategoryID uint `gorm:"index"`
	ProposalCategory   ProposalCategory

	// ProposalRecordId used to identify proposal version
	ProposalRecordId string `gorm:"index:proposalVer"`
	Version          uint   `gorm:"index:proposalVer"`

	// IPFS CID and Arveave hash for the proposal
	IpfsCid     string `gorm:"index"`
	ArveaveHash string `gorm:"index"`

	Applicant string `gorm:"index"`

	// Fields for poll state
	PollStartTs int64 `gorm:"index"`
	PollEndTs   int64 `gorm:"index"`
}

// ProposalBlocks saves blocks in proposal.
// In proposal the content is built by blocks, each block contains a title and content.
// The content are saved in order in proposal records.
type ProposalBlocks struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`

	Title      string
	Content    string
	ProposalID uint
}

// ProposalComponentRecord saves components in proposal.
// It builds the association between Component and Proposal records, one Component can be used in many Proposals
type ProposalComponentRecord struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`

	ComponentId uint
	ProposalId  uint
	Data        string // Data field stores data for the component
}

// ProposalComment saves comments for proposal, the comment is bind to specified version of proposal
type ProposalComment struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	// Reference ID for proposal and specified version
	ProposalId    uint `gorm:"index"`
	ProposalVerId uint `gorm:"index"`

	Content string

	// IPFS CID for the proposal
	IpfsCid string `gorm:"index"`

	ArweaveLink string
}

type ProposalCategory struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

type ProposalAuditLog struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`
}

// Component defines the automation actions should be done and related data structure
// The required data is defined in schema, and the actions are defined by action ids
type Component struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	Name   string `gorm:"uniqueIndex"`
	Editor string
	Schema string

	ApproveActionId uint // ApproveActionId indicates the action will be executed when the component is approved
	RejectActionId  uint // RejectActionId indicates the action will be executed when the component is rejected
}

// ComponentAction saves automate actions will be executed
// The Command field currently saves predefined command name that can be recognized by code and launch pre defined automate action.
type ComponentAction struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	// Command field saves the automation commands will be executed by component
	// Currently it saves the command name which is implemented in code.
	Command string
}
