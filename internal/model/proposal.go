package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

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

	// IPFS CID and Arweave hash for the proposal
	IpfsCid     string `gorm:"index"`
	ArweaveHash string `gorm:"index"`

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

func (p *Proposal) GetMetaforoThreadId() int {
	threadIdStr := strings.TrimPrefix(p.ProposalRecordId, "metaforo:")
	threadId, err := strconv.Atoi(threadIdStr)
	if err != nil {
		log.Error().Msgf("Parse metafor thread ID error, proposalRecordId: %s, error: %+v", p.ProposalRecordId, err)
		return 0
	}
	return threadId
}

// BumpUpVersion creates a new proposal record with copied content_blocks, components, vote_records and bumps up proposal version.
// The ArweaveHash will be set to empty string since the file changed
func (p *Proposal) BumpUpVersion(db *gorm.DB) (*Proposal, error) {
	newRecord := p
	newRecord.ID = 0
	newRecord.ArweaveHash = ""
	newRecord.Version += 1

	if err := db.Create(&newRecord).Error; err != nil {
		log.Error().Msgf("create proposal record with data %+v failed. error: %+v", newRecord, err)
		return nil, err
	}

	// Duplicate content blocks and components
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, block := range p.ContentBlocks {
			block.ID = 0
			if err := tx.Create(&block).Error; err != nil {
				log.Error().Msgf("create proposal content block with data %+v failed. error: %+v", block, err)
				return err
			}
		}

		for _, component := range p.Components {
			component.ID = 0
			if err := tx.Create(&component).Error; err != nil {
				log.Error().Msgf("create proposal component block with data %+v failed. error: %+v", component, err)
				return err
			}
		}

		// voteRecords data contains metaforoID, so need to copy to new record, then it can be handled by updateVoteTime API
		for _, voteRecord := range p.VoteRecords {
			voteRecord.ID = 0
			if err := tx.Create(&voteRecord).Error; err != nil {
				log.Error().Msgf("create vote record with data %+v failed. error: %+v", voteRecord, err)
				return err
			}
		}

		return nil
	}); err != nil {
		log.Error().Msgf("create proposal record with data %+v failed. error: %+v", newRecord, err)
		return nil, err
	}

	return newRecord, nil
}

func BuildProposalRecordIdFromMetaforoThreadId(threadId int) string {
	return fmt.Sprintf("metaforo:%d", threadId)
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

	ParentID string

	// Reference ID for proposal and specified version
	ProposalID       uint `gorm:"index"`
	Proposal         *Proposal
	ProposalRecordID uint `gorm:"index"`

	Content string

	// IPFS CID for the proposal
	IpfsCid     string `gorm:"index"`
	ArweaveLink string `gorm:"index"`

	MetaforoCommentId string

	IsHidden bool

	// Indicate whether this comment is a reject comment
	IsRejectComment bool `gorm:"index"`
}

type ProposalCategory struct {
	ID         uint `gorm:"primaryKey"`
	ParentID   uint `gorm:"index"` // Save category hierarchy information
	Name       string
	MetaforoId uint

	IsActive bool
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
	ID         uint `gorm:"primaryKey"`
	GateID     uint
	Title      string
	StartTs    int64 `gorm:"index"`
	EndTs      int64 `gorm:"index"`
	MetaforoID int

	ProposalID uint
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

	Name      string `gorm:"uniqueIndex"`
	Author    string
	Schema    string
	Thumbnail string

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
	Command string
}

type ProposalTemplate struct {
	ID       uint  `gorm:"primaryKey"`
	CreateTs int64 `gorm:"index"`
	UpdateTs int64 `gorm:"index"`

	Name   string
	Schema string
}
