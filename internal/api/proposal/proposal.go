package proposal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var err error

var StateOrder = []model.ProposalState{
	model.ProposalStateVoting,
	model.ProposalStateDraft,
}

// List handles the HTTP request to list proposals.
//
//	@summary	lists all proposals based on query params and return in JSON format
//	@router		/proposals/list [get]
//	@tags		Proposal
//	@Param		page		query		int		false	"which page"
//	@Param		size		query		int		false	"size of each page"
//	@Param		sort_field	query		string	false	"sort by which field"
//	@Param		sort_order	query		string	false	"order of sort"	Enum(asc desc)
//	@Param		state		query		string	false	"state of proposal, for multiple states, use comma as separator"
//	@Param		sip			query		string	false	"whether query only proposals with SIP number"
//	@Param		category_id	query		int		false	"filter proposal records with specified category"
//	@success	200			{object}	api.Reply{data=api.ListReplyData{rows=FrontendProposalListRecord}}
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	queryParams := ListProposalQueryParams{}
	if err = ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}
	// Parse pagination
	page := api.ParseAndConvertPageParam(ctx)

	querySql := ListProposalsSQL

	// Build query
	if queryParams.State != "" {
		stateList := strings.Split(queryParams.State, ",")
		var stateVals []string
		for _, stateName := range stateList {
			if stateVal, found := model.ProposalStateIdNameMapping[stateName]; found {
				stateVals = append(stateVals, fmt.Sprintf("%d", stateVal))
			} else {
				sdk.LogUserSideError(ctx, fmt.Errorf("query proposal state %s error", queryParams.State))
				log.Warn().Msgf("query proposal state %s error", queryParams.State)
			}
		}

		querySql += fmt.Sprintf(" AND state IN (%s)", strings.Join(stateVals, ","))
	} else {
		querySql += fmt.Sprintf(" AND state != %d", model.ProposalStatePendingSubmit)
	}

	if queryParams.CategoryId != 0 {
		querySql += fmt.Sprintf(" AND proposal_category_id = %d", queryParams.CategoryId)
	}

	if queryParams.CategoryId != 0 {
		querySql += fmt.Sprintf(" AND proposal_category_id = %d", queryParams.CategoryId)
	}

	if queryParams.Q != "" {
		querySql += fmt.Sprintf(" AND title ilike '%%%s%%'", queryParams.Q)
	}

	listBySip := false
	if queryParams.Sip != "" {
		querySql += fmt.Sprintf(" AND sip != 0")
		listBySip = true
	}

	total, resultRows, err := generateFrontendProposalRecords(db, querySql, page, listBySip)
	if err != nil {
		log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", querySql, err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  resultRows,
	}))
}

// Detail function returns proposal detail data
//
//	@summary	Show proposals with given ID
//	@router		/proposals/show/:id [get]
//	@tags		Proposal
//	@param		start_post_id	query		int		false	"start post ID"
//	@param		access_token	query		string	true	"metaforo access token"
//	@success	200				{object}	api.Reply{data=FrontendProposalDetailRecord}
func Detail(ctx *gin.Context) {
	startPostIdStr := ctx.Query("start_post_id")
	metaforoAccessToken := ctx.Query("access_token")
	startPostId := 0
	if startPostIdStr != "" {
		startPostId, err = strconv.Atoi(startPostIdStr)
		if err != nil {
			log.Warn().Msgf("parse ")
		}
	}

	proposalIdStr := ctx.Param("id")
	db, cfg := api.ForContextDBAndConfig(ctx)
	proposalRecord, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, api.BadRequest(err))
			return
		} else {
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf("parse proposal id %s error: %+v", ctx.Param("id"), err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
	}

	responseData, err := ConvertProposalToFrontendDetailRecord(db, proposalRecord.ID, startPostId, metaforoAccessToken, cfg.MetaforoData.GroupName)
	if err != nil {
		log.Error().Msgf("convert proposal to frontend format error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("load proposal data error")))
		return
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}

// Update existing proposal
// 1. Only proposal in PendingSubmit, Withdrawn, Rejected state can be updated
// 2. Updating PendingSubmit proposal does not change the state and version number, and is update in place directly
// 3. Updating proposal in Withdrawn, Rejected state will create a new record and update the version number
//
//	@router		/proposals/update/:id [post]
//	@summary	Update proposals with passed in data
//	@tags		Proposal
//	@Param		JsonBody	body		CreateOrUpdateProposalData	true	"request json body"
//	@success	200			{object}	api.Reply{}
func Update(ctx *gin.Context) {
	user, _, db, cfg := api.ForContext(ctx)
	proposalIdStr := ctx.Param("id")
	proposalRcd, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
			return
		} else {
			log.Error().Msgf("get proposal id %s error: %+v", proposalIdStr, err)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("get proposal error")))
			return
		}
	}

	if !proposalRcd.CanBeUpdatedBy(user.Wallet) {
		log.Error().Msgf("proposal id %s can't be updated by user %s", proposalIdStr, user.Wallet)
		sdk.LogUserSideError(ctx, fmt.Errorf("proposal id %s can't be updated by user %s", proposalIdStr, user.Wallet))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("proposal can't be updated by current user")))
		return
	}

	if proposalRcd.ProposalRecordId != "" {
		if proposalRcd.VoteStartTs < time.Now().Unix() {
			err = fmt.Errorf("proposal id %s has expired the publicity time, can't be updated by user %+v", proposalIdStr, user.Wallet)
			log.Error().Msg(err.Error())
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("proposal has expired and can't be updated")))
			return
		}
	} else {
		// No actions should be done for draft proposals
	}

	// Proposal is in updatable state

	// Parsing request to create proposal object
	var reqData CreateOrUpdateProposalData
	if err = ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	// In close project proposal, verify whether the project to be closed is in open or closed_failed state,
	// and only set project to closing in those status. For other cases, return error
	// In update cases, the proposal should already have a template ID, which is not changeable in update action,
	// and frontend request doesn't send template ID in request, so use the proposal data
	pTmplType, err := getProposalTemplateType(db, *proposalRcd.ProposalTemplateID)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal template error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}
	reqData.TemplateId = *proposalRcd.ProposalTemplateID

	// In close project proposal, verify whether the project to be closed is in open or closed_failed state,
	// and only set project to closing in those status. For other cases, return error
	if pTmplType == model.ProposalTemplateTypeCloseProject {
		projectCanBeClosed, err := verifyProjectCanBeClosed(db, reqData.CreateProjectProposalId)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("get proposal created project error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
			return
		}

		if !projectCanBeClosed {
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf("project in status can't be closed")
			ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("related project can't be closed")))
			return
		}
	}

	proposalRecord, err := SaveProposalRecordToDB(db, &reqData, user.Wallet, proposalRcd.ID, cfg)
	if err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	if reqData.SubmitToMetaforo {
		if reqData.MetaforoAccessToken == "" {
			log.Error().Msgf("metaforo access token is empty")
			sdk.LogUserSideError(ctx, errors.New("metaforo access token is empty"))
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("metaforo access token is empty")))
			return
		}

		if err = updateProposalAssociatedProjectStatusInCloseProjectToClosing(db, reqData); err != nil {
			log.Error().Msgf("associate mushrooms: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
			return
		}

		if err := SaveProposalToMetaforo(db, proposalRecord.ID, proposalRecord.VoteType, reqData.MetaforoAccessToken, reqData.EditorType, reqData.IsMultipleVote, cfg.MetaforoData.GroupName); err != nil {
			log.Error().Msgf("create metaforo proposal error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
			return
		}

		// If the publicity second is 0, update the db proposal to voting state
		if proposalRecord.PublicitySecond == 0 {
			log.Debug().Msgf("proposal has no publicity time, change to approved status directly")
			_, err = UpdateProposalStateAndLaunchStateChangeActions(db, user, proposalIdStr, model.ProposalStateApproved, cfg)
		} else if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			if err = createJobToUpdateNoVoteProposalToNextState(db, proposalRecord.ID, proposalRecord.VoteType, proposalRecord.CreateTs+proposalRecord.PublicitySecond, model.ProposalStateApproved); err != nil {
				log.Error().Msgf("create proposal state change error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
				return
			}
		}
	}

	responseData, err := ConvertProposalToFrontendDetailRecord(db, proposalRecord.ID, 0, reqData.MetaforoAccessToken, cfg.MetaforoData.GroupName)
	if err != nil {
		log.Error().Msgf("convert proposal to frontend format error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("load proposal data error")))
		return
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}

// Create function saves proposal to DB and create Metaforo thread as well, the proposal will be in Draft state
//
//	@router		/proposals/create [post]
//	@tags		Proposal
//	@summary	Create metaforo proposal and public to others
//	@Param		JsonBody	body		CreateOrUpdateProposalData	true	"request json body"
//	@success	200			{object}	api.Reply{}
func Create(ctx *gin.Context) {
	// Parsing request to create proposal object
	var reqData CreateOrUpdateProposalData
	if err = ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	user, _, db, cfg := api.ForContext(ctx)

	// Get proposal template data, if it is a close project proposal, try to get project info and check whether it is in open or closed_failed state
	pTmplType, err := getProposalTemplateType(db, reqData.TemplateId)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal template error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	// In close project proposal, verify whether the project to be closed is in open or closed_failed state,
	// and only set project to closing in those status. For other cases, return error
	if pTmplType == model.ProposalTemplateTypeCloseProject {
		projectCanBeClosed, err := verifyProjectCanBeClosed(db, reqData.CreateProjectProposalId)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("get proposal created project error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
			return
		}

		if !projectCanBeClosed {
			err := fmt.Errorf("project %d can't be closed", reqData.CreateProjectProposalId)
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf(err.Error())
			ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("related project can't be closed")))
			return
		}
	}

	proposalRecord, err := SaveProposalRecordToDB(db, &reqData, user.Wallet, 0, cfg)
	if err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	if reqData.SubmitToMetaforo {
		if reqData.MetaforoAccessToken == "" {
			log.Error().Msgf("metaforo access token is empty")
			sdk.LogUserSideError(ctx, errors.New("metaforo access token is empty"))
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("metaforo access token is empty")))
			return
		}

		if err = updateProposalAssociatedProjectStatusInCloseProjectToClosing(db, reqData); err != nil {
			log.Error().Msgf("associate proposal with project error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
			return
		}

		if err := SaveProposalToMetaforo(db, proposalRecord.ID, proposalRecord.VoteType, reqData.MetaforoAccessToken, reqData.EditorType, reqData.IsMultipleVote, cfg.MetaforoData.GroupName); err != nil {
			log.Error().Msgf("create metaforo proposal error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
			return
		}

		// If the publicity second is 0, update the db proposal to voting state or pending execution state, and handle SIP data
		if proposalRecord.PublicitySecond == 0 {
			log.Debug().Msgf("proposal has no publicity time, change to approved status directly")
			_, err = UpdateProposalStateAndLaunchStateChangeActions(db, user, fmt.Sprintf("%d", proposalRecord.ID), model.ProposalStateApproved, cfg)
		} else if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			if err = createJobToUpdateNoVoteProposalToNextState(db, proposalRecord.ID, proposalRecord.VoteType, proposalRecord.CreateTs+proposalRecord.PublicitySecond, model.ProposalStateApproved); err != nil {
				log.Error().Msgf("create proposal state change error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
				return
			}
		}

		db.First(&proposalRecord, proposalRecord.ID)
	}

	responseData, err := ConvertProposalToFrontendDetailRecord(db, proposalRecord.ID, 0, reqData.MetaforoAccessToken, cfg.MetaforoData.GroupName)
	if err != nil {
		log.Error().Msgf("convert proposal to frontend format error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("load proposal data error")))
		return
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}

// Withdraw, Approve and Reject change proposal to named state and update the Metaforo label

// Withdraw changes proposal state to withdrawn
//
//	@router		/proposals/withdraw/:id [post]
//	@tags		Proposal
//	@summary	withdraw proposal in Draft state, the proposal will be changed to withdrawn state after success. Only proposal applicant can withdraw the proposal
//	@param		id	query		int	true	"proposal id"
//	@success	200	{object}	api.Reply{data=nil}
func Withdraw(ctx *gin.Context) {
	user, _, db, cfg := api.ForContext(ctx)
	proposalIdStr := ctx.Param("id")
	_, err := UpdateProposalStateAndLaunchStateChangeActions(db, user, proposalIdStr, model.ProposalStateWithdrawn, cfg)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
			return
		} else {
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf("parse proposal id %s error: %+v", ctx.Param("id"), err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
	}
	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Approve changes proposal state to approved
//
//	@router		/proposals/approve/:id [post]
//	@tags		Proposal
//	@summary	approve proposal in Draft state, the proposal will be changed to approved state after success. Only user has cityhall permission can do this
//	@param		id	query		int	true	"proposal id"
//	@success	200	{object}	api.Reply{data=nil}
func Approve(ctx *gin.Context) {
	user, enforcer, db, cfg := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proposalIdStr := ctx.Param("id")
	_, err = UpdateProposalStateAndLaunchStateChangeActions(db, user, proposalIdStr, model.ProposalStateApproved, cfg)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
		} else {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("update proposal %s state to approved error: %+v", proposalIdStr, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("approve proposal error")))
			return
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// Reject changes proposal state to approved
//
//	@router		/proposals/reject/:id [post]
//	@tags		Proposal
//	@summary	reject proposal in Draft state, the proposal will be changed to rejected state after success. Only user has cityhall permission can do this
//	@param		id	query		int	true	"proposal id"
//	@success	200	{object}	api.Reply{data=nil}
func Reject(ctx *gin.Context) {
	user, enforcer, db, cfg := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var rejectRequestData RejectProposalData
	if err = ctx.BindJSON(&rejectRequestData); err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("parse request data error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("parse request data error")))
		return
	}

	if rejectRequestData.MetaforoAccessToken == "" {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("missing metaforo_access_token value")
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo_access_token value")))
		return
	}

	proposalIdStr := ctx.Param("id")
	proposalRecordId, err := UpdateProposalStateAndLaunchStateChangeActions(db, user, proposalIdStr, model.ProposalStateRejected, cfg)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
		} else {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("update proposal %s state to rejected error: %+v", proposalIdStr, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("reject proposal error")))
			return
		}
	}

	rejectComment := model.ProposalComment{
		CreateTs:        time.Now().Unix(),
		UpdateTs:        time.Now().Unix(),
		ProposalID:      proposalRecordId,
		Content:         rejectRequestData.Reason,
		IsHidden:        false,
		IsRejectComment: true,
	}
	db.Save(&rejectComment)

	proposalRecord, err := GetProposalFromStringId(db, proposalIdStr)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal %s error: %+v", proposalIdStr, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
		return
	}

	// Add reject comment
	commentData, err := metaforo.AddComment(
		rejectRequestData.MetaforoAccessToken,
		cfg.MetaforoData.GroupName,
		proposalRecord.GetMetaforoThreadId(),
		rejectComment.Content,
		"",
		1)
	if err != nil {
		log.Error().Msgf("add comment to proposal %s error: %+v", proposalIdStr, err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("add comment error")))
		return
	}

	// Comment currently is fetched from Metaforo directly, so only RejectReason is saved in local DB
	db.Model(&rejectComment).Where("id = ?", rejectComment.ID).Update("metaforo_comment_id", fmt.Sprintf("%d", commentData.Id))

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// MyList returns proposals created by login user
// TODO: Almost same with List, find way to merge them
//
//	@summary	Returns proposals created by login user
//	@tags		Proposal
//	@router		/proposals/my [get]
//	@param		page			query		int		false	"which page"
//	@param		size			query		int		false	"size of each page"
//	@param		sort_field		query		string	false	"sort by which field"
//	@param		sort_order		query		string	false	"order of sort"	Enum(asc desc)
//	@param		pending_submit	query		int		false	"Return pending submit proposals or other state, only query PendingSubmit records when value is `1`"
//	@success	200				{object}	api.Reply{data=FrontendProposalDetailRecord}
func MyList(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)
	queryParams := ListProposalQueryParams{}
	if err = ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}

	// Parse pagination
	page := api.ParseAndConvertPageParam(ctx)

	querySql := fmt.Sprintf("%s WHERE applicant = '%s'", ListProposalsSQL, common.FormatUserWallet(user.Wallet))

	queryPendingSubmitFlag := ctx.Query("pending_submit")
	if queryPendingSubmitFlag == "1" {
		querySql += fmt.Sprintf(" AND state = %d", model.ProposalStatePendingSubmit)
	} else {
		querySql += fmt.Sprintf(" AND state != %d", model.ProposalStatePendingSubmit)
	}

	total, resultRows, err := generateFrontendProposalRecords(db, querySql, page, false)
	if err != nil {
		log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", querySql, err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  resultRows,
	}))
}

// GetProposalsUsedForCreatingProjects returns proposals that is created by request user and from creating proposal template.
// The projects returned will be used for close project component
//
//	@summary	Returns creating project and executed proposals created by login user, if category_id is not specified, all proposals for opening project will be returned
//	@tags		Proposal
//	@router		/proposals/creating_project_proposals [get]
//	@param		category_id	query		int	false	"Limit the category of creating project proposal"
//	@success	200			{object}	api.Reply{data=FrontendProposalDetailRecord}
func GetProposalsUsedForCreatingProjects(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)
	categoryIdStr := ctx.Query("category_id")

	var newProjectTemplateIds []uint
	tmplQueryParams := model.ProposalTemplate{
		Type: model.ProposalTemplateTypeNewProject,
	}

	queryingCommonProjectsByOwner := false

	// Some proposal templates uses another category ID for closing project proposal.
	if categoryIdStr != "" {
		categoryId, err := strconv.Atoi(categoryIdStr)
		if err != nil {
			log.Error().Msgf("parse category ID %s to int error: %+v", categoryIdStr, err)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.ServerError(fmt.Errorf("parse category ID %s to int error: %+v", categoryIdStr, err)))
			return
		}

		// Try to get category data and check whether it is needed to remap the category ID
		var pCategory model.ProposalCategory
		db.Model(&pCategory).Where("id = ?", categoryId).First(&pCategory)
		if pCategory.CategoryIdForCloseProject != 0 {
			tmplQueryParams.ProposalCategoryID = pCategory.CategoryIdForCloseProject
			if pCategory.Name == internal.AutomationCreatedCommonProjectCategory {
				// This is a closing project proposal for common project, change to query by project owner
				queryingCommonProjectsByOwner = true
			}
		} else {
			tmplQueryParams.ProposalCategoryID = uint(categoryId)
		}
	}

	querySql := ""

	if queryingCommonProjectsByOwner {
		closableCommonProjects, err := model.ProjectModel.GetClosableProject(db, user.Wallet, []string{
			internal.ManuallyCreatedCommonProjectCategory,
			internal.AutomationCreatedCommonProjectCategory,
		})

		if err != nil {
			log.Error().Msgf("get closable common project error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
			return
		}

		proposalIds := []string{}
		for _, project := range closableCommonProjects {
			proposalIds = append(proposalIds, project.Proposals...)
		}

		err = db.Model(&model.Proposal{}).Where("id IN (?)", proposalIds).Error
		if err != nil {
			log.Error().Msgf("get create project template id error: err: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
			return
		}

		querySql = fmt.Sprintf("%s WHERE p.id IN (%s) order by sip desc, create_ts desc",
			ListProposalsSQLForGettingCreatingProjectProposal,
			lo.Map(proposalIds, func(proposalId string, _ int) string { return fmt.Sprintf("'%s'", proposalId) }),
		)

	} else {
		err := db.Model(&model.ProposalTemplate{}).Where(tmplQueryParams).Pluck("id", &newProjectTemplateIds).Error
		if err != nil {
			log.Error().Msgf("get create project template id error: err: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
			return
		}

		if len(newProjectTemplateIds) == 0 {
			log.Warn().Msgf("no opening project template found for category: %s", categoryIdStr)
			ctx.JSON(http.StatusOK, api.Success([]*FrontendProposalListRecord{}))
			return
		}

		querySql = fmt.Sprintf("%s WHERE applicant = '%s' AND proposal_template_id IN (%s) AND state = %d AND p.sip != 0 AND projects.status IN (%s) order by sip desc, create_ts desc",
			ListProposalsSQLForGettingCreatingProjectProposal,
			common.FormatUserWallet(user.Wallet),
			strings.Join(lo.Map(newProjectTemplateIds, func(tmpId uint, _ int) string {
				return fmt.Sprintf("%d", tmpId)
			}), ","),
			model.ProposalStateExecuted,
			strings.Join(lo.Map([]model.ProjectStatus{model.ProjectStatusOpen, model.ProjectStatusCloseFailed}, func(projectStatus model.ProjectStatus, _ int) string {
				return fmt.Sprintf("'%s'", projectStatus)
			}), ","),
		)
	}

	// The passed in query seg has already sorted by sip desc, no need to specify it in listBySip param
	_, resultRows, err := generateFrontendProposalRecords(db, querySql, nil, false)
	if err != nil {
		log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", querySql, err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
		return
	}

	resultRowsWithSipRecords := lo.Map(resultRows, func(r *FrontendProposalListRecord, _ int) *FrontendProposalListRecord {
		if r.Sip != 0 {
			r.Title = fmt.Sprintf("SIP-%d: %s", r.Sip, r.Title)
		}
		return r
	})

	ctx.JSON(http.StatusOK, api.Success(resultRowsWithSipRecords))
}

// UpdateProposalStateAndLaunchStateChangeActions changes proposal state and launch specified actions associated with state change
// This function is invoked in directly API calls, like approve, withdrawn, etc. and proposal state change automation tasks
// The state change actions contains:
// - Withdrawn: Set vote start time to 1 yr later after withdrawn
// - Approved: Set proposal sip, create project for p1 create project proposal (no vote and 0 pending execution time), or start vote
// - Rejected: Set vote start time to 1 yr later after rejected
// - PendingExecution: None
// - Executed: Create project if it is new project proposal
func UpdateProposalStateAndLaunchStateChangeActions(db *gorm.DB, user *middleware.CurUser, proposalStrId string, newState model.ProposalState, cfg *config.Config) (uint, error) {
	proposalId, err := strconv.Atoi(proposalStrId)
	if err != nil {
		log.Error().Msgf("parse proposal ID %s to int error: %+v", proposalStrId, err)
		return 0, err
	}

	if TryAcquireUpdateProposalDbLockOrReturn(uint(proposalId)) == false {
		err := fmt.Errorf("proposal %s is updating", proposalStrId)
		log.Error().Msg(err.Error())
		return 0, err
	}
	defer ReleaseUpdateProposalDbLock(uint(proposalId))

	log.Debug().Msgf("enter UpdateProposalStateAndLaunchStateChangeActions, proposalStrId: %s, newState: %s", proposalStrId, model.ProposalStateName[newState])
	proposalRecord, err := GetProposalFromStringId(db, proposalStrId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalStrId)
		}
		return 0, err
	}

	var pTemplate *model.ProposalTemplate
	if err = db.Model(&proposalRecord).Association("ProposalTemplate").Find(&pTemplate); err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return 0, err
	}

	// Login to admin account to avoid token expiration
	//metaforo.Login(cfg.Metaforo.GroupName, cfg.Metaforo.AdminWallet, cfg.Metaforo.Sign, cfg.Metaforo.SignMsg, cfg.Metaforo.WalletType)
	RefreshMetaforoAdminToken()

	// Check whether user has permission to the change the proposal state
	switch newState {
	case model.ProposalStateWithdrawn:
		if user == nil || !strings.EqualFold(user.Wallet, proposalRecord.Applicant) {
			return 0, errors.New("proposal can only be withdrawn by applicant")
		}

		var voteRecords []*model.ProposalVoteRecord
		err = db.Model(proposalRecord).Association("VoteRecords").Find(&voteRecords)
		if err != nil {
			log.Error().Msgf("get vote records error: %+v", err)
			return 0, err
		}

		if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			log.Debug().Msgf("proposal %d has no vote records, clear cronjob created for updating state", proposalId)
			if err = db.Model(&model.CronJob{}).Where(&model.CronJob{ProposalId: proposalRecord.ID}).Delete(&model.CronJob{}).Error; err != nil {
				log.Error().Msgf("delete cronjob error while withdrawing proposal: %+v", err)
				return 0, err
			}
		}

		for _, record := range voteRecords {
			oneYearDuration := 24 * 365 * time.Hour
			err := metaforo.UpdateVoteTime(cfg.MetaforoData.AccessToken,
				cfg.MetaforoData.GroupName,
				record.MetaforoID,
				time.Now().UTC().Add(oneYearDuration).Unix(), // Set the start time 1 minute in advanced
				time.Now().UTC().Add(oneYearDuration+proposalRecord.VoteDuration()).Unix(),
			)
			if err != nil {
				log.Error().Msgf("update vote information error: %+v", err)
				return 0, err
			}
		}

		err = db.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", model.ProposalStateWithdrawn).Error
		if err != nil {
			log.Error().Msgf("change proposal to withdrawn error")
			return 0, err
		}
	case model.ProposalStateApproved:
		return 0, db.Transaction(func(tx *gorm.DB) error {
			if err = setProposalSip(db, pTemplate, proposalRecord); err != nil {
				log.Error().Msgf("set proposal sip error: %+v", err)
				return err
			}

			var voteRecords []*model.ProposalVoteRecord
			err = tx.Model(&proposalRecord).Association("VoteRecords").Find(&voteRecords)
			if err != nil {
				log.Error().Msgf("get vote records error: %+v", err)
				return err
			}

			if proposalRecord.VoteType == model.ProposalVoteTypeNone {
				if proposalRecord.PendingExecutionSecond == 0 {
					proposalRecord.State = int(model.ProposalStateExecuted)

					// Additional tasks for executed proposal
					if pTemplate.Type == model.ProposalTemplateTypeNewProject {
						// Get create project params from proposal components
						prjRecord, err := CreateProjectFromAutoTasks(tx, proposalRecord.ID)
						if err != nil {
							log.Error().Msgf("create project error: %+v", err)
							return err
						}
						api.PrintStructAsJson(prjRecord, "TTT: Create Project After approving no vote and no pending execution proposal")
					}
				} else {
					// TODO: this code branch is missing create job to execute task if required, but for now, only one type of no vote proposal, so this code branch should be a dead code.
					// Create a cronjob mark the proposal to executed directly after pending execution time
					log.Error().Msgf("NOTICE: this code branch shouldn't be hit, since there is only one type of novote type proposal, if you see this log, check db data or requirements")
					proposalRecord.State = int(model.ProposalStatePendingExecution)
					// Delete cronjob created while creating for updating proposal state to approved
					if err = tx.Model(&model.CronJob{}).Delete(&model.CronJob{}, &model.CronJob{
						ProposalId:  proposalRecord.ID,
						HandlerName: internal.TaskUpdateProposalState,
					}).Error; err != nil {
						log.Error().Msgf("create proposal state change error: %+v", err)
						return err
					}

					if err = tx.Model(&model.ProposalComponentRecord{}).
						Delete(&model.ProposalComponentRecord{}, map[string]any{"proposal_id": proposalRecord.ID, "component_id": 0}).Error; err != nil {
						log.Error().Msgf("create proposal state change error: %+v", err)
						return err
					}

					if err = createJobToUpdateNoVoteProposalToNextState(tx, proposalRecord.ID, proposalRecord.VoteType, model.GetCurrentUtcEpochSecond()+proposalRecord.PendingExecutionSecond, model.ProposalStateExecuted); err != nil {
						log.Error().Msgf("create proposal state change error: %+v", err)
						return err
					}
				}
			} else {
				for _, record := range voteRecords {
					err := metaforo.UpdateVoteTime(cfg.MetaforoData.AccessToken,
						cfg.MetaforoData.GroupName,
						record.MetaforoID,
						time.Now().UTC().Add(-1*time.Minute).Unix(), // Set the start time 1 minute in advanced
						time.Now().UTC().Add(proposalRecord.VoteDuration()).Unix(),
					)
					if err != nil {
						log.Error().Msgf("update vote information error: %+v", err)
						break
					}
				}
				// Only update proposal to voting state when no error returns
				if err == nil {
					log.Debug().Msgf("update proposal state to voting")
					proposalRecord.State = int(model.ProposalStateVoting)
				}
			}
			err = tx.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", proposalRecord.State).Error
			if err != nil {
				log.Error().Msgf("change proposal to approved error")
				return err
			}
			return nil
		})
	case model.ProposalStateRejected:
		// Update vote to 1 yr later
		var voteRecords []*model.ProposalVoteRecord
		err = db.Model(proposalRecord).Association("VoteRecords").Find(&voteRecords)
		if err != nil {
			log.Error().Msgf("get vote records error: %+v", err)
			return 0, err
		}

		if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			log.Debug().Msgf("proposal %d has no vote records, clear cronjob created for updating state", proposalId)
			if err = db.Model(&model.CronJob{}).Where(&model.CronJob{ProposalId: proposalRecord.ID}).Delete(&model.CronJob{}).Error; err != nil {
				log.Error().Msgf("delete cronjob error while rejecting proposal: %+v", err)
				return 0, err
			}
		}

		for _, record := range voteRecords {
			oneYearDuration := 24 * 365 * time.Hour
			err := metaforo.UpdateVoteTime(cfg.MetaforoData.AccessToken,
				cfg.MetaforoData.GroupName,
				record.MetaforoID,
				time.Now().UTC().Add(oneYearDuration).Unix(), // Set the start time 1 minute in advanced
				time.Now().UTC().Add(oneYearDuration+proposalRecord.VoteDuration()).Unix(),
			)
			if err != nil {
				log.Error().Msgf("update vote information error: %+v", err)
				return 0, err
			}
		}

		err = db.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", model.ProposalStateRejected).Error
		if err != nil {
			log.Error().Msgf("change proposal to rejected error")
			return 0, err
		}
	case model.ProposalStatePendingExecution:
		if err = db.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", model.ProposalStatePendingExecution).Error; err != nil {
			log.Error().Msgf("update proposal %d state to pending execution error", proposalRecord.ID)
			return 0, err
		}
	case model.ProposalStateExecuted:
		// Additional tasks for executed proposal
		if pTemplate.Type == model.ProposalTemplateTypeNewProject {
			// Get create project params from proposal components
			prjRecord, err := CreateProjectFromAutoTasks(db, proposalRecord.ID)
			if err != nil {
				log.Error().Msgf("create project error: %+v", err)
				return 0, err
			}
			api.PrintStructAsJson(prjRecord, "TTT: Create Project After approving no vote and no pending execution proposal")
		}

		err = db.Model(&proposalRecord).Where("id = ?", proposalRecord.ID).Update("state", model.ProposalStateExecuted).Error
		if err != nil {
			log.Error().Msgf("change proposal to executed error")
			return 0, err
		}
	default:
		return 0, fmt.Errorf("changing proposal from state %s to %s is not approved", proposalRecord.StateName(), model.ProposalStateName[newState])
	}
	return proposalRecord.ID, nil
}

func generateFrontendProposalRecords(db *gorm.DB, querySql string, page *gormfind.Page, listBySip bool) (int64, []*FrontendProposalListRecord, error) {
	var tmpRcd []*FrontendProposalListRecord
	var countTx *gorm.DB
	countTx = db.Raw(querySql).Scan(&tmpRcd)
	if err = countTx.Error; err != nil {
		log.Error().Msgf("get proposal count error: %+v", err)
		return 0, nil, err
	}
	total := countTx.RowsAffected

	if page != nil {
		// Specify custom order by state
		// Note: this is PG specified function
		if listBySip {
			querySql += fmt.Sprintf("\nORDER BY sip desc, create_ts desc")
		} else {
			querySql += fmt.Sprintf("\nORDER BY array_position(array[%s], p.state), create_ts desc",
				strings.Join(lo.Map(StateOrder, func(state model.ProposalState, _ int) string { return fmt.Sprintf("%d", state) }), ", "))
		}
		querySql += fmt.Sprintf("\nLIMIT %d OFFSET %d", page.Size, (page.Page-1)*page.Size)
	}

	var resultRows []*FrontendProposalListRecord
	err := db.Raw(querySql).Find(&resultRows).Error
	if err != nil {
		log.Error().Msgf("get proposal list error: query sql: %s, err: %+v", querySql, err)
		return 0, nil, err
	}

	resultRows = lo.Map(resultRows, func(r *FrontendProposalListRecord, _ int) *FrontendProposalListRecord {
		dup_r := r
		dup_r.State = model.ProposalStateName[r.StateId]
		return dup_r
	})

	return total, resultRows, nil
}

// TODO: Temporary solution to get open project proposal info while creating close project proposal

func createJobToUpdateNoVoteProposalToNextState(db *gorm.DB, proposalId uint, proposalVoteType int, jobExecTs int64, nextState model.ProposalState) error {
	proposalComponentRecord := model.ProposalComponentRecord{
		ProposalID:  proposalId,
		ComponentID: 0,
	}

	if err = db.Model(&proposalComponentRecord).
		Where(map[string]any{"proposal_id": proposalId, "component_id": 0}). // Note: 0 won't be passed to query if using struct data
		First(&proposalComponentRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			db.Create(&proposalComponentRecord)
		} else {
			log.Error().Msgf("create proposal component record error: %+v", err)
			return err
		}
	} else {
		if proposalVoteType != model.ProposalVoteTypeNone {
			log.Debug().Msgf("automation for updating state has already created, return")
			return nil
		}
	}

	updateProposalStateTaskParams := map[string]any{
		"proposal_id": proposalId,
		"state":       int(nextState),
	}

	jobParamsStr, err := json.Marshal(updateProposalStateTaskParams)
	if err != nil {
		log.Error().Msgf("marshal update proposal state params error: %+v", err)
		return err
	}

	currentTs := model.GetCurrentUtcEpochSecond()

	finTask := &model.CronJob{
		CreateTs:                  currentTs,
		UpdateTs:                  currentTs,
		HandlerName:               internal.TaskUpdateProposalState,
		ProposalComponentRecordId: int(proposalComponentRecord.ID),
		ProposalId:                proposalId,
		LastExecTs:                0,
		NextExecTs:                jobExecTs,
		JobParams:                 string(jobParamsStr),
		VoteResult:                "",
		VoteType:                  proposalVoteType,
		State:                     model.CronJobStateActive,
		LastExecResult:            "",
	}
	createTaskTx := db.Where(model.CronJob{
		HandlerName:               internal.TaskUpdateProposalState,
		ProposalComponentRecordId: int(proposalComponentRecord.ID),
		ProposalId:                proposalId,
		NextExecTs:                jobExecTs,
	}).
		Assign(&finTask).FirstOrCreate(&finTask)

	if createTaskTx.Error != nil {
		log.Error().Msgf("create proposal fin task error: %+v", createTaskTx.Error)
		return createTaskTx.Error
	} else if createTaskTx.RowsAffected == 0 {
		log.Warn().Msgf("proposal fin task already exists: %+v", finTask)
	}

	return nil
}

func getProposalTemplateType(db *gorm.DB, templateId uint) (model.ProposalTemplateType, error) {
	var pTemplate model.ProposalTemplate
	err := db.Find(&pTemplate, templateId).Error
	if err != nil {
		log.Error().Msgf("get proposal template error: %+v", err)
		return 0, err
	}
	return pTemplate.Type, nil
}

func findProjectCreatedByProposal(db *gorm.DB, proposalId uint) (*model.Project, error) {
	log.Debug().Msgf("find project created by proposal: %d", proposalId)
	var createProjectProposal model.Proposal
	if err = db.Find(&createProjectProposal, proposalId).Error; err != nil {
		log.Error().Msgf("get create project proposal error: %+v", err)
		return nil, err
	}

	createdProject := model.Project{
		SIP: fmt.Sprintf("%d", createProjectProposal.Sip),
	}
	if err = db.Clauses(clause.Locking{
		Strength: "UPDATE",
		Options:  "NOWAIT",
	}).Model(&createdProject).Where("s_ip = ?", createdProject.SIP).First(&createdProject).Error; err != nil {
		log.Error().Msgf("get create project proposal error: %+v", err)
		return nil, err
	}

	return &createdProject, nil
}
