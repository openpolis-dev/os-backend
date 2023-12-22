package proposal

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/component"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/xiaosongfu/gormfind"
	"gorm.io/gorm"
)

// List handles the HTTP request to list proposals.
//
//		@summary	lists all proposals based on query params and return in JSON format
//		@router		/proposals [get]
//	    @Param          page            query           int  false   "which page"
//	    @Param          size            query           int  false   "size of each page"
//	    @Param          sort_field      query           string  false   "sort by which field"
//	    @Param          sort_order      query           string  false   "order of sort"                 Enum(asc desc)
//	    @Param          state           query           string  false   "state of proposal"   Enum(draft withdrawn voting passed failed rejected)
//	    @Param          category_id           query     int  false   "filter proposal records with specified category"
//		@success	200	{object}	api.Reply{data=api.ListReplyData{rows=FrontendProposalDetailRecord}}
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	queryParams := QueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}
	// Parse pagination
	page := api.ParseAndConvertPageParam(ctx)

	// Execute query
	querySeg := db.Model(&model.Proposal{})
	if queryParams.State != "" {
		if stateVal, found := model.ProposalStateIdNameMapping[queryParams.State]; found {
			querySeg = querySeg.Where("state = ?", stateVal)
		} else {
			sdk.LogUserSideError(ctx, fmt.Errorf("query proposal state %s error", queryParams.State))
			log.Warn().Msgf("query proposal state %s error", queryParams.State)
		}
	}

	if queryParams.CategoryId != 0 {
		querySeg.Where("proposal_category_id = ?", queryParams.CategoryId)
	}

	total, err := gormfind.Count(querySeg)
	if err != nil {
		log.Error().Msgf("get proposal count error: %+v, query sql: %s, query params: %+v", err, querySeg)
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query proposal error: %+v", err),
		})
		return
	}

	querySeg = querySeg.Joins("ProposalCategory")

	dbRcds, err := model.QueryRows[model.Proposal](querySeg, page)
	if err != nil {
		log.Error().Msgf("get proposal list error: %+v, query sql: %s, query params: %+v", err, querySeg)
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query proposal error: %+v", err),
		})
		return
	}

	// Transform proposal records to frontend format
	resultRows := lo.Map(dbRcds, func(r *model.Proposal, _ int) *FrontendProposalListRecord {
		return &FrontendProposalListRecord{
			ID:           r.ID,
			Title:        r.Title,
			Applicant:    r.Applicant,
			CategoryName: r.ProposalCategory.Name,
			State:        model.ProposalStateName[r.State],
			CreateTs:     r.CreateTs,
			PollState:    "",
		}
	})

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  resultRows,
	}))
}

// Save function builds proposal DB records, and save it in PendingSubmit state
//
//		@router		/proposals/create [post]
//		@summary	Create proposals with passed in data
//	  	@Param          JsonBody        body            CreateProposalData       true    "request json body"
//		@success	200	{object}	api.Reply{}
func Save(ctx *gin.Context) {
	// Parsing request to create proposal object
	var reqData CreateProposalData
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("parse request data error: %+v", err),
		})
		return
	}

	user, _, db, _ := api.ForContext(ctx)
	// TODO: Confirm whether permission is required here and add permission check if required

	proposalRecord, err := service.SaveProposalRecordToDB(db, &reqData, user.Wallet)
	if err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	responseData, err := ConvertProposalToFrontendDetailRecord(db, proposalRecord)
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
//	@summary	Create metaforo proposal and public to others
//	@Param          JsonBody        body            CreateProposalData       true    "request json body"
//	@success	200	{object}	api.Reply{}
func Create(ctx *gin.Context) {
	// Parsing request to create proposal object
	var reqData CreateProposalData
	if err := ctx.BindJSON(&reqData); err != nil {
		log.Error().Msgf("parse request data error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("parse request data error: %+v", err),
		})
		return
	}

	user, _, db, _ := api.ForContext(ctx)
	// TODO: Confirm whether permission is required here and add permission check if required

	proposalRecord, err := service.SaveProposalRecordToDB(db, &reqData, user.Wallet)
	if err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	if err := service.SaveProposalToMetaforo(db, proposalRecord, reqData.MetaforoAccessToken); err != nil {
		log.Error().Msgf("create metaforo proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	responseData, err := ConvertProposalToFrontendDetailRecord(db, proposalRecord)
	if err != nil {
		log.Error().Msgf("convert proposal to frontend format error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("load proposal data error")))
		return
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}

// Detail function returns proposal detail data
//
//		@router		/proposals/show/:id [post]
//		@summary	Create proposals with passed in data
//	  	@Param          JsonBody        body            CreateProposalData       true    "request json body"
//		@success	200	{object}	api.Reply{data=FrontendProposalDetailRecord}
func Detail(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	proposalRecord, err := service.GetProposalFromStringId(db, ctx.Param("id"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalRecord.ID)
			ctx.JSON(http.StatusNotFound, nil)
			return
		} else {
			sdk.LogUserSideError(ctx, err)
			log.Error().Msgf("parse proposal id %s error: %+v", ctx.Param("id"), err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
	}

	// Load proposal content block
	var proposalContents []*model.ProposalContentBlock
	if err := db.Where(model.ProposalContentBlock{ProposalID: proposalRecord.ID}).Find(&proposalContents).Error; err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal blocks error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
		return
	}

	// Load proposal components
	var proposalComponents []*model.ProposalComponentRecord
	if err := db.Where(model.ProposalComponentRecord{ProposalID: proposalRecord.ID}).Find(&proposalComponents).Error; err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("get proposal components error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
		return
	}

	// TODO: Migrate converting model.ProposalContentBlock to FrontendContentBlockRecord into function
	// TODO: Migrate converting model.ProposalComponentRecord to ComponentInstance into function
	responseData := FrontendProposalDetailRecord{
		ID:    proposalRecord.ID,
		Title: proposalRecord.Title,
		ContentBlocks: lo.Map(proposalContents, func(item *model.ProposalContentBlock, _ int) *FrontendContentBlockRecord {
			return &FrontendContentBlockRecord{
				Title:   item.Title,
				Content: item.Content,
			}
		}),
		ProposalCategoryId: proposalRecord.ProposalCategoryID,
		State:              model.ProposalStateName[proposalRecord.State],
		Components: lo.Map(proposalComponents, func(item *model.ProposalComponentRecord, _ int) *component.ComponentInstance {
			return &component.ComponentInstance{
				ID:          item.ID,
				ComponentId: item.ComponentId,
				Schema:      "",
				Data:        item.Data,
				CreateTs:    item.CreateTs,
			}
		}),
		Applicant:  proposalRecord.Applicant,
		IsApproved: false,
		CreateTs:   proposalRecord.CreateTs,
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}

// Update existing proposal
// 1. Only proposal in PendingSubmit, Withdrawn, Rejected state can be updated
// 2. Updating PendingSubmit proposal does not change the state and version number, and is update in place directly
// 3. Updating proposal in Withdrawn, Rejected state will create a new record and update the version number
func Update(ctx *gin.Context) {}

// Withdraw, Approve and Reject change proposal to named state and update the Metaforo label

func Withdraw(ctx *gin.Context) {
	user, _, db, _ := api.ForContext(ctx)
	proposalIdStr := ctx.Param("id")
	err := updateProposalState(db, user, proposalIdStr, model.ProposalStateWithdrawn)
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

func Approve(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, api.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, api.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proposalIdStr := ctx.Param("id")
	err = updateProposalState(db, user, proposalIdStr, model.ProposalStateApproved)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
		} else {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("update proposal %s state to approved error: %+v", proposalIdStr, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("approve proposal error")))
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func Reject(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)

	//  check permission
	ok, err := enforcer.HasRoleForUser(formattedWallet, api.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get cityhall permission error")))
		return
	}

	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, api.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	proposalIdStr := ctx.Param("id")
	err = updateProposalState(db, user, proposalIdStr, model.ProposalStateRejected)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
		} else {
			sdk.LogServerErrorToSentry(ctx, err)
			log.Error().Msgf("update proposal %s state to rejected error: %+v", proposalIdStr, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("approve proposal error")))
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func updateProposalState(db *gorm.DB, user *middleware.CurUser, proposalStrId string, newState model.ProposalState) error {
	proposalRecord, err := service.GetProposalFromStringId(db, proposalStrId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalStrId)
		}
		return err
	}

	if proposalRecord.State != int(model.ProposalStateDraft) {
		return fmt.Errorf("proposal %s in %s state can;t be withdrawn", proposalStrId, proposalRecord.StateName())
	}

	// Check whether user has permission to the change the proposal state
	switch newState {
	case model.ProposalStateWithdrawn:
		if !strings.EqualFold(user.Wallet, proposalRecord.Applicant) {
			return errors.New("proposal can only be withdrawn by applicant")
		}
		err = db.Model(&proposalRecord).Update("state = ?", model.ProposalStateWithdrawn).Error
		// TODO: Update Metaforo Label: Remove old draft and add new, verify whether metaforo can handle this
	case model.ProposalStateApproved:
		// TODO: Update Metaforo Label: Remove old draft and add new, verify whether metaforo can handle this
		err = db.Model(&proposalRecord).Update("state = ?", model.ProposalStateApproved).Error
		// TODO: Update vote record related to this proposal
	case model.ProposalStateRejected:
		// TODO: Update Metaforo Label: Remove old draft and add new, verify whether metaforo can handle this
		err = db.Model(&proposalRecord).Update("state = ?", model.ProposalStateRejected).Error
	default:
		return fmt.Errorf("changing proposal from state %s to %s is not approved", proposalRecord.StateName(), model.ProposalStateName[newState])
	}
	return nil
}
