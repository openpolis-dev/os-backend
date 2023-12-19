package proposal

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"github.com/xiaosongfu/gormfind"
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
			ID:           0,
			Title:        r.Title,
			Applicant:    r.Applicant,
			CategoryName: r.ProposalCategory.Name,
			State:        model.ProposalStateName[r.State],
			CreateTs:     0,
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

// Create function builds proposal DB records, then invoke Metaforo API to create in metaforo site.
//
//		@router		/proposals/create [post]
//		@summary	Create proposals with passed in data
//	  	@Param          JsonBody        body            CreateReq       true    "request json body"
//		@success	200	{object}	api.Reply{}
func Create(ctx *gin.Context) {
	// Parsing request to create proposal object
	var reqData CreateProposalData
	if err := ctx.BindJSON(&reqData); err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("parse request data error: %+v", err),
		})
		return
	}

	user, _, db, _ := api.ForContext(ctx)
	// TODO: Confirm whether permission is required here?
	// TODO: Validate whether user has permission to create the proposal

	// Store proposla data into Metaforo
	// The CreateProposal function will be invoked for new created propsal while UpdateProposal for existing one.
	// In our system, each proposal submitted will be saved as a new record, and the ProposalID field will be used to group all versions of the same proposal.
	proposalVer := 0
	var metaforoProposal metaforo.ProposalResponse
	// TODO: Build metaforo proposal data and send
	if reqData.ProposalId != "" {
		metaforoProposal, err := metaforo.UpdateProposal(reqData.MetaforoAccessToken)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("update metaforoProposal error")))
			return
		}

		log.Debug().Msgf("TTT: Update metaforoProposal: %+v", metaforoProposal)

		// Upgrade proposal version
		var proposalVers []int
		db.Where(&model.Proposal{ProposalRecordId: reqData.ProposalId}).Order("version desc").Pluck("version", &proposalVers)
		if len(proposalVers) > 0 {
			proposalVer = proposalVers[0] + 1
		}
	} else {
		metaforoProposal, err := metaforo.CreateProposal(reqData.MetaforoAccessToken)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("create metaforoProposal error")))
			return
		}

		log.Debug().Msgf("TTT: New metaforoProposal: %+v", metaforoProposal)
	}

	// Create DB proposal record with returned Metaforo data
	proposalRecord := model.Proposal{
		CreateTs:         time.Now().UTC().Unix(),
		Title:            "",
		ContentBlocks:    nil,
		Components:       nil,
		ProposalRecordId: fmt.Sprintf("metaforo:%d", metaforoProposal.Data.Thread.Id),
		Version:          uint(proposalVer),
		Applicant:        common.FormatUserWallet(user.Wallet),
		ArveaveHash:      metaforoProposal.Data.Thread.EditHistory.Lists[0].Arweave,
	}

	if err := db.Create(&proposalRecord).Error; err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	// TODO: Process other proposal records

	// Process components
	//if err := db.Transaction(func(tx *gorm.DB) error {
	//	for _, componentData := range reqData.Components {
	//		componentRecord := db.Model(&model.Component{}).Where("name = ?", component.Name)
	//		proposalComponentRcd := model.ProposalComponentRecord{
	//			CreateTs:    0,
	//			ComponentId: reqData.Components.,
	//			ProposalId:  0,
	//			Data:        "",
	//		}
	//		err := tx.Save(&component).Error
	//		if err != nil {
	//			tx.Rollback()
	//			sdk.LogServerErrorToSentry(ctx, err)
	//			return err
	//		}
	//	}
	//	return nil
	//}); err != nil {
	//	ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
	//}

	ctx.JSON(http.StatusOK, api.Success(proposalRecord))
}
