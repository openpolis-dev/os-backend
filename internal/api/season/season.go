package season

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type SeasonResponse struct {
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

// List returns all seasons currently existing in database
//
//	@Summary	returns all seasons currently existing in database
//	@Router		/seasons/ [get]
//	@Tag		seasons
//
//	@Success	200	{object}	[]SeasonResponse
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	var seasonRcds []*model.Season
	err := db.Model(&model.Season{}).Find(&seasonRcds).Error

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
	}

	resp := lo.Map(seasonRcds, func(seasonRcd *model.Season, _ int) *SeasonResponse {
		return &SeasonResponse{
			ID:      seasonRcd.ID,
			Name:    seasonRcd.Name,
			StartAt: fmt.Sprintf("%d", seasonRcd.StartAt),
			EndAt:   fmt.Sprintf("%d", seasonRcd.EndAt),
		}
	})
	ctx.JSON(http.StatusOK, api.Success(&resp))

}

// Current returns the current season.
//
//	@Summary	returns current season
//	@Router		/seasons/curr [get]
//	@Tag		seasons
//
//	@Success	200	{object}	SeasonResponse
func Current(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	currSeason, err := model.GetCurrentSeason(db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(&SeasonResponse{
		ID:      currSeason.ID,
		Name:    currSeason.Name,
		StartAt: fmt.Sprintf("%d", currSeason.StartAt),
		EndAt:   fmt.Sprintf("%d", currSeason.EndAt),
	}))
}
