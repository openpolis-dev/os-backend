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
	"github.com/theseed-labs/os-backend/internal/api/component"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
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
//	  	@Param          JsonBody        body            CreateProposalData       true    "request json body"
//		@success	200	{object}	api.Reply{}
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

	log.Error().Msgf("TTT: request data: %+v", reqData)

	user, _, db, _ := api.ForContext(ctx)
	// TODO: Confirm whether permission is required here and add permission check if required

	// Init proposal record to get ID
	proposalRecord := model.Proposal{
		CreateTs:           time.Now().UTC().Unix(),
		Title:              reqData.Title,
		Applicant:          common.FormatUserWallet(user.Wallet),
		ProposalCategoryID: reqData.ProposalCategoryId,
	}

	if err := db.Create(&proposalRecord).Error; err != nil {
		log.Error().Msgf("create proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	// Create proposal content blocks
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, block := range reqData.ContentBlocks {
			if err := db.Model(&model.ProposalContentBlock{}).Create(&model.ProposalContentBlock{
				ProposalID: proposalRecord.ID,
				Title:      block.Title,
				Content:    block.Content,
				CreateTs:   time.Now().UTC().Unix(),
			}).Error; err != nil {
				log.Error().Msgf("create proposal block error: %+v, block data: %+v", err, block)
				sdk.LogServerErrorToSentry(ctx, err)
				log.Error().Msgf("create proposal block error: %+v", err)
			}
		}
		return nil
	}); err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("create proposal block error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
	}

	// Create proposal components
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, componentData := range reqData.Components {
			// Try to get component record from DB
			componentRecord := model.Component{
				Name: componentData.Name,
			}
			if err := tx.Where(&componentRecord).Error; err != nil {
				tx.Rollback()
				log.Error().Msgf("create proposal component error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
				return err
			}

			// Create proposal component record and save to DB
			if err := db.Create(&model.ProposalComponentRecord{
				CreateTs:    time.Now().UTC().Unix(),
				ComponentId: componentRecord.ID,
				ProposalID:  proposalRecord.ID,
				Data:        componentData.Data,
			}).Error; err != nil {
				sdk.LogServerErrorToSentry(ctx, err)
				log.Error().Msgf("create proposal component error: %+v", err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
				return err
			}
		}
		return nil
	}); err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("create proposal component error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	//// Store proposal data into Metaforo
	//// The CreateProposal function will be invoked for new created proposal while UpdateProposal for existing one.
	//// In our system, each proposal submitted will be saved as a new record, and the ProposalID field will be used to group all versions of the same proposal.
	proposalVer := 0
	//var metaforoProposal metaforo.ProposalResponse
	//// TODO: Build metaforo proposal data and send
	//if reqData.ProposalID != "" {
	//	metaforoProposal, err := metaforo.UpdateProposal(reqData.MetaforoAccessToken)
	//	if err != nil {
	//		log.Error().Msgf("update metaforoProposal error: %+v", err)
	//		sdk.LogServerErrorToSentry(ctx, err)
	//		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("update metaforoProposal error")))
	//		return
	//	}
	//
	//	log.Debug().Msgf("TTT: Update metaforoProposal: %+v", metaforoProposal)
	//
	//	// Upgrade proposal version
	//	var proposalVers []int
	//	db.Where(&model.Proposal{ProposalRecordId: reqData.ProposalID}).Order("version desc").Pluck("version", &proposalVers)
	//	if len(proposalVers) > 0 {
	//		proposalVer = proposalVers[0] + 1
	//	}
	//} else {
	//	metaforoProposal, err := metaforo.CreateProposal(reqData.MetaforoAccessToken)
	//	if err != nil {
	//		log.Error().Msgf("update metaforoProposal error: %+v", err)
	//		sdk.LogServerErrorToSentry(ctx, err)
	//		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create metaforoProposal error")))
	//		return
	//	}
	//
	//	log.Debug().Msgf("TTT: New metaforoProposal: %+v", metaforoProposal)
	//}
	//
	//// Save data backed from metaforo API response to DB
	//proposalRecord.ProposalRecordId = fmt.Sprintf("metaforo:%d", metaforoProposal.Data.Thread.Id)
	proposalRecord.Version = uint(proposalVer)
	//proposalRecord.ArveaveHash = metaforoProposal.Data.Thread.EditHistory.Lists[0].Arweave

	var proposalBlocks []*model.ProposalContentBlock
	if err := db.Where(&model.ProposalContentBlock{ProposalID: proposalRecord.ID}).Find(&proposalBlocks).Error; err != nil {
		log.Error().Msgf("get proposal blocks error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	var proposalComponentRecords []*model.ProposalComponentRecord
	if err := db.Where(&model.ProposalComponentRecord{ProposalID: proposalRecord.ID}).Find(&proposalComponentRecords).Error; err != nil {
		log.Error().Msgf("get proposal components error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal error")))
		return
	}

	proposalContentResponse := lo.Map(proposalBlocks, func(item *model.ProposalContentBlock, _ int) *FrontendContentBlockRecord {
		return &FrontendContentBlockRecord{
			Title:   item.Title,
			Content: item.Content,
		}
	})

	proposalComponentResponse := lo.Map(proposalComponentRecords, func(item *model.ProposalComponentRecord, _ int) *component.ComponentInstance {
		return &component.ComponentInstance{
			ID:          item.ID,
			ComponentId: item.ComponentId,
			Schema:      "",
			Data:        item.Data,
			CreateTs:    item.CreateTs,
		}
	})

	responseData := FrontendProposalDetailRecord{
		ID:                 proposalRecord.ID,
		Title:              proposalRecord.Title,
		ContentBlocks:      proposalContentResponse,
		ProposalCategoryId: proposalRecord.ProposalCategoryID,
		State:              model.ProposalStateName[proposalRecord.State],
		Components:         proposalComponentResponse,
		Applicant:          proposalRecord.Applicant,
		IsApproved:         false,
		CreateTs:           proposalRecord.CreateTs,
	}

	// Return to frontend
	ctx.JSON(http.StatusOK, api.Success(responseData))
}
