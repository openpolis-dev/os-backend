package proposal

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

// AddComment attach comment to specified proposal
//
//	@summary	Attach comment to specified proposal or comment
//	@router		/proposals/add_comment/:id [post]
//	@Param		id		path		string					true	"id of the proposal"
//	@Param		request	body		proposal.AddCommentData	true	"Comment data"
//	@success	200		{object}	api.Reply{data=nil}
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
	if addComment.ReplyToMetaforoCommentId != 0 {
		replyId = addComment.ReplyToMetaforoCommentId
	}

	_, err = metaforo.AddComment(
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

// EditComment handles the editing of a comment.
//
// ctx: The gin context.
// Returns: None.
//
//	@summary	edit comment and save back to metaforo. If the comment is reject_comment, the data saved in db will also be saved
//	@router		/proposals/edit_comment/:id [post]
//	@tags		proposals
//	@param		id		query		string						true	"id of the proposal"
//	@param		request	body		proposal.EditCommentData	true	"Comment data"
//	@success	200		{object}	api.Reply{data=nil}
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

	// Check whether the comment modified is reject comment, if yes update the db content
	db := api.ForContextOnlyDB(ctx)
	var rejectComment model.ProposalComment
	err = db.Model(&model.ProposalComment{}).
		Where("metaforo_comment_id = ? AND is_reject_comment = ?", fmt.Sprintf("%d", editComment.MetaforoCommentId), true).
		First(&rejectComment).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error().Msgf("query reject comment error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("edit comment error")))
		return
	}

	err = metaforo.EditComment(
		editComment.MetaforoAccessToken,
		internal.MetaforoGroupName,
		fmt.Sprintf("%d", editComment.MetaforoCommentId),
		editComment.Content,
	)
	if err != nil {
		log.Error().Msgf("edit comment error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("edit comment error")))
		return
	}

	if rejectComment.ID != 0 {
		rejectComment.Content = editComment.Content
		err = db.Save(&rejectComment).Error
		if err != nil {
			log.Error().Msgf("update reject comment error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("edit comment error")))
			return
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// DeleteComment deletes a comment from the database and the Metaforo API.
//
// It takes a gin.Context object as a parameter.
// Returns nothing.
//
//	@summary	delete comment from metaforo. The reject reason comment can't be deleted
//	@router		/proposals/delete_comment/:id [post]
//	@tags		proposals
//	@param		id		query		string						true	"id of the proposal"
//	@param		request	body		proposal.DeleteCommentData	true	"Delete Comment request data"
//	@success	200		{object}	api.Reply{data=nil}
func DeleteComment(ctx *gin.Context) {
	deleteComment := DeleteCommentData{}
	err := ctx.BindJSON(&deleteComment)
	if err != nil {
		log.Error().Msgf("bind json error: %+v", err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}
	if deleteComment.MetaforoAccessToken == "" {
		log.Error().Msgf("missing metaforo access token")
		sdk.LogUserSideError(ctx, errors.New("missing metaforo access token"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo access token")))
		return
	}

	user, _, db, _ := api.ForContext(ctx)
	var rejectComment model.ProposalComment
	err = db.Model(&model.ProposalComment{}).
		Where("metaforo_comment_id = ? AND is_reject_comment = ?", fmt.Sprintf("%d", deleteComment.MetaforoCommentId), true).
		First(&rejectComment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Reject comment not found, the comment can be deleted
			err = metaforo.DeleteComment(
				deleteComment.MetaforoAccessToken,
				internal.MetaforoGroupName,
				fmt.Sprintf("%d", deleteComment.MetaforoCommentId),
			)
			if err != nil {
				log.Error().Msgf("delete comment error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("delete comment error")))
				return
			}
			ctx.JSON(http.StatusOK, api.Success(nil))
		} else {
			log.Error().Msgf("query reject comment error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("delete comment error")))
			return
		}
	} else {
		// Reject comment found, the comment can not be deleted
		log.Error().Msgf("try to delete reject comment")
		sdk.LogUserSideError(ctx, fmt.Errorf("user %s try to delete reject comment with comment metaforo id %s", user.Wallet, rejectComment.MetaforoCommentId))
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("reject comment can't be deleted")))
		return
	}
}
