package webhook

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
)

// Tally is the webhook for the tally.so
// @Summary	Tally is the webhook for the tally.so
// @Tags		Webhook
// @Accept		json
// @Produce		json
// @Success	200	{object}	api.Reply{}
// @Router		/webhook/tally [post]
func Tally(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, api.Success(nil))
}
