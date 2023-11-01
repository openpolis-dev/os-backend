package push

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/api"
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
// @Summary Create a push
// @Tags Push
// @Accept json
// @Produce json
// @Param push body CreateReq true "request json body"
// @Success 200 {object} Reply
// @Router /v1/push [post]
func Create(ctx *gin.Context) {
	req := CreateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, api.ObjPush, api.ActCreatePush)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
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
	}
	err = model.PushModel.CreateOrUpdate(db, &push)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// send push
	pushSDK := api.ForContextOnlyPush(ctx)
	go func(pushSDK sdk.Pusher, title, body map[string]string, jumpURL string) {
		data := sdk.GenerateCustomNotificationParams(jumpURL)
		err := pushSDK.PushAll(title, body, data)
		if err != nil {
			log.Error().Msgf("push to all failed: %s", err)
		}
	}(pushSDK, map[string]string{sdk.LanguageZH: req.Title, sdk.LanguageEN: req.Title}, map[string]string{sdk.LanguageZH: req.Content, sdk.LanguageEN: req.Content}, req.JumpURL)

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// List push
//
//	`GET /push?status=1&page=1&size=10&sort_field=created_at&sort_order=desc`
//
// @Summary List push
// @Tags Push
// @Accept json
// @Produce json
// @Param status query int false "status"
// @Param page query int false "page"
// @Param size query int false "size"
// @Param sort_field query string false "sort_field"
// @Param sort_order query string false "sort_order"
// @Success 200 {object} ListReplyData
// @Router /v1/push [get]
func List(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	//status := ctx.Query("status") // TODO handle status query param
	page := api.ParseAndConvertPageParam(ctx)

	pushes, total, err := model.PushModel.List(db, nil, page)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: total,
		Rows:  pushes,
	}))
}
