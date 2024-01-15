package proposal

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"
)

// AddComment attach comment to specified proposal
//
//	@summary	Attach comment to specified proposal or comment
//	@tags		Proposal
//	@router		/proposals/add_comment/:id [post]
//	@param		id		path		string					true	"id of the proposal"
//	@param		request	body		proposal.AddCommentData	true	"Comment data"
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

	user, _, db, cfg := api.ForContext(ctx)
	proposalIdStr := ctx.Param("id")
	proposalRcd, proposalMetaforoData, err := GetMetaforoProposalByInternalId(db, proposalIdStr, cfg.MetaforoData.GroupName)
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

	replyId := proposalMetaforoData.Thread.FirstPostId
	if addComment.ReplyToMetaforoCommentId != 0 {
		replyId = addComment.ReplyToMetaforoCommentId
	}

	metaforoCommentData, err := metaforo.AddComment(
		addComment.MetaforoAccessToken,
		cfg.MetaforoData.GroupName,
		proposalRcd.GetMetaforoThreadId(),
		addComment.Content,
		fmt.Sprintf("%d", replyId),
		addComment.EditorType,
	)
	if err != nil {
		log.Error().Msgf("add comment to proposal %s error: %+v", proposalIdStr, err)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("add comment error")))
		return
	}

	var parentComment model.ProposalComment
	if addComment.ReplyToMetaforoCommentId != 0 {
		err := db.Model(model.ProposalComment{}).Where("metaforo_comment_id = ?", fmt.Sprintf("%d", addComment.ReplyToMetaforoCommentId)).First(&parentComment).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				log.Error().Msgf("get parent comment error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get parent comment error")))
				return
			} else {
				// This branch indicates that the parent comment has been added to metaforo but haven't saved in DB
				// Check whether it can be synced from some API calls
			}
		}
	}

	proposalComment := model.ProposalComment{
		CreateTs:          time.Now().UTC().Unix(),
		ParentID:          parentComment.ID,
		ProposalID:        proposalRcd.ID,
		ProposalRecordID:  proposalRcd.ProposalRecordId,
		Content:           addComment.Content,
		MetaforoCommentId: metaforoCommentData.Id,
		IsRejectComment:   false, // Reject comment is added in other API endpoint
		AuthorWallet:      common.FormatUserWallet(user.Wallet),
	}

	err = db.Create(&proposalComment).Error
	if err != nil {
		log.Error().Msgf("create proposal comment error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create proposal comment error")))
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
//	@tags		Proposal
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
	db, cfg := api.ForContextDBAndConfig(ctx)
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
		cfg.MetaforoData.GroupName,
		fmt.Sprintf("%d", editComment.MetaforoCommentId),
		editComment.Content,
		editComment.EditorType,
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
//	@tags		Proposal
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

	user, _, db, cfg := api.ForContext(ctx)
	var rejectComment model.ProposalComment
	err = db.Model(&model.ProposalComment{}).
		Where("metaforo_comment_id = ? AND is_reject_comment = ?", fmt.Sprintf("%d", deleteComment.MetaforoCommentId), true).
		First(&rejectComment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Reject comment not found, the comment can be deleted
			err = metaforo.DeleteComment(
				deleteComment.MetaforoAccessToken,
				cfg.MetaforoData.GroupName,
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
		sdk.LogUserSideError(ctx, fmt.Errorf("user %s try to delete reject comment with comment metaforo id %d", user.Wallet, rejectComment.MetaforoCommentId))
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("reject comment can't be deleted")))
		return
	}
}
