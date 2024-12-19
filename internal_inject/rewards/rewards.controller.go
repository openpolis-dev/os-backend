package rewards_inject

import (
	"fmt"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"gorm.io/gorm"
)

type RewardsController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	RewardsService *RewardsService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var rewards RewardsController

	err := inject.Populate(&rewards, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var rewardsAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		rewardsAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/rewards")
	} else {
		rewardsAuthGroup = rewards.Gin.Group("/", middleware.AuthRequired).Group("/rewards")
	}

	// auth
	rewardsAuthGroup.POST("/approve_mint_reward", rewards.ApproveMintReward)
	rewardsAuthGroup.POST("/snapshot_seed", rewards.SnapshotSeed)
	rewardsAuthGroup.POST("/approve_mint_snap_seed", rewards.ApproveMintAndSnapshotSeed)
}

func (c *RewardsController) ApproveMintReward(ctx *gin.Context) {
	err := c.RewardsService.DoApproveMintReward(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("approve mint reward error: %+v", err)))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *RewardsController) SnapshotSeed(ctx *gin.Context) {
	seedSnapshotAt, err := c.RewardsService.DoSnapshotSeed(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("snapshot seed error: %+v", err)))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(fmt.Sprintf("SEED snapshoted at %d", seedSnapshotAt)))

}

func (c *RewardsController) ApproveMintAndSnapshotSeed(ctx *gin.Context) {
	err := c.RewardsService.DoApproveMintReward(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("approve mint and snapshot seed error: %+v", err)))
		return
	}
	seedSnapshotAt, err := c.RewardsService.DoSnapshotSeed(ctx)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(fmt.Errorf("approve mint and snapshot seed error: %+v", err)))
		return
	}
	ctx.JSON(http.StatusOK, api.Success(fmt.Sprintf("SEED snapshoted at %d", seedSnapshotAt)))

}
