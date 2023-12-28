package proposal

import (
	"errors"

	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api/component"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

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
	Q          string `form:"q"`
}

// ComponentRequestData represents a component request, which contains component name and associated data
// This record will be saved to database's ProposalComponentRecord table
// The Name filed is used to identify which component this
// If the ID field has no value, this is a new component should be created,
// while if it has value, this is an existing component should be updated.
type ComponentRequestData struct {
	ID         uint   `json:"id"`
	AutoAction string `json:"auto_action"`
	Name       string `json:"component_name"`
	Data       string `json:"data"`
}

type CreateOrUpdateProposalData struct {
	Title               string                           `json:"title"`
	ProposalCategoryId  uint                             `json:"proposal_category_id"`
	ContentBlocks       []*FrontendContentBlockRecord    `json:"content_blocks"`
	Components          map[string]*ComponentRequestData `json:"components"`
	VoteGateId          uint                             `json:"vote_gate_id"`
	MetaforoAccessToken string                           `json:"metaforo_access_token"`
	SubmitToMetaforo    bool                             `json:"submit_to_metaforo"`
}

type RejectProposalData struct {
	Reason              string `json:"reason"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

type AddCommentData struct {
	Content string `json:"content"`
	ReplyId int    `json:"reply_id"`

	MetaforoAccessToken string `json:"metaforo_access_token"`
}

///////////////////////
// Response data definitions
///////////////////////

type FrontendProposalListRecord struct {
	ID              uint   `json:"id"`
	Title           string `json:"title"`
	Applicant       string `json:"applicant"`
	ApplicantAvatar string `json:"applicant_avatar"`
	CategoryName    string `json:"category_name"`
	State           string `json:"state"`
	CreateTs        int64  `json:"create_ts"`

	// Vote related state
	// TODO: Vote Gate related logic
	VoteState string `json:"vote_state"`
	IsVoted   bool
}

type FrontendContentBlockRecord struct {
	ID      uint   `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

type FrontendProposalDetailRecord struct {
	ID            uint                           `json:"id"`
	Title         string                         `json:"title"`
	ContentBlocks []*FrontendContentBlockRecord  `json:"content_blocks"`
	State         string                         `json:"state"`
	Components    []*component.ComponentInstance `json:"components"`

	ProposalCategoryId uint `json:"proposal_category_id"`

	// Some user information
	Applicant       string `json:"applicant"`
	ApplicantAvatar string `json:"applicant_avatar"`
	Reviewer        string `json:"reviewer"`
	ReviewerAvatar  string `json:"reviewer_avatar"`

	// Arveave Hash
	Arveave string `json:"arveave"`

	// Reject related data
	IsRejected   bool   `json:"is_rejected"`
	RejectReason string `json:"reject_reason"`

	// Comments
	Comments []any `json:"comments"`

	// Is current user voted for this proposal
	IsVoted bool `json:"is_voted"`

	// Timestamps
	CreateTs int64 `json:"create_ts"`
}

type FrontendProposalCategory struct {
	ID         uint   `json:"id"`
	ParentID   uint   `json:"parent_id"`
	Name       string `json:"name"`
	MetaforoId uint   `json:"metaforo_id"`
}

type FrontendVoteGateResponse struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	TokenAddr string `json:"contract_addr"`
	TokenId   string `json:"token_id"`
	TokenType string `json:"token_type"`
	ChainType string `json:"chain_type"`
}

type UpdateProposalCategoryReq struct {
	ID         uint   `json:"id"`
	ParentID   uint   `json:"parent_id"`
	Name       string `json:"name"`
	MetaforoId string `json:"metaforo_id"`
}

///////////////////////
// Some converter functions
///////////////////////

func ConvertProposalToFrontendDetailRecord(db *gorm.DB, proposal *model.Proposal) (*FrontendProposalDetailRecord, error) {
	var proposalBlocks []*model.ProposalContentBlock
	if err := db.Where(&model.ProposalContentBlock{ProposalID: proposal.ID}).Find(&proposalBlocks).Error; err != nil {
		return nil, err
	}

	var proposalComponentRecords []*model.ProposalComponentRecord
	if err := db.Where(&model.ProposalComponentRecord{ProposalID: proposal.ID}).Find(&proposalComponentRecords).Error; err != nil {
		return nil, err
	}

	proposalContentResponse := lo.Map(proposalBlocks, func(item *model.ProposalContentBlock, _ int) *FrontendContentBlockRecord {
		return &FrontendContentBlockRecord{
			ID:      item.ID,
			Title:   item.Title,
			Content: item.Content,
		}
	})

	proposalComponentResponse := lo.Map(proposalComponentRecords, func(item *model.ProposalComponentRecord, _ int) *component.ComponentInstance {
		return &component.ComponentInstance{
			ID:          item.ID,
			ComponentId: item.ComponentID,
			Schema:      "",
			Data:        item.Data,
			CreateTs:    item.CreateTs,
		}
	})

	metaforoProposal, err := metaforo.GetProposal(proposal.GetMetaforoThreadId(), internal.MetaforoGroupName)
	if err != nil {
		return nil, err
	}
	// Save arveave if not existing in current DB record
	// TODO: Merge duplicated code in Update proposal
	if metaforoProposal.Thread.EditHistory.Lists != nil && len(metaforoProposal.Thread.EditHistory.Lists) > 0 {
		proposal.ArveaveHash = metaforoProposal.Thread.EditHistory.Lists[0].Arweave
		db.Save(&proposal)
	}

	// TODO: Query UserVoteRecord and update isVoted field

	// TODO: Get rejected comments if have
	rejectedComment := model.ProposalComment{}
	err = db.Model(model.ProposalComment{}).Where("proposal_id = ? AND is_reject_comment = ?", proposal.ID, true).First(&rejectedComment).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	// TODO: Optimize the avatar query
	var applicantAvatarLink string
	db.Model(model.User{}).Where("wallet = ?", common.FormatUserWallet(proposal.Applicant)).Select("avatar").First(&applicantAvatarLink)

	return &FrontendProposalDetailRecord{
		ID:                 proposal.ID,
		Title:              proposal.Title,
		ContentBlocks:      proposalContentResponse,
		ProposalCategoryId: proposal.ProposalCategoryID,
		State:              model.ProposalStateName[proposal.State],
		Components:         proposalComponentResponse,
		Applicant:          proposal.Applicant,
		ApplicantAvatar:    applicantAvatarLink,
		IsRejected:         proposal.State == int(model.ProposalStateRejected),
		RejectReason:       rejectedComment.Content,
		Arveave:            proposal.ArveaveHash,
		Comments:           metaforoProposal.Thread.Posts,
		CreateTs:           proposal.CreateTs,
	}, nil
}
