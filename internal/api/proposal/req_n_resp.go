package proposal

import (
	"encoding/json"
	"errors"

	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api/component"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/db_agent"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

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
	ProposalCategoryId      uint                          `json:"proposal_category_id"`
	ContentBlocks           []*FrontendContentBlockRecord `json:"content_blocks"`
	Components              []*ComponentRequestData       `json:"components"`
	MetaforoAccessToken     string                        `json:"metaforo_access_token"`
	SubmitToMetaforo        bool                          `json:"submit_to_metaforo"`
	EditorType              int                           `json:"editor_type"`
	VoteType                int                           `json:"vote_type"`
	VoteOptions             []string                      `json:"vote_options"`
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
	IsVoted   bool   `json:"-"`
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

	// Is current user voted for this proposal
	IsVoted bool `json:"is_voted"`

	IsBasedOnCustomTemplate bool   `json:"is_based_on_custom_template"`
	TemplateName            string `json:"template_name"`

	IsInstantExecution bool  `json:"is_instant_execution"`
	ExecutionTs        int64 `json:"execution_ts"`
	PublicityTs        int64 `json:"publicity_ts"`

	// Timestamps
	CreateTs int64 `json:"create_ts"`
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

///////////////////////
// Some converter functions
///////////////////////

func ConvertProposalToFrontendDetailRecord(db *gorm.DB, proposalId uint, startPostId int, accessToken string, metaforoGroupName string) (*FrontendProposalDetailRecord, error) {
	var proposalBlocks []*model.ProposalContentBlock
	if err := db.Where(&model.ProposalContentBlock{ProposalID: proposalId}).Order("id").Find(&proposalBlocks).Error; err != nil {
		return nil, err
	}

	var proposalComponentRecords []*model.ProposalComponentRecord
	if err := db.Where(&model.ProposalComponentRecord{ProposalID: proposalId}).
		Where("component_id != ?", 0).
		Order("id").Find(&proposalComponentRecords).Error; err != nil {
		return nil, err
	}

	proposalContentResponse := lo.Map(proposalBlocks, func(item *model.ProposalContentBlock, _ int) *FrontendContentBlockRecord {
		return &FrontendContentBlockRecord{
			ID:            item.ID,
			Title:         item.Title,
			Content:       item.Content,
			Type:          item.Type,
			ComponentList: item.ComponentList,
		}
	})

	proposalComponentResponse := lo.Map(proposalComponentRecords, func(item *model.ProposalComponentRecord, _ int) *component.ComponentInstance {
		var componentRecord model.ProposalComponent
		err := db.Find(&componentRecord, item.ComponentID).Error
		if err != nil {
			log.Error().Msgf("fetch component %d from DB error: %+v", item.ComponentID, err)
			return nil
		}

		// Special processing for `associate_proposal`
		if componentRecord.Name == internal.ComponentNameAssociateProposal {
			var parsedData associatedProposalData
			err := json.Unmarshal([]byte(item.Data), &parsedData)

			if err != nil {
				log.Error().Msgf("unmarshal associate proposal data error: %+v", err)
			} else {
				var associatedProposalRecord model.Proposal
				err = db.Find(&associatedProposalRecord, parsedData.Proposal.Id).Error
				if err != nil {
					log.Error().Msgf("query associated proposal %d from DB error: %+v", parsedData.Proposal.Id, err)
				} else {
					parsedData.Proposal.State = model.ProposalStateName[associatedProposalRecord.State]
				}

				var associatedApplicantRecord model.User
				err = db.Model(&model.User{}).Where("wallet = ?", common.FormatUserWallet(parsedData.Applicant)).First(&associatedApplicantRecord).Error
				if err != nil {
					log.Error().Msgf("query associated applicant %s from DB error: %+v", parsedData.Applicant, err)
				} else {
					parsedData.ApplicantAvatar = associatedApplicantRecord.Avatar
				}

				dataBytes, err := json.Marshal(parsedData)
				if err != nil {
					log.Error().Msgf("marshal associate proposal data error: %+v", err)
				}

				item.Data = string(dataBytes)
			}
		}

		return &component.ComponentInstance{
			ID:            item.ID,
			ComponentId:   item.ComponentID,
			ComponentName: componentRecord.Name,
			Schema:        "",
			Data:          item.Data,
			CreateTs:      item.CreateTs,
		}
	})

	var proposal model.Proposal
	if err := db.Find(&proposal, proposalId).Error; err != nil {
		log.Error().Msgf("query proposal %d from DB error: %+v", proposalId, err)
		return nil, err
	}

	var editHistoryRecords []*FrontendProposalEditHistoryRecord
	var frontendCommentsRecords []*FrontendProposalCommentRecord
	var votes []metaforo.PollRecord
	rejectedComment := model.ProposalComment{}
	commentCount := 0

	if proposal.ProposalRecordId != "" {
		metaforoProposal, err := metaforo.GetProposal(proposal.GetMetaforoThreadId(), metaforoGroupName, accessToken, startPostId)
		if err != nil {
			return nil, err
		}

		commentCount = metaforoProposal.Thread.PostsCount
		votes = metaforoProposal.Thread.Polls

		err = UpdateDbRecordsFromMetaforoProposalResponse(db, proposalId, metaforoProposal)
		if err != nil {
			log.Error().Msgf("update proposal %d from metaforo error: %+v", proposalId, err)
			return nil, err
		}

		pollStatusChanged, err := UpdateDbVoteOptionRecordsFromMetaforoProposalResponse(db, proposalId, metaforoProposal)
		if err != nil {
			log.Error().Msgf("update propsal vote option records with metaforo response error: %+v", err)
			return nil, err
		}

		if pollStatusChanged {
			if err = HandleProposalPollStatusChange(db, proposalId); err != nil {
				log.Error().Msgf("handle proposal poll status change error: %+v", err)
				return nil, err
			}
		}

		err = db.Model(model.ProposalComment{}).Where("proposal_id = ? AND is_reject_comment = ?", proposalId, true).First(&rejectedComment).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error().Msgf("fetch rejected comment error: %+v", err)
			return nil, err
		}

		editHistoryRecords, err = GetLocalEditHistoriesWithOsUserData(db, proposal.ProposalRecordId)
		if err != nil {
			log.Error().Msgf("fetch local history record error: %+v", err)
			return nil, err
		}

		// Process comments
		frontendCommentsRecords, err = GetProposalCommentsWithOsUserData(db, metaforoProposal.Thread.Posts)
		if err != nil {
			log.Error().Msgf("fetch proposal comments error: %+v", err)
			return nil, err
		}
	}

	// Fetch vote_gate info
	// TODO: duplicated code in vote check logic, use function to replace it.
	var proposalCategory *model.ProposalCategory
	err := db.Model(&model.ProposalCategory{}).
		Joins("ProposalVoteGate").
		Where(model.ProposalCategory{ID: proposal.ProposalCategoryID}).First(&proposalCategory).Error
	if err != nil {
		log.Error().Msgf("fetch proposal category error: %+v", err)
		return nil, err
	}

	var voteGate *FrontendVoteGateResponse
	if proposalCategory.ProposalVoteGate != nil {
		voteGate = &FrontendVoteGateResponse{
			ID:        proposalCategory.ProposalVoteGate.ID,
			Name:      proposalCategory.ProposalVoteGate.Name,
			TokenAddr: proposalCategory.ProposalVoteGate.TokenAddress,
			TokenId:   proposalCategory.ProposalVoteGate.TokenId,
			TokenType: proposalCategory.ProposalVoteGate.TokenTypeName(),
			ChainType: proposalCategory.ProposalVoteGate.ChainName(),
		}
	}

	templateName := ""
	if proposal.ProposalTemplateID != nil {
		var template model.ProposalTemplate
		err := db.Find(&template, *proposal.ProposalTemplateID).Error
		if err != nil {
			log.Error().Msgf("fetch proposal template error: %+v", err)
			return nil, err
		}
		templateName = template.Name
	}

	proposalExecTs := int64(0)
	var proposalCronJobs []*model.CronJob
	if err = db.Where(&model.CronJob{ProposalId: proposalId}).Find(&proposalCronJobs).Error; err != nil {
		log.Error().Msgf("get proposal cronjob error: %+v", err)
	}

	var voteRecords []*model.ProposalVoteRecord
	err = db.Model(proposal).Association("VoteRecords").Find(&voteRecords)
	if err != nil {
		log.Error().Msgf("get vote records error: %+v", err)
		return nil, err
	}

	proposalPublicityTs := proposal.CreateTs + proposal.PublicitySecond
	for _, r := range voteRecords {
		proposalPublicityTs = r.StartTs
	}

	for _, job := range proposalCronJobs {
		if job.NextExecTs > proposalExecTs {
			proposalExecTs = job.NextExecTs
		}
	}

	// Refresh proposal record
	if err = db.Find(&proposal, proposalId).Error; err != nil {
		log.Error().Msgf("fetch proposal error: %+v", err)
		return nil, err
	}
	return &FrontendProposalDetailRecord{
		ID:                      proposalId,
		Title:                   proposal.Title,
		ContentBlocks:           proposalContentResponse,
		ProposalCategoryId:      proposal.ProposalCategoryID,
		State:                   model.ProposalStateName[proposal.State],
		Components:              proposalComponentResponse,
		Applicant:               proposal.Applicant,
		ApplicantAvatar:         db_agent.GetUserAvatar(proposal.Applicant),
		IsRejected:              proposal.State == int(model.ProposalStateRejected),
		RejectReason:            rejectedComment.Content,
		RejectTs:                rejectedComment.CreateTs,
		RejectMetaforoCommentId: rejectedComment.MetaforoCommentId,
		Histories: &FrontendProposalEditHistories{
			TotalCount: len(editHistoryRecords),
			Lists:      editHistoryRecords,
		},
		Sip:                     proposal.Sip,
		Arweave:                 proposal.ArweaveHash,
		CommentCount:            commentCount,
		Comments:                frontendCommentsRecords,
		VoteGate:                voteGate,
		Votes:                   votes,
		VoteType:                proposal.VoteType,
		CreateTs:                proposal.CreateTs,
		IsBasedOnCustomTemplate: proposal.IsBasedOnCustomTemplate,
		TemplateName:            templateName,
		IsInstantExecution:      proposal.PendingExecutionSecond == 0,
		ExecutionTs:             proposalExecTs,
		PublicityTs:             proposalPublicityTs,
	}, nil
}
