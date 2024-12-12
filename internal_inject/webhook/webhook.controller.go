package webhook_inject

import (
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"
)

type WebhookController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	WebhookService *WebhookService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var webhook WebhookController

	err := inject.Populate(&webhook, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var webhookGroup *gin.RouterGroup

	if fatherGroup != nil {
		webhookGroup = fatherGroup.Group("/webhook")
	} else {
		webhookGroup = webhook.Gin.Group("/webhook")
	}

	// no auth
	webhookGroup.POST("/tally", webhook.Tally)
}

func (c *WebhookController) Tally(ctx *gin.Context) {
	body, _ := ctx.GetRawData()
	log.Warn().Msgf("webhook>>tally: %s", string(body))

	ctx.JSON(http.StatusOK, api.Success(nil))
}
