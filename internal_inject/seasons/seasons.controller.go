package seasons_inject

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type SeasonsController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	SeasonsService *SeasonsService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var seasons SeasonsController

	err := inject.Populate(&seasons, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var seasonsGroup *gin.RouterGroup

	if fatherGroup != nil {
		seasonsGroup = fatherGroup.Group("/public_data")
	} else {
		seasonsGroup = seasons.Gin.Group("/public_data")
	}

	seasonsGroup.GET("/", seasons.List)
	seasonsGroup.GET("/current", seasons.Current)
}

func (c *SeasonsController) List(ctx *gin.Context) {
	var seasonRcds []*model.Season
	err := c.Db.Model(&model.Season{}).Find(&seasonRcds).Error

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get seasons error")))
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

func (c *SeasonsController) Current(ctx *gin.Context) {
	currSeason, err := model.GetCurrentSeason(c.Db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season error")))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(&SeasonResponse{
		ID:      currSeason.ID,
		Name:    currSeason.Name,
		StartAt: fmt.Sprintf("%d", currSeason.StartAt),
		EndAt:   fmt.Sprintf("%d", currSeason.EndAt),
	}))
}
