package model

import "strings"

type ProposalState int

const (
	ProposalStatePendingSubmit ProposalState = iota // PendingSubmit means the proposal is still in personal box, no one else can view it.
	ProposalStateDraft
	ProposalStateWithdrawn
	ProposalStateRejected
	ProposalStateApproved

	ProposalStateVoting
	ProposalStateVotePassed
	ProposalStateVoteFailed
)

var ProposalStateIdNameMapping = map[string]ProposalState{
	"pending_submit": ProposalStatePendingSubmit,
	"draft":          ProposalStateDraft,
	"withdrawn":      ProposalStateWithdrawn,
	"rejected":       ProposalStateRejected,
	"approved":       ProposalStateApproved,
	"voting":         ProposalStateVoting,
	"vote_passed":    ProposalStateVotePassed,
	"vote_failed":    ProposalStateVoteFailed,
}

var ProposalStateName = []string{
	"pending_submit",
	"draft",
	"withdrawn",
	"rejected",
	"approved",
	"voting",
	"vote_passed",
	"vote_failed",
}

type Proposal struct {
	ID uint `gorm:"primaryKey"`

	// Only CreateTs for Proposal record since proposal is non-editable
	// Each proposal will be a new record in DB, and the ProposalID field will be used to identify the proposal.
	// While querying, the proposal with max Version will be returned in API,
	// and the historical versions can only be returned in detailed request
	CreateTs int64 `gorm:"index"`

	// int format state, refer ProposalState type const for name and value mapping
	State int `gorm:"index"`

	Title string

	ContentBlocks []*ProposalContentBlock

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

	VoteRecords []*ProposalVoteRecord

	IsHidden bool
}

func (p *Proposal) StateName() string {
	return ProposalStateName[p.State]
}

// CanBeUpdated returns bool value indicates whether this proposal can be updated.
// The record can be updated if value returned is true
// Only proposal in those states can be updated: PendingSubmit, Withdrawn and Rejected
func (p *Proposal) CanBeUpdated() bool {
	return p.State == int(ProposalStatePendingSubmit) ||
		p.State == int(ProposalStateWithdrawn) ||
		p.State == int(ProposalStateRejected)
}

// CanBeUpdatedBy returns bool value indicates whether this proposal can be updated by specified user wallet
// The logic is the proposal in updatable state and the applicant equals to passed in wallet
func (p *Proposal) CanBeUpdatedBy(wallet string) bool {
	return p.CanBeUpdated() && strings.EqualFold(p.Applicant, wallet)
}

// ProposalContentBlock saves blocks in proposal.
// In proposal the content is built by blocks, each block contains a title and content.
// The content are saved in order in proposal records.
type ProposalContentBlock struct {
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

	ComponentID uint
	ProposalID  uint
	Data        string // Data field stores data for the component
}

// ProposalComment saves comments for proposal, the comment is bind to specified version of proposal
type ProposalComment struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	ParentID string

	// Reference ID for proposal and specified version
	ProposalID uint `gorm:"index"`
	Proposal   *Proposal

	Content string

	// IPFS CID for the proposal
	IpfsCid     string `gorm:"index"`
	ArweaveLink string `gorm:"index"`

	MetaforoPostId string

	IsHidden bool

	// Indicate whether this comment is a reject comment
	IsRejectComment bool `gorm:"index"`
}

type ProposalCategory struct {
	ID         uint `gorm:"primaryKey"`
	ParentID   uint `gorm:"index"` // Save category hierarchy information
	Name       string
	MetaforoId uint
}

// ProposalVoteGate saves the token requirements to vote
type ProposalVoteGate struct {
	ID           uint   `gorm:"primaryKey"`
	ChainType    int    `gorm:"index:assetAttr"`
	TokenType    int    `gorm:"index:assetAttr"`
	TokenAddress string `gorm:"index"`
	TokenId      string
	MetaforoId   uint

	Name string // Name of the vote gate
}

func (ppg *ProposalVoteGate) TokenTypeName() string {
	switch ppg.TokenType {
	case 0:
		return "ERC20"
	case 1:
		return "ERC721"
	case 2:
		return "ERC1155"
	default:
		return "Unknown"
	}
}

func (ppg *ProposalVoteGate) ChainName() string {
	switch ppg.ChainType {
	case 1:
		return "Polygon"
	case 8:
		return "ETH"
	case 7:
		return "Bsc"
	case 9:
		return "Arbitrum"
	default:
		return "Unknown"
	}
}

type ProposalAuditLog struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`
}

// ProposalVoteRecord saves vote object and associated to specified Proposal
type ProposalVoteRecord struct {
	ID         User `gorm:"primary Key"`
	GateID     uint
	Title      string
	VoteGate   *ProposalVoteGate
	StartTs    int64 `gorm:"index"`
	EndTs      int64 `gorm:"index"`
	MetaforoID int
}

// ProposalUserVoteRecord saves user vote record
type ProposalUserVoteRecord struct {
	ID         uint   `gorm:"primaryKey"`
	UserWallet string `gorm:"index"`
	ProposalID uint   `gorm:"index"`
	Option     string // Which option the user selected

	VoteTs int64 `gorm:"index"`
}

// Component defines the automation actions should be done and related data structure
// The required data is defined in schema, and the actions are defined by action ids
type Component struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	Name      string `gorm:"uniqueIndex"`
	Author    string
	Schema    string
	Thumbnail string

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
