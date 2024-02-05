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
)

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
	if err := ctx.Bind(&queryParams); err != nil {
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

	if queryParams.Sip != "" {
		querySql += fmt.Sprintf(" AND sip != 0")
	}

	total, resultRows, err := generateFrontendProposalRecords(db, querySql, page)
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
		var err error
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

	responseData, err := ConvertProposalToFrontendDetailRecord(db, proposalRecord, startPostId, metaforoAccessToken, cfg.MetaforoData.GroupName)
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

	// Proposal is in updatable state

	// Parsing request to create proposal object
	var reqData CreateOrUpdateProposalData
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	proposalRecord, err := SaveProposalRecordToDB(db, &reqData, user.Wallet, ctx.Param("id"), cfg)
	if err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	// FIXME: refactor here: If not submitting to metaforo, a new version will be created in DB but no metaforo record.
	// FIXME: Do we need to force passing the metaforo access token if not in pending submit state?
	if reqData.SubmitToMetaforo {
		if err := SaveProposalToMetaforo(db, proposalRecord, proposalRecord.VoteType, reqData.VoteOptions, reqData.MetaforoAccessToken, reqData.EditorType, cfg.MetaforoData.GroupName); err != nil {
			log.Error().Msgf("create metaforo proposal error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
			return
		}

		// If the publicity second is 0, update the db proposal to voting state
		if proposalRecord.PublicitySecond == 0 {
			log.Debug().Msgf("proposal has no publicity time, change to approved status directly")
			_, err = updateProposalState(db, user, proposalIdStr, model.ProposalStateApproved, cfg)
		} else if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			if err = createJobToUpdateNoVoteProposalToNextState(db, proposalRecord, proposalRecord.CreateTs+proposalRecord.PublicitySecond, model.ProposalStateApproved); err != nil {
				log.Error().Msgf("create proposal state change error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
				return
			}
			if err = createJobToUpdateNoVoteProposalToNextState(db, proposalRecord, proposalRecord.CreateTs+proposalRecord.PublicitySecond+proposalRecord.PendingExecutionSecond, model.ProposalStateExecuted); err != nil {
				log.Error().Msgf("create proposal state change error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
				return
			}
		}
	}

	responseData, err := ConvertProposalToFrontendDetailRecord(db, proposalRecord, 0, reqData.MetaforoAccessToken, cfg.MetaforoData.GroupName)
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
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request data error: %+v", err)))
		return
	}

	user, _, db, cfg := api.ForContext(ctx)

	proposalRecord, err := SaveProposalRecordToDB(db, &reqData, user.Wallet, "", cfg)
	if err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	if reqData.SubmitToMetaforo {
		if err := SaveProposalToMetaforo(db, proposalRecord, reqData.VoteType, reqData.VoteOptions, reqData.MetaforoAccessToken, reqData.EditorType, cfg.MetaforoData.GroupName); err != nil {
			log.Error().Msgf("create metaforo proposal error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
			return
		}

		// If the publicity second is 0, update the db proposal to voting state or pending execution state, and handle SIP data
		if proposalRecord.PublicitySecond == 0 {
			log.Debug().Msgf("proposal has no publicity time, change to approved status directly")
			_, err = updateProposalState(db, user, fmt.Sprintf("%d", proposalRecord.ID), model.ProposalStateApproved, cfg)
		} else if proposalRecord.VoteType == model.ProposalVoteTypeNone {
			if err = createJobToUpdateNoVoteProposalToNextState(db, proposalRecord, proposalRecord.CreateTs+proposalRecord.PublicitySecond, model.ProposalStateApproved); err != nil {
				log.Error().Msgf("create proposal state change error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
				return
			}
			if err = createJobToUpdateNoVoteProposalToNextState(db, proposalRecord, proposalRecord.CreateTs+proposalRecord.PublicitySecond+proposalRecord.PendingExecutionSecond, model.ProposalStateExecuted); err != nil {
				log.Error().Msgf("create proposal state change error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
				return
			}
		}

		db.First(&proposalRecord, proposalRecord.ID)
	}

	responseData, err := ConvertProposalToFrontendDetailRecord(db, proposalRecord, 0, reqData.MetaforoAccessToken, cfg.MetaforoData.GroupName)
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
	_, err := updateProposalState(db, user, proposalIdStr, model.ProposalStateWithdrawn, cfg)
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
	_, err = updateProposalState(db, user, proposalIdStr, model.ProposalStateApproved, cfg)
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
	if err := ctx.BindJSON(&rejectRequestData); err != nil {
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
	proposalRecord, err := updateProposalState(db, user, proposalIdStr, model.ProposalStateRejected, cfg)
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
		ProposalID:      proposalRecord.ID,
		Content:         rejectRequestData.Reason,
		IsHidden:        false,
		IsRejectComment: true,
	}
	db.Save(&rejectComment)

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
	db.Model(&rejectComment).Update("metaforo_comment_id", fmt.Sprintf("%d", commentData.Id))

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
	if err := ctx.Bind(&queryParams); err != nil {
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

	total, resultRows, err := generateFrontendProposalRecords(db, querySql, page)
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

// GetProposalsUsedForCreatingProjects returns proposals that is created by request user and from creating proposal template
//
//	@summary	Returns creating project and executed proposals created by login user, if category_id is not specified, all proposals for opening project will be returned
//	@tags		Proposal
//	@router		/proposals/creating_project_proposals [get]
//	@param		category_id	query		int	false	"Limit the category of creating project proposal"
//	@success	200			{object}	api.Reply{data=FrontendProposalDetailRecord}
func GetProposalsUsedForCreatingProjects(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)
	categoryIdStr := ctx.Query("category_id")

	// Get template id which match the
	var newProjectTemplateIds []uint
	tmplQueryParams := model.ProposalTemplate{
		Type: model.ProposalTemplateTypeNewProject,
	}
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
		} else {
			tmplQueryParams.ProposalCategoryID = uint(categoryId)
		}
	}

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

	querySql := fmt.Sprintf("%s WHERE applicant = '%s' AND proposal_template_id IN (%s) AND state = %d",
		ListProposalsSQL,
		common.FormatUserWallet(user.Wallet),
		strings.Join(lo.Map(newProjectTemplateIds, func(tmpId uint, _ int) string {
			return fmt.Sprintf("%d", tmpId)
		}), ","),
		model.ProposalStateExecuted,
	)

	_, resultRows, err := generateFrontendProposalRecords(db, querySql, nil)
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

// Internal function to handle duplicated logic of updating proposal state
func updateProposalState(db *gorm.DB, user *middleware.CurUser, proposalStrId string, newState model.ProposalState, cfg *config.Config) (*model.Proposal, error) {
	proposalRecord, err := GetProposalFromStringId(db, proposalStrId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalStrId)
		}
		return nil, err
	}

	// Check whether user has permission to the change the proposal state
	switch newState {
	case model.ProposalStateWithdrawn:
		if !strings.EqualFold(user.Wallet, proposalRecord.Applicant) {
			return nil, errors.New("proposal can only be withdrawn by applicant")
		}

		var voteRecords []*model.ProposalVoteRecord
		err = db.Model(proposalRecord).Association("VoteRecords").Find(&voteRecords)
		if err != nil {
			log.Error().Msgf("get vote records error: %+v", err)
			return nil, err
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
				return nil, err
			}
		}

		proposalRecord.State = int(model.ProposalStateWithdrawn)
		err = db.Save(&proposalRecord).Error
		// TODO: Update Metaforo Label: Remove old label and add new, verify whether metaforo can handle this
		if err != nil {
			log.Error().Msgf("change proposal to withdrawn error")
			return nil, err
		}
	case model.ProposalStateApproved:
		return nil, db.Transaction(func(tx *gorm.DB) error {
			proposalRecord.State = int(model.ProposalStateApproved)

			var pTemplate *model.ProposalTemplate
			if err := tx.Model(&proposalRecord).Association("ProposalTemplate").Find(&pTemplate); err != nil {
				log.Error().Msgf("get proposal template error: %+v", err)
				return err
			}

			if pTemplate != nil && pTemplate.Type == model.ProposalTemplateTypeCloseProject {
				createProjectProposal := model.Proposal{ID: proposalRecord.AssociateProposalId}
				if err = db.Find(&createProjectProposal).Error; err != nil {
					log.Error().Msgf("get creating project proposal error: %+v", err)
					return err
				}
				proposalRecord.Sip = createProjectProposal.Sip
			} else {
				var maxSipVal int
				if err := db.Model(&model.Proposal{}).Select("max(sip)").Limit(1).Pluck("sip", &maxSipVal).Error; err != nil {
					log.Error().Msgf("get vote records error: %+v", err)
					return err
				}

				if maxSipVal != 0 {
					proposalRecord.Sip = maxSipVal + 1
				} else {
					log.Error().Msgf("TTT: init sip val: %d", cfg.ProposalData.SipInitNumber)
					proposalRecord.Sip = cfg.ProposalData.SipInitNumber
				}
			}

			err = tx.Updates(&proposalRecord).Error
			var voteRecords []*model.ProposalVoteRecord
			err = tx.Model(proposalRecord).Association("VoteRecords").Find(&voteRecords)
			if err != nil {
				log.Error().Msgf("get vote records error: %+v", err)
				return err
			}

			if proposalRecord.VoteType == model.ProposalVoteTypeNone {
				if proposalRecord.PendingExecutionSecond == 0 {
					proposalRecord.State = int(model.ProposalStateExecuted)
				} else {
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
						Delete(&model.ProposalComponentRecord{}, &model.ProposalComponentRecord{ProposalID: proposalRecord.ID, ComponentID: 0}).Error; err != nil {
						log.Error().Msgf("create proposal state change error: %+v", err)
						return err
					}

					if err = createJobToUpdateNoVoteProposalToNextState(tx, proposalRecord, model.GetCurrentUtcEpochSecond()+proposalRecord.PendingExecutionSecond, model.ProposalStateExecuted); err != nil {
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
					}
				}
				proposalRecord.State = int(model.ProposalStateVoting)
			}
			err = tx.Updates(&proposalRecord).Error
			if err != nil {
				log.Error().Msgf("change proposal to approved error")
				return err
			}
			return nil
		})
	case model.ProposalStateRejected:
		// TODO: Update Metaforo Label: Remove old label and add new, verify whether metaforo can handle this
		proposalRecord.State = int(model.ProposalStateRejected)
		err = db.Save(&proposalRecord).Error
		if err != nil {
			log.Error().Msgf("change proposal to rejected error")
			return nil, err
		}
	default:
		return nil, fmt.Errorf("changing proposal from state %s to %s is not approved", proposalRecord.StateName(), model.ProposalStateName[newState])
	}
	return proposalRecord, nil
}

func generateFrontendProposalRecords(db *gorm.DB, querySql string, page *gormfind.Page) (int64, []*FrontendProposalListRecord, error) {
	var tmpRcd []*FrontendProposalListRecord
	var countTx *gorm.DB
	countTx = db.Raw(querySql).Scan(&tmpRcd)
	if err := countTx.Error; err != nil {
		log.Error().Msgf("get proposal count error: %+v", err)
		return 0, nil, err
	}
	total := countTx.RowsAffected

	if page != nil {
		orderByClause := fmt.Sprintf("%s %s ", *page.SortField, *page.Order)
		querySql += fmt.Sprintf("\nORDER BY %s ", orderByClause)
		querySql += fmt.Sprintf("LIMIT %d OFFSET %d", page.Size, (page.Page-1)*page.Size)
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

type createProjectProposalContentBlockStruct struct {
	ComponentId int    `json:"component_id"`
	Name        string `json:"name"`
	Schema      string `json:"schema"`
	Data        any    `json:"data"`
	Id          int    `json:"id,omitempty"`
	CreateTs    int    `json:"create_ts,omitempty"`
}

func createJobToUpdateNoVoteProposalToNextState(db *gorm.DB, proposal *model.Proposal, jobExecTs int64, nextState model.ProposalState) error {
	var err error
	proposalComponentRecord := model.ProposalComponentRecord{
		ProposalID:  proposal.ID,
		ComponentID: 0,
	}
	if err = db.Model(&proposalComponentRecord).
		Where(map[string]any{"proposal_id": proposal.ID, "component_id": 0}). // Note: 0 won't be passed to query if using struct data
		First(&proposalComponentRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			db.Create(&proposalComponentRecord)
		} else {
			log.Error().Msgf("create proposal component record error: %+v", err)
			return err
		}
	} else {
		log.Debug().Msgf("automation for updating state has already created, return")
		return nil
	}

	updateProposalStateTaskParams := map[string]any{
		"proposal_id": proposal.ID,
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
		ProposalId:                proposal.ID,
		LastExecTs:                0,
		NextExecTs:                jobExecTs,
		JobParams:                 string(jobParamsStr),
		VoteResult:                "",
		VoteType:                  proposal.VoteType,
		State:                     model.CronJobStateActive,
		LastExecResult:            "",
	}
	createTaskTx := db.Where(model.CronJob{
		HandlerName:               internal.TaskUpdateProposalState,
		ProposalComponentRecordId: int(proposalComponentRecord.ID),
		ProposalId:                proposal.ID}).
		Assign(&finTask).FirstOrCreate(&finTask)

	if createTaskTx.Error != nil {
		log.Error().Msgf("create proposal fin task error: %+v", createTaskTx.Error)
		return createTaskTx.Error
	} else if createTaskTx.RowsAffected == 0 {
		log.Warn().Msgf("proposal fin task already exists: %+v", finTask)
	}

	return nil
}
