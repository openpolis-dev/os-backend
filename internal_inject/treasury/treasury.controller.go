package treasury_inject

import (
	"errors"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type TreasuryController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	TreasuryService *TreasuryService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var treasury TreasuryController

	err := inject.Populate(&treasury, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var treasuryGroup *gin.RouterGroup
	var treasuryAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		treasuryGroup = fatherGroup.Group("/treasury")
		treasuryAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/treasury")
	} else {
		treasuryGroup = treasury.Gin.Group("/treasury")
		treasuryAuthGroup = treasury.Gin.Group("/", middleware.AuthRequired).Group("/treasury")
	}

	// no auth
	treasuryGroup.GET("/current", treasury.GetOrCreateCurrentAssetRecords)

	// auth
	treasuryAuthGroup.POST("/update_assets", treasury.UpdateAssets)
}

func (c *TreasuryController) GetOrCreateCurrentAssetRecords(ctx *gin.Context) {
	currQuarterTreasuryRecord, err := model.TreasuryAssetHelper.GetOrCreateCurrentSeasonRecord(c.Db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get or create current quarter treasury record error detail:"+err.Error())))
		return
	}

	treasuryAssetResp, err := currQuarterTreasuryRecord.ToTreasuryAssetsResponse(c.Db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get or create current quarter treasury record error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(treasuryAssetResp))
}

func (c *TreasuryController) UpdateAssets(ctx *gin.Context) {
	httpCode, reply := c.TreasuryService.UpdateAssets(ctx)

	ctx.JSON(httpCode, reply)
}
