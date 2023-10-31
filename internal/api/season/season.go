package season

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
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
//	@Success	200	{object}	model.Season
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	var seasonRcds []*model.Season
	err := db.Model(&model.Season{}).Find(&seasonRcds).Error

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
	}

	resp := lo.Map(seasonRcds, func(seasonRcd *model.Season, _ int) *SeasonResponse {
		return &SeasonResponse{
			ID:      seasonRcd.ID,
			Name:    seasonRcd.Name,
			StartAt: seasonRcd.StartAt.In(internal.ProjectTimezone).String(),
			EndAt:   seasonRcd.EndAt.In(internal.ProjectTimezone).String(),
		}
	})
	ctx.JSON(http.StatusOK, api.Success(&resp))

}
