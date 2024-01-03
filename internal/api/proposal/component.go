package proposal

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type ComponentResponse struct {
	ID            uint   `json:"id"`
	Name          string `json:"name"`
	Schema        string `json:"schema"`
	ScreenshotUri string `json:"screenshot_uri"`
}

// ListComponents returns component list from database
//
//	@summary	list components from DB
//	@router		/proposal_components/ [get]
//	@tags		Proposal
//
//	@success	200	{object}	api.Reply{data=[]ComponentResponse}
func ListComponents(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	var records []*ComponentResponse
	err := db.Model(&model.ProposalComponent{}).Find(&records).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(500, api.ServerError(errors.New("list components failed")))
		return
	}
	ctx.JSON(200, api.Success(records))
}

// GetComponent returns component detail for given ID
//
//	@summary	Get component detail from DB
//	@router		/proposal_components/:id [get]
//	@param		id	path	number	true	"component ID"
//	@tags		Proposal
//
//	@success	200	{object}	api.Reply{data=ComponentResponse}
func GetComponent(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	idParam := ctx.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	var rcd *ComponentResponse
	err = db.First(&rcd, id).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(500, api.ServerError(errors.New("list components failed")))
		return
	}
	ctx.JSON(200, api.Success(rcd))
}
