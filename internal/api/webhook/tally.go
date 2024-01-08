package webhook

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
)

// Tally is the webhook for the tally.so
//
//	@summary	Tally is the webhook for the tally.so
//	@tags		Webhook
//	@accept		json
//	@produce	json
//	@success	200	{object}	api.Reply{}
//	@router		/webhook/tally [post]
func Tally(ctx *gin.Context) {
	body, _ := ctx.GetRawData()
	log.Warn().Msgf("webhook>>tally: %s", string(body))

	ctx.JSON(http.StatusOK, api.Success(nil))
}
