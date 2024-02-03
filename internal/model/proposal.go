package model

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ProposalState int
type ProposalTemplateType int

const (
	ProposalStatePendingSubmit ProposalState = iota // PendingSubmit means the proposal is still in personal box, no one else can view it.
	ProposalStateDraft
	ProposalStateWithdrawn
	ProposalStateRejected
	ProposalStateApproved

	ProposalStateVoting
	ProposalStateVotePassed
	ProposalStateVoteFailed

	// ProposalStatePendingExecution indicates vote has passed, and the execution command has been added into cronjob table
	// In this state, the proposal should have execution_ts field settled, which can be fetched from cronjob table.
	ProposalStatePendingExecution
	ProposalStateExecuted

	// ProposalStateExecutionFailed indicates the execution of cronjob failed, need to handle it manually
	ProposalStateExecutionFailed

	// ProposalStateVetoed indicates the proposal has been voted by city hall proposal.
	// TODO: In this case, there should be some field to reflect this relationship
	ProposalStateVetoed
)

const (
	ProposalTemplateTypeNewProject     ProposalTemplateType = 10
	ProposalTemplateTypeCloseProject                        = 11
	ProposalTemplateTypeRejectProposal                      = 20
)

var ProposalStateIdNameMapping = map[string]ProposalState{
	"pending_submit":    ProposalStatePendingSubmit,
	"draft":             ProposalStateDraft,
	"withdrawn":         ProposalStateWithdrawn,
	"rejected":          ProposalStateRejected,
	"approved":          ProposalStateApproved,
	"voting":            ProposalStateVoting,
	"vote_passed":       ProposalStateVotePassed,
	"vote_failed":       ProposalStateVoteFailed,
	"pending_execution": ProposalStatePendingExecution,
	"executed":          ProposalStateExecuted,
	"execution_failed":  ProposalStateExecutionFailed,
	"vetoed":            ProposalStateVetoed,
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
	"pending_execution",
	"executed",
	"execution_failed",
	"vetoed",
}

// MetaforoUser saves user mapping between OS and metaforo
type MetaforoUser struct {
	ID uint `gorm:"primaryKey"`

	// user id in metaforo system
	MetaforoUserId int `gorm:"index"`

	// UserWallet saves user wallet address
	UserWallet string `gorm:"index"`

	// Groups saves groups user joined in metaforo
	Groups datatypes.JSON
}

type VoteTimeProperties struct {
	PublicitySecond        int64
	VoteDurationSecond     int64
	PendingExecutionSecond int64
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

	//  Fields for versioned proposals
	// ProposalRecordId is built from metaforo thread ID, which should be kept same in various versions
	ProposalRecordId string `gorm:"index:proposalVer"`
	Version          uint   `gorm:"index:proposalVer"`

	// IPFS CID and Arweave hash for the proposal
	IpfsCid     string `gorm:"index"`
	ArweaveHash string `gorm:"index"`

	Applicant string `gorm:"index"`

	// VoteType indicates the type of attached vote for this proposal, the available values are:
	// - ProposalVoteTypeNone
	// - ProposalVoteTypeNumericAvg
	// - ProposalVoteTypeDecision
	// - ProposalVoteTypeCustomerDefinedAlwaysPassed
	VoteType    int
	VoteRecords []*ProposalVoteRecord

	IsHidden bool

	CanBeVetoed bool

	// If proposal is created from template, this value will be set
	ProposalTemplateID *uint `gorm:"index"`
	ProposalTemplate   *ProposalTemplate

	IsBasedOnCustomTemplate bool

	// Proposal category, this field is used for template created w/o template,
	// while for proposal created with template, the category data wil be updated by value in template record
	ProposalCategoryID uint `gorm:"index"`
	ProposalCategory   ProposalCategory

	VoteTimeProperties
}

func (p *Proposal) StateName() string {
	return ProposalStateName[p.State]
}

func (p *Proposal) PublicityDuration() time.Duration {
	return time.Duration(p.PublicitySecond) * time.Second
}

func (p *Proposal) VoteDuration() time.Duration {
	return time.Duration(p.VoteDurationSecond) * time.Second
}

func (p *Proposal) TaskStartDelay() time.Duration {
	return time.Duration(p.PendingExecutionSecond) * time.Second
}

func (p *Proposal) IsInFinState() bool {
	return p.State == int(ProposalStateVoteFailed) ||
		p.State == int(ProposalStateExecuted) ||
		p.State == int(ProposalStateVetoed)
}

// StateIsUpdatable returns bool value indicates whether this proposal can be updated.
// The record can be updated if value returned is true
// Only proposal in those states can be updated: PendingSubmit, Withdrawn and Rejected
func (p *Proposal) StateIsUpdatable() bool {
	return p.State == int(ProposalStatePendingSubmit) ||
		p.State == int(ProposalStateWithdrawn) ||
		p.State == int(ProposalStateRejected)
}

// CanBeUpdatedBy returns bool value indicates whether this proposal can be updated by specified user wallet
// The logic is the proposal in updatable state and the applicant equals to passed in wallet
func (p *Proposal) CanBeUpdatedBy(wallet string) bool {
	return p.StateIsUpdatable() && strings.EqualFold(p.Applicant, wallet)
}

func (p *Proposal) GetMetaforoThreadId() int {
	threadIdStr := strings.TrimPrefix(p.ProposalRecordId, "metaforo:")
	threadId, err := strconv.Atoi(threadIdStr)
	if err != nil {
		log.Error().Msgf("Parse metafor thread ID error, proposalRecordId: %s, error: %+v", p.ProposalRecordId, err)
		return 0
	}
	return threadId
}

// BumpUpVersion creates a new proposal record with bumps up proposal version.
// The ArweaveHash will be set to empty string since the file changed
// The content_blocks and components will be cloned outside this function
func (p *Proposal) BumpUpVersion(db *gorm.DB) (*Proposal, error) {
	newRecord := p
	newRecord.ID = 0
	newRecord.CreateTs = time.Now().UTC().Unix()
	newRecord.ArweaveHash = ""
	newRecord.Version += 1
	newRecord.State = int(ProposalStateDraft) // record with bumped up version should be in Draft state

	if err := db.Create(&newRecord).Error; err != nil {
		log.Error().Msgf("create proposal record with data %+v failed. error: %+v", newRecord, err)
		return nil, err
	}

	return newRecord, nil
}

func BuildProposalRecordIdFromMetaforoThreadId(threadId int) string {
	return fmt.Sprintf("metaforo:%d", threadId)
}

// ProposalContentBlock saves blocks in proposal.
// The block contains brief text block and components block.
// The brief text block contains title and content.
// The component block can contain multiple components, with a title field
type ProposalContentBlock struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`

	Title         string
	Content       string
	ProposalID    uint
	Type          string
	ComponentList string
}

// ProposalComponentRecord saves components in proposal.
// It builds the association between ProposalComponent and Proposal records, one ProposalComponent can be used in many Proposals
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

	ParentID uint

	AuthorWallet string `gorm:"index"`

	// Reference ID for proposal and specified version
	ProposalID       uint `gorm:"index"`
	Proposal         *Proposal
	ProposalRecordID string `gorm:"index"`

	Content string

	// IPFS CID for the proposal
	IpfsCid     string `gorm:"index"`
	ArweaveLink string `gorm:"index"`

	MetaforoCommentId int

	IsHidden bool

	// Indicate whether this comment is a reject comment
	IsRejectComment bool `gorm:"index"`
}

type ProposalCategory struct {
	ID         uint `gorm:"primaryKey"`
	ParentID   uint `gorm:"index"` // Save category hierarchy information
	Name       string
	MetaforoId uint

	// Specify the display index while returning to frontend
	DisplayIndex int `gorm:"index"`

	ProposalVoteGateId *uint
	ProposalVoteGate   *ProposalVoteGate

	IsActive bool

	CanBeVetoed bool

	// CategoryIdForCloseProject saves which category should be queried when trying to get project can be closed
	CategoryIdForCloseProject uint

	VoteTimeProperties
}

// ProposalVoteGate saves the token requirements to vote
// TODO: Add logic to the record and check
type ProposalVoteGate struct {
	ID           uint   `gorm:"primaryKey"`
	ChainType    int    `gorm:"index:assetAttr"`
	TokenType    int    `gorm:"index:assetAttr"`
	TokenAddress string `gorm:"index"`
	TokenId      string
	Amount       string // Amount for specified gate
	MetaforoId   uint

	Name string // Name of the vote gate
}

func (pvg *ProposalVoteGate) TokenTypeName() string {
	switch pvg.TokenType {
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

func (pvg *ProposalVoteGate) ChainName() string {
	switch pvg.ChainType {
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

var (
	ProposalVoteTypeNone                        int = 0
	ProposalVoteTypeDecision                        = 1
	ProposalVoteTypeNumericAvg                      = 2  // This means if the result contains same vote value, use the average for result
	ProposalVoteTypeNumericSingle                   = 3  // This means if the result contains same vote value, mark vote as failed
	ProposalVoteTypeCustomerDefinedEqualFailed      = 98 // This is custom voting, but if more than one max value, the vote failed
	ProposalVoteTypeCustomerDefinedAlwaysPassed     = 99 // Custom vote, always passed
)

// ProposalVoteRecord saves vote record and associated to specified Proposal
// In metaforo, this record is mapped from its `poll` field
// Regards the detailed vote option, only voter count data will be saved into the *Count fields
type ProposalVoteRecord struct {
	ID         uint `gorm:"primaryKey"`
	GateID     uint // Not using now, find way to get it from proposal category
	Title      string
	StartTs    int64 `gorm:"index"`
	EndTs      int64 `gorm:"index"`
	MetaforoID int   `gorm:"index"` // Poll id from metaforo

	OptionType int
	Options    []*ProposalVoteOptionRecord

	// Indicates whether the vote has passed
	IsVotePassed bool

	ArweaveHash string

	ProposalID uint

	// VoteType indicates the type of attached vote for this proposal, the available values are:
	// - ProposalVoteTypeNone
	// - ProposalVoteTypeNumericAvg
	// - ProposalVoteTypeDecision
	// - ProposalVoteTypeCustomerDefinedAlwaysPassed
	VoteType int
}

// ProposalVoteOptionRecord saves option used in proposal vote record, it contains a
type ProposalVoteOptionRecord struct {
	ID uint `gorm:"primaryKey"`

	// Foreign key
	ProposalVoteRecordId uint
	//
	//ProposalId uint

	// Metaforo related data
	// Text field is used to
	Text           string
	MetaforoID     int
	MetaforoVoteID int

	// Value is used in for automation tasks related to this
	Value string

	VoterCount int
}

// GetPredefinedVoteOptionValue returns the predefined value for vote option.
// The logic for this function is check Text field from record and try to find it in predefined internal variables
func GetPredefinedVoteOptionValue(optLabel string, voteType int) string {
	var optBucket map[string]string
	switch voteType {
	case ProposalVoteTypeNumericAvg, ProposalVoteTypeNumericSingle:
		optBucket = internal.ProposalNumericVoteOptionsMap
	case ProposalVoteTypeDecision:
		optBucket = internal.ProposalDecisionVoteOptionsMap
	default:
		log.Warn().Msgf("unknown vote type: %d, request label is: %s", voteType, optLabel)
		return "0"
	}

	if value, found := optBucket[optLabel]; found {
		return value
	} else {
		return "0"
	}
}

// ProposalUserVoteRecord saves user vote record
type ProposalUserVoteRecord struct {
	ID         uint   `gorm:"primaryKey"`
	UserWallet string `gorm:"index"`
	ProposalID uint   `gorm:"index"`
	Option     string // Which option the user selected

	VoteTs int64 `gorm:"index"`
}

// ProposalComponent defines the automation actions should be done and related data structure
// The required data is defined in schema, and the actions are defined by action ids
type ProposalComponent struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	Name          string `gorm:"uniqueIndex"`
	Author        string
	Schema        string
	Thumbnail     string
	ScreenshotUri string

	IsHidden bool

	ApproveActionId uint // ApproveActionId indicates the action will be executed when the component is approved
	RejectActionId  uint // RejectActionId indicates the action will be executed when the component is rejected
}

// ProposalComponentAction saves automate actions will be executed
// The Command field currently saves predefined command name that can be recognized by code and launch pre defined automate action.
type ProposalComponentAction struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	// Command field saves the automation commands will be executed by component
	// Currently it saves the command name which is implemented in code.
	Command string `gorm:"uniqueIndex"`
}

type ProposalTemplate struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	Name string `gorm:"uniqueIndex"`

	// RuleDesc saves a description for template rule
	RuleDesc string

	// VoteType indicates the type of attached vote for this proposal, the available values are:
	// - ProposalVoteTypeNone
	// - ProposalVoteTypeNumericAvg
	// - ProposalVoteTypeDecision
	// - ProposalVoteTypeCustomerDefinedAlwaysPassed
	VoteType int

	// Type is used to filter proposal template, this field is not set to all templates, but only specified template records are required
	Type ProposalTemplateType

	ScreenshotUri string

	// ContentSchema saves content blocks used for this template, each block contains one title and one content field
	ContentSchema string

	// Components saves component used in this template,
	Components []*ProposalComponent `gorm:"many2many:template_components;"`

	// Proposal category
	ProposalCategoryID uint `gorm:"index"`
	ProposalCategory   *ProposalCategory

	// UseTemplateGates saves gate for creating proposal based on this template
	UseTemplateGates []*ProposalVoteGate `gorm:"many2many:template_usage_gates;"`

	//////////
	// Vote related information
	//////////
	// VoteGates saves gate info of voting on proposal created by this template
	VoteGates []*ProposalVoteGate `gorm:"many2many:proposal_voting_gates;"`

	VoteTimeProperties

	// Specify the display index while returning to frontend
	DisplayIndex int `gorm:"index"`

	// Identify whether this template is custom template
	IsCustomTemplate bool
}
