package adminApisCtrl

import (
	"errors"
	"net/http"
	"time"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

// AdminApisCtrl is the controller for the admin APIs
type AdminApisCtrl struct {
	Router *gin.Engine `inject:""`
	Db     *gorm.DB    `inject:""`
}

// this func for auto make inject register file
func Register() {
	g := global_object.GetGlobalObject()

	var ctrl AdminApisCtrl

	err := inject.Populate(&ctrl, g.Gin, g.Db)
	if err != nil {
		panic(err)
	}

	// add router
	ctrl.Router.GET("/test_inject", ctrl.testInject)
}

func (ctrl *AdminApisCtrl) testInject(c *gin.Context) {
	now := time.Now().In(internal.ProjectTimezone).Unix()
	var currSeason model.Season
	err := ctrl.Db.Model(&model.Season{}).
		Where("start_at < ?", now).
		Order("start_at desc").
		First(&currSeason).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season error")))
		return
	}

	c.JSON(http.StatusOK, api.Success(currSeason))
}
