package datasrv_inject

import (
	"errors"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type DataSrvController struct {
	// Inject
	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	DataSrvService *DataSrvService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var dataSrv DataSrvController

	err := inject.Populate(&dataSrv, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var dataSrvGroup *gin.RouterGroup

	if fatherGroup != nil {
		dataSrvGroup = fatherGroup.Group("/data_srv")
	} else {
		dataSrvGroup = dataSrv.Gin.Group("/data_srv")
	}

	dataSrvGroup.GET("/aggr_scr", dataSrv.AggrScr)
}

func (c *DataSrvController) AggrScr(ctx *gin.Context) {
	// Fetch current season data from database
	currentSeason, err := model.GetCurrentSeason(c.Db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season error")))
		return
	}
	log.Debug().Msgf("current season: %+v", currentSeason)

	mintResult, err := c.DataSrvService.CalcMintRewards(ctx, currentSeason)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("calc mint rewards error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&NodeCalcResponse{
		SeasonName:                   currentSeason.Name,
		SeasonTotalCreditWithoutMint: mintResult.TotalSeasonCreditWithoutMint.String(),
		SeasonTotalMintCredit:        mintResult.TotalMetaforoCredits.String(),
		TotalWalletCount:             len(mintResult.UserCredits),
		ActivateWalletCount:          mintResult.ActivateWalletCount,
		MintRewardConfirmed:          currentSeason.MintRewardConfirmed,
		SeedSnapshoted:               currentSeason.SeedSnapshotSaved,
		Records:                      mintResult.DetailRecords,
	}))
}
