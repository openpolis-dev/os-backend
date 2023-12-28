package proposal

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

// AddComment attach comment to specified proposal
//
//	@summary	Attach comment to specified proposal or comment
//	@router		/proposals/add_comment/:id [post]
//	@Param			id		path		string		true	"id of the proposal"
//	@Param			request	body		AddCommentData	true	"Comment data"
//	@success	200	{object}	api.Reply{data=nil}
func AddComment(ctx *gin.Context) {
	addComment := AddCommentData{}
	err := ctx.BindJSON(&addComment)
	if err != nil {
		log.Error().Msgf("bind json error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	if addComment.MetaforoAccessToken == "" {
		log.Error().Msgf("missing metaforo access token")
		sdk.LogUserSideError(ctx, errors.New("missing metaforo access token"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo access token")))
		return
	}

	db := api.ForContextOnlyDB(ctx)
	proposalIdStr := ctx.Param("id")
	proposalRcd, proposalDetailRecord, err := GetMetaforoProposalByInternalId(db, proposalIdStr)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Warn().Msgf("proposal %s not found", proposalIdStr)
			ctx.JSON(http.StatusNotFound, nil)
			return
		} else {
			log.Error().Msgf("get proposal id %s error: %+v", proposalIdStr, err)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("get proposal error")))
			return
		}
	}

	replyId := proposalDetailRecord.Thread.FirstPostId
	if addComment.ReplyId != 0 {
		replyId = addComment.ReplyId
	}

	err = metaforo.AddComment(
		addComment.MetaforoAccessToken,
		internal.MetaforoGroupName,
		proposalRcd.GetMetaforoThreadId(),
		addComment.Content,
		fmt.Sprintf("%d", replyId),
	)
	if err != nil {
		log.Error().Msgf("add comment to proposal %s error: %+v", proposalIdStr, err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("add comment error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func EditComment(ctx *gin.Context) {
	editComment := EditCommentData{}
	err := ctx.BindJSON(&editComment)
	if err != nil {
		log.Error().Msgf("bind json error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	if editComment.MetaforoAccessToken == "" {
		log.Error().Msgf("missing metaforo access token")
		sdk.LogUserSideError(ctx, errors.New("missing metaforo access token"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo access token")))
		return
	}

	// TODO: Check whether the comment modified is reject comment, if yes update the db content

	err = metaforo.EditComment(
		editComment.MetaforoAccessToken,
		internal.MetaforoGroupName,
		fmt.Sprintf("%d", editComment.MetaforoCommentId),
		editComment.Content,
	)
	if err != nil {
		log.Error().Msgf("edit comment error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("add comment error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
