package asset_records_inject

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type AssetRecordsController struct {
	// inject
	Gin     *gin.Engine          `inject:""`
	Db      *gorm.DB             `inject:""`
	Cfg     *config.Config       `inject:""`
	Service *AssetRecordsService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var assetRecords AssetRecordsController
	assetRecords.Service = NewAssetRecordsService(g.Db)

	err := inject.Populate(&assetRecords, g.Gin, g.Db, g.Cfg)
	if err != nil {
		panic(err)
	}

	var assetRecordsGroup *gin.RouterGroup
	var assetRecordsAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		assetRecordsGroup = fatherGroup.Group(BaseRoutePath)
		assetRecordsAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group(BaseRoutePath)
	} else {
		assetRecordsGroup = assetRecords.Gin.Group(BaseRoutePath)
		assetRecordsAuthGroup = assetRecords.Gin.Group("/", middleware.AuthRequired).Group(BaseRoutePath)
	}

	// No auth endpoints
	assetRecordsGroup.GET("/", assetRecords.List)
	assetRecordsGroup.GET("/:id", assetRecords.Detail)

	// Auth required endpoints
	assetRecordsAuthGroup.POST("/", assetRecords.Create)
}

// Create creates a new asset transfer
func (c *AssetRecordsController) Create(ctx *gin.Context) {
	var req CreateTransferRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	transferLog, err := c.Service.CreateTransfer(req.FromUser, req.ToUser, req.AssetName, req.Amount, req.Comment)
	if err != nil {
		log.Error().Msgf("Error creating transfer: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)

		// Handle specific error cases
		if err.Error() == ErrSameUserTransfer ||
			err.Error() == ErrInvalidAmount ||
			err.Error() == ErrNoAssetRecords ||
			err.Error() == ErrInsufficientBalance {
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}

		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusCreated, api.Success(map[string]any{
		FieldTransferID: transferLog.ID,
		FieldStatus:     StatusSuccess,
	}))
}

// List returns paginated asset transfer records with optional query filters
func (c *AssetRecordsController) List(ctx *gin.Context) {
	var queryParams TransferListQueryParams
	if err := ctx.ShouldBindQuery(&queryParams); err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	// Validate pagination parameters
	if queryParams.Page <= 0 {
		queryParams.Page = DefaultPageNumber
	}
	if queryParams.Size <= 0 || queryParams.Size > MaxPageSize {
		queryParams.Size = DefaultPageSize
	}

	transfers, total, err := c.Service.ListTransfers(queryParams.Page, queryParams.Size, queryParams.FromUser, queryParams.ToUser)
	if err != nil {
		log.Error().Msgf("Error listing transfers: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// Convert to response format
	response := make([]*TransferResponse, len(transfers))
	for i, transfer := range transfers {
		response[i] = &TransferResponse{
			ID:            transfer.ID,
			FromUser:      transfer.FromUser,
			ToUser:        transfer.ToUser,
			AssetName:     transfer.AssetName,
			Amount:        transfer.Amount,
			TransactionTs: transfer.TransactionTs,
			Result:        transfer.Result,
			Comment:       transfer.Comment,
		}
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  response,
	}))
}

// Detail returns details of a single asset transfer record
func (c *AssetRecordsController) Detail(ctx *gin.Context) {
	transferIDStr := ctx.Param("id")
	transferID, err := strconv.ParseUint(transferIDStr, 10, 32)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	transfer, err := c.Service.GetTransferByID(uint(transferID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusNotFound, api.BadRequest(errors.New(ErrTransactionNotFound)))
			return
		}
		log.Error().Msgf("Error getting transfer detail: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	response := &TransferResponse{
		ID:            transfer.ID,
		FromUser:      transfer.FromUser,
		ToUser:        transfer.ToUser,
		AssetName:     transfer.AssetName,
		Amount:        transfer.Amount,
		TransactionTs: transfer.TransactionTs,
		Result:        transfer.Result,
		Comment:       transfer.Comment,
	}

	ctx.JSON(http.StatusOK, api.Success(response))
}
