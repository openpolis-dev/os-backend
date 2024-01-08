package push

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type CreateReq struct {
	// TODO support multi language
	//Title   map[string]string `json:"title"`   // [zh]你好,[en]Hello
	//Content map[string]string `json:"content"` // [zh]你好,[en]Hello
	Title   string `json:"title"`
	Content string `json:"content"`

	JumpURL string `json:"jump_url"`

	//PushDate time.Time `json:"push_date"`
}

// Create a push
//
//	@summary	Create a push
//	@tags		Push
//	@accept		json
//	@produce	json
//	@param		push	body		CreateReq	true	"request json body"
//	@success	200		{object}	api.Reply
//	@router		/push [post]
func Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjPush, internal.ActCreatePush)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjPush, internal.ActCreatePush)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	// save push
	push := model.Push{
		CreatorWallet: user.Wallet,
		Title:         req.Title,
		Content:       req.Content,
		JumpURL:       req.JumpURL,
		PushDate:      time.Now(),
		//Status: 0,
		CreatedAt: time.Now().In(internal.ProjectTimezone),
		CreateTs:  model.GetCurrentUtcEpochSecond(),
		UpdatedAt: time.Now().In(internal.ProjectTimezone),
		UpdateTs:  model.GetCurrentUtcEpochSecond(),
	}
	err = model.PushModel.CreateOrUpdate(db, &push)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create push error")))
		return
	}

	// send push
	pushSDK := api.ForContextOnlyPush(ctx)
	go func(pushSDK []sdk.Pusher, title, body map[string]string, jumpURL string) {
		data := sdk.GenerateCustomNotificationParams(jumpURL)
		for _, p := range pushSDK {
			err := p.PushAll(title, body, data)
			if err != nil {
				log.Error().Msgf("push to all failed: %s", err)
			}
		}
	}(pushSDK, map[string]string{sdk.LanguageZH: req.Title, sdk.LanguageEN: req.Title}, map[string]string{sdk.LanguageZH: req.Content, sdk.LanguageEN: req.Content}, req.JumpURL)

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// List push
//
//	`GET /push?status=1&page=1&size=10&sort_field=created_at&sort_order=desc`
//
//	@summary	list push
//	@tags		Push
//	@accept		json
//	@produce	json
//	@param		status		query		int		false	"status"
//	@param		page		query		int		false	"page"
//	@param		size		query		int		false	"size"
//	@param		sort_field	query		string	false	"sort_field"
//	@param		sort_order	query		string	false	"sort_order"
//	@success	200			{object}	api.Reply{data=api.ListReplyData{rows=model.Push}}
//	@router		/push [get]
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	//status := ctx.Query("status") // TODO handle status query param
	page := api.ParseAndConvertPageParam(ctx)

	pushes, total, err := model.PushModel.List(db, nil, page)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list push error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  pushes,
	}))
}
