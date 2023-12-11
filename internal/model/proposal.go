package model

type Proposal struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	Title string

	ContentBlocks []*ProposalBlocks

	Components []*ProposalComponentRecord

	// ProposalVerId used to identify proposal version
	ProposalVerId uint `gorm:"index:proposalVer"`
	Version       uint `gorm:"index:proposalVer"`

	// IPFS CID for the proposal
	IpfsCid string `gorm:"index"`

	ArweaveLink string
}

// ProposalBlocks saves blocks in proposal.
// In proposal the content is built by blocks, each block contains a title and content.
// The content are saved in order in proposal records.
type ProposalBlocks struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	Title   string
	Content string
}

// ProposalComponentRecord saves components in proposal.
// It builds the association between Component and Proposal records, one Component can be used in many Proposals
type ProposalComponentRecord struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

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
