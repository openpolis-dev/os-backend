package proposal_inject

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api/project"
	"github.com/theseed-labs/os-backend/internal/model"
)

var updateLock sync.RWMutex
var updatingProposals = make(map[uint]bool)

type proposalComponentActions struct {
	ProposalComponentRecordId int    `json:"proposal_component_record_id"`
	ComponentParams           string `json:"component_params"`
	ApproveActionName         string `json:"approve_action_name"`
	RejectActionName          string `json:"reject_action_name"`
}

type budgetComponentDataP1 struct {
	Amount     string `json:"amount"`
	Applicant  string `json:"applicant"`
	ProposalId string `json:"proposal_id"`
	AssetInfo  struct {
		Name string `json:"name"`
	} `json:"typeTest"`
}

func (r *budgetComponentDataP1) prepareBudgetRecords(proposalId uint) []*model.ProjectBudget {
	totalBudgetAmount := decimal.RequireFromString(r.Amount)
	advancedRatio := decimal.Zero
	totalAdvanceAmount := decimal.Zero

	return []*model.ProjectBudget{
		{
			ProposalID:          proposalId,
			ProjectID:           0,
			AssetName:           r.AssetInfo.Name,
			TotalAmount:         totalBudgetAmount,
			UsedAmount:          decimal.Zero,
			RemainAmount:        totalBudgetAmount,
			AdvanceRatio:        advancedRatio,
			TotalAdvanceAmount:  totalAdvanceAmount,
			UsedAdvanceAmount:   decimal.Zero,
			RemainAdvanceAmount: totalAdvanceAmount,
			CreateTs:            model.GetCurrentUtcEpochSecond(),
			UpdateTs:            model.GetCurrentUtcEpochSecond(),
		},
	}
}

type budgetComponentData struct {
	Applicant  string `json:"applicant"`
	BudgetList []struct {
		Amount      string `json:"amount"`
		Description string `json:"description"`
		Proportion  string `json:"proportion"`
		AssetInfo   struct {
			Name string `json:"name"`
		} `json:"typeTest"`
	} `json:"budgetList"`
	ProposalId string `json:"proposal_id"`
}

func (r *budgetComponentData) prepareBudgetRecords(proposalId uint) []*model.ProjectBudget {
	projectBudgetRcds := make([]*model.ProjectBudget, 0)
	for _, r := range r.BudgetList {
		totalBudgetAmount := decimal.RequireFromString(r.Amount)
		advancedRatio := decimal.RequireFromString(r.Proportion).Div(decimal.NewFromInt(100))
		totalAdvanceAmount := totalBudgetAmount.Mul(advancedRatio).Round(0)

		projectBudgetRcds = append(projectBudgetRcds, &model.ProjectBudget{
			ProposalID:          proposalId,
			ProjectID:           0,
			AssetName:           r.AssetInfo.Name,
			TotalAmount:         totalBudgetAmount,
			UsedAmount:          decimal.Zero,
			RemainAmount:        totalBudgetAmount,
			AdvanceRatio:        advancedRatio,
			TotalAdvanceAmount:  totalAdvanceAmount,
			UsedAdvanceAmount:   decimal.Zero,
			RemainAdvanceAmount: totalAdvanceAmount,
			CreateTs:            model.GetCurrentUtcEpochSecond(),
			UpdateTs:            model.GetCurrentUtcEpochSecond(),
		})
	}

	return projectBudgetRcds
}

type commonCreateProjectRelatedData struct {
	Desc string `json:"description"`
}

const ListProposalsSQL = `
SELECT p.id,
       p.title,
       lower(p.applicant) as applicant,
       u.avatar as applicant_avatar,
       pc.name  as category_name,
       p.create_ts,
       p.sip,
       p.version,
       p.state as state_id
FROM proposals p
         JOIN (SELECT proposal_record_id, MAX(version) AS max_version
               FROM proposals
               GROUP BY proposal_record_id) t2
              ON p.proposal_record_id = t2.proposal_record_id AND p.version = t2.max_version
         JOIN proposal_categories pc ON p.proposal_category_id = pc.id
         JOIN users u ON p.applicant = u.wallet`

const ListProposalsSQLForGettingCreatingProjectProposal = `
SELECT p.id,
       p.title,
       lower(p.applicant) as applicant,
       u.avatar as applicant_avatar,
       pc.name  as category_name,
       p.create_ts,
       p.sip,
       projects.status as project_status,
       p.version,
       p.state as state_id
FROM proposals p
         JOIN (SELECT proposal_record_id, MAX(version) AS max_version
               FROM proposals
               GROUP BY proposal_record_id) t2
              ON p.proposal_record_id = t2.proposal_record_id AND p.version = t2.max_version
         JOIN proposal_categories pc ON p.proposal_category_id = pc.id
         JOIN users u ON p.applicant = u.wallet
         JOIN projects ON projects.s_ip = p.sip::text`

const QueryMetaforoUserWithOsUserBaseSQL = `
SELECT u.wallet            AS wallet,
       mu.metaforo_user_id AS metaforo_user_id,
       u.id                AS os_user_id,
       u.avatar            AS os_avatar,
       u.name              AS os_username
FROM users u
         INNER JOIN metaforo_users mu ON u.wallet = mu.user_wallet`

const QueryProposalWithJointUserBaseSQL = `
SELECT p.title AS title,
u.wallet            AS wallet,
u.name AS os_username,
p.create_ts AS create_ts,
p.arweave_hash as arweave
FROM users u INNER JOIN proposals p ON u.wallet = p.applicant`

type JointMetaforoAndOsUser struct {
	Wallet string `json:"wallet"`

	MetaforoUserID int `json:"metaforo_user_id"`

	OsUserID   int    `json:"os_user_id"`
	OsAvatar   string `json:"os_avatar"`
	OsUserName string `json:"os_user_name"`
}

const QueryComponentActionNameBaseSQL = `
select pcr.id as proposal_component_record_id,
       pcr.data as component_params,
       approve_pca.command as approve_action_name,
       reject_pca.command  as reject_action_name
from proposal_component_records pcr
         join proposal_components pc on pcr.component_id = pc.id
         join proposal_component_actions approve_pca on pc.approve_action_id = approve_pca.id
         join proposal_component_actions reject_pca on pc.reject_action_id = reject_pca.id`

///////////////////////
// Request data definitions
///////////////////////

type ListProposalQueryParams struct {
	Page          int    `form:"page"`
	Size          int    `form:"size"`
	SortField     string `form:"sort_field"`
	SortOrder     string `form:"sort_order"`
	State         string `form:"state"`
	CategoryId    uint   `form:"category_id"`
	PendingSubmit int    `form:"pending_submit"`
	Q             string `form:"q"`
	Sip           string `form:"sip"`
}

// ComponentRequestData represents a component request, which contains component name and associated data
// This record will be saved to database's ProposalComponentRecord table
// The Name filed is used to identify which component this
// If the ID field has no value, this is a new component should be created,
// while if it has value, this is an existing component should be updated.
type ComponentRequestData struct {
	ID         uint           `json:"id"`
	AutoAction string         `json:"auto_action"`
	Name       string         `json:"name"`
	Data       map[string]any `json:"data"`
}

type CreateOrUpdateProposalData struct {
	TemplateId              uint                          `json:"template_id"`
	Title                   string                        `json:"title"`
	ContentBlocks           []*FrontendContentBlockRecord `json:"content_blocks"`
	Components              []*ComponentRequestData       `json:"components"`
	MetaforoAccessToken     string                        `json:"metaforo_access_token"`
	SubmitToMetaforo        bool                          `json:"submit_to_metaforo"`
	EditorType              int                           `json:"editor_type"`
	VoteOptions             []string                      `json:"vote_options"`
	IsMultipleVote          bool                          `json:"is_multiple_vote"`
	CreateProjectProposalId uint                          `json:"create_project_proposal_id"`
}

type RejectProposalData struct {
	Reason              string `json:"reason"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

type AddCommentData struct {
	Content                  string `json:"content"`
	ReplyToMetaforoCommentId int    `json:"reply_id"`
	MetaforoAccessToken      string `json:"metaforo_access_token"`
	EditorType               int    `json:"editor_type"`
}

type EditCommentData struct {
	Content             string `json:"content"`
	MetaforoCommentId   int    `json:"post_id"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
	EditorType          int    `json:"editor_type"`
}

type DeleteCommentData struct {
	MetaforoCommentId   int    `json:"post_id"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

type CastVoteData struct {
	MetaforoVoteId      int    `json:"vote_id"`
	MetaforoVoteOptions []int  `json:"options"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

type RevokeVoteData struct {
	MetaforoVoteId      int    `json:"vote_id"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

type CloseVoteRequest struct {
	MetaforoVoteId      int    `json:"vote_id"`
	MetaforoAccessToken string `json:"metaforo_access_token"`
}

///////////////////////
// Response data definitions
///////////////////////

// ComponentInstance indicates the component be added into proposal
// It is generated from the Component object, and includes the data filled in proposal
type ComponentInstance struct {
	ID            uint   `json:"id"`
	ComponentId   uint   `json:"component_id"`
	ComponentName string `json:"name"`
	Schema        string `json:"schema"`
	Data          string `json:"data"`

	CreateTs int64 `json:"create_ts"`
}

type FrontendProposalListRecord struct {
	ID              uint   `json:"id"`
	Title           string `json:"title"`
	Applicant       string `json:"applicant"`
	ApplicantAvatar string `json:"applicant_avatar"`
	CategoryName    string `json:"category_name"`
	State           string `json:"state"`
	StateId         int    `json:"-"`
	CreateTs        int64  `json:"create_ts"`
	Version         uint   `json:"version"`
	Sip             int    `json:"sip"`

	// Vote related state
	// TODO: Vote Gate related logic
	VoteState string `json:"-"`
	IsVoted   bool   `json:"is_voted"`
	CanVote   bool   `json:"can_vote"`
}

type FrontendContentBlockRecord struct {
	ID            uint   `json:"id"`
	Title         string `json:"title"`
	Content       string `json:"content"`
	Type          string `json:"type"`
	ComponentList string `json:"name"`
}

type FrontendProposalEditHistories struct {
	TotalCount int                                  `json:"total_count"`
	Lists      []*FrontendProposalEditHistoryRecord `json:"lists"`
}

type FrontendProposalEditHistoryRecord struct {
	Title      string `json:"title"`
	Wallet     string `json:"wallet"`
	OsUsername string `json:"os_username"`
	CreateTs   int64  `json:"create_ts"`
	Arweave    string `json:"arweave"`
}

type FrontendProposalCommentRecord struct {
	MetaforoPostId      int                              `json:"metaforo_post_id"`
	Content             string                           `json:"content"`
	Wallet              string                           `json:"wallet"`
	Avatar              string                           `json:"avatar"`
	ReplyMetaforoPostId int                              `json:"reply_metaforo_post_id"`
	Children            []*FrontendProposalCommentRecord `json:"children"`

	ProposalTitle       string `json:"proposal_title"`
	ProposalTs          int64  `json:"proposal_ts"`
	ProposalArweaveHash string `json:"proposal_arweave_hash"`
	CreatedTs           int64  `json:"created_ts"`
	Deleted             bool   `json:"deleted"`

	IsRejected bool `json:"is_rejected"` // indicate whether this comment is a rejected comment
}

type FrontendProposalVoteOptionRecord struct {
	ID         uint   `json:"id"`
	Label      string `json:"label"`
	MetaforoId int    `json:"metaforo_id"`
}

type FrontendProposalDetailRecord struct {
	ID            uint                          `json:"id"`
	Title         string                        `json:"title"`
	ContentBlocks []*FrontendContentBlockRecord `json:"content_blocks"`
	State         string                        `json:"state"`
	Components    []*ComponentInstance          `json:"components"`

	ProposalCategoryId uint `json:"proposal_category_id"`

	// Some user information
	Applicant       string `json:"applicant"`
	ApplicantAvatar string `json:"applicant_avatar"`
	Reviewer        string `json:"reviewer"`
	ReviewerAvatar  string `json:"reviewer_avatar"`

	Sip int `json:"sip"`

	// Arweave Hash
	Arweave string `json:"arweave"`

	// Reject related data
	IsRejected              bool   `json:"is_rejected"`
	RejectReason            string `json:"reject_reason"`
	RejectTs                int64  `json:"reject_ts"`
	RejectMetaforoCommentId int    `json:"reject_metaforo_comment_id"`

	Histories *FrontendProposalEditHistories `json:"histories"`

	// Comments
	CommentCount int                              `json:"comment_count"`
	Comments     []*FrontendProposalCommentRecord `json:"comments"`

	// Vote
	Votes    any                       `json:"votes"`
	VoteGate *FrontendVoteGateResponse `json:"vote_gate"`

	VoteType int `json:"vote_type"`

	IsMultipleVote bool `json:"is_multiple_vote"`

	OsVoteOptions []*FrontendProposalVoteOptionRecord `json:"os_vote_options"`

	// Is current user voted for this proposal
	IsVoted bool `json:"is_voted"`

	IsBasedOnCustomTemplate bool   `json:"is_based_on_custom_template"`
	TemplateName            string `json:"template_name"`

	IsInstantExecution bool  `json:"is_instant_execution"`
	ExecutionTs        int64 `json:"execution_ts"`
	PublicityTs        int64 `json:"publicity_ts"`

	// Timestamps
	CreateTs int64 `json:"create_ts"`

	// Associated project ID, used for close_project proposal
	AssociatedProjectId uint `json:"associated_project_id"`

	AssociatedProjectBudgets []*project.ProjectBudgetResp `json:"associated_project_budgets"`
}

type FrontendProposalCategory struct {
	ID         uint   `json:"id"`          // Proposal Category ID
	ParentID   uint   `json:"parent_id"`   // Parent ID of this category
	Name       string `json:"name"`        // Name of the category
	MetaforoId uint   `json:"metaforo_id"` // Metaforo ID for the category
	HasPerm    bool   `json:"has_perm"`    // Whether the requester has permission to create proposal in this category
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

type associatedProposalData struct {
	Applicant       string `json:"applicant"`
	ApplicantAvatar string `json:"applicant_avatar"`
	ProjectGuild    struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"project_guild"`
	Proposal struct {
		CreateTs             int    `json:"create_ts"`
		Id                   int    `json:"id"`
		Name                 string `json:"name"`
		ProposalCategoryName string `json:"proposal_category_name"`
		State                string `json:"state,omitempty"`
	} `json:"proposal"`
	ProposalId string `json:"proposal_id"`
}

const oneYearDuration = 24 * 365 * time.Hour

var StateOrder = []model.ProposalState{
	model.ProposalStateVoting,
	model.ProposalStateDraft,
}

type ComponentResponse struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	Schema        string `json:"schema"`
	ScreenshotUri string `json:"screenshot_uri"`
	IsHidden      bool   `json:"is_hidden"`
}

const listTemplateWithPermSQL = `select
pt.id,pt.name,pt.content_schema,pt.screenshot_uri,
pc.name as category_name,
pt.display_index as display_index,
pc.display_index as category_display_index,
pt.proposal_category_id as category_id,
pt.rule_desc as rule_description,
pt.publicity_second = 0 as is_instant_vote,
pt.type = ? as is_closing_project,
pt.vote_type
from proposal_templates pt
left join proposal_categories pc on pt.proposal_category_id =pc.id
where pt.is_hidden=false
order by pc.display_index, pt.display_index`

type TemplateResponse struct {
	ID                   uint   `json:"id"`
	Name                 string `json:"name"`
	DisplayIndex         int    `json:"display_index"`
	ScreenshotUri        string `json:"screenshot_uri"`
	ContentSchema        string `json:"schema"`
	CategoryName         string `json:"-"`
	CategoryId           uint   `json:"-"`
	CategoryDisplayIndex uint   `json:"category_display_index"`
	HasPermToUse         bool   `json:"has_perm_to_use"`
	RuleDescription      string `json:"rule_description"`
	IsInstantVote        bool   `json:"is_instant_vote"`
	IsClosingProject     bool   `json:"is_closing_project"`
	VoteType             int    `json:"vote_type"`
}

type TemplateResponseWithComponents struct {
	TemplateResponse
	Components []*ComponentResponse `json:"components"`
}

type TmplWithCategoryNameRecord struct {
	CategoryId           uint                              `json:"category_id"`
	CategoryDisplayIndex uint                              `json:"category_display_index"`
	CategoryName         string                            `json:"category_name"`
	Templates            []*TemplateResponseWithComponents `json:"templates"`
}

type updateTmplRequest struct {
	Name          string   `json:"name"`
	CategoryId    uint     `json:"category_id"`
	Schema        string   `json:"schema"`
	ScreenshotUri string   `json:"screenshot_uri"`
	Components    []string `json:"components"`
}

type VoterInfo struct {
	MetaforoUserId int    `json:"metaforo_user_id"`
	Wallet         string `json:"wallet"`
	Avatar         string `json:"avatar"`
}

type userVoteDetailInfo struct {
	JointMetaforoAndOsUser
	VoteWeight int `json:"weight"`
}
