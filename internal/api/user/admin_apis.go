package user

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type AdminCheckVotePermissionRequest struct {
	VoteGateId  int      `json:"vote_gate_id"`
	UserWallets []string `json:"user_wallets"`
}

func CheckVotePermission(ctx *gin.Context) {
	req := AdminCheckVotePermissionRequest{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	db := api.ForContextOnlyDB(ctx)

	usersSeepassData := make(map[string]*sdk.SeepassResponse)
	for _, wallet := range req.UserWallets {
		seepassData, err := api.GetCachedSeepassData(sdk.GetSppClient(), wallet, false)
		if err != nil {
			log.Error().Msgf("get seepass data error: %+v", err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
		usersSeepassData[wallet] = seepassData
	}

	var voteGate *model.ProposalVoteGate
	err = db.Model(&voteGate).Where("id = ?", req.VoteGateId).First(&voteGate).Error
	if err != nil {
		log.Error().Msgf("get vote gate error: %+v", err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	checkResult := make(map[string]bool)
	for userWallet, seepassData := range usersSeepassData {
		checkResult[userWallet] = proposal.IsUserMetVoteGate(seepassData, voteGate)
	}

	responseData := make(map[string]any)
	responseData["check_result"] = checkResult
	responseData["vote_gate"] = voteGate

	ctx.JSON(http.StatusOK, api.Success(responseData))
}
