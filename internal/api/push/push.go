package push

import (
	"net/http"

	"github.com/rs/zerolog/log"

	"github.com/gin-gonic/gin"
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
// `POST /v1/push`
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
		//PushDate:      req.PushDate,
		//Status: 0,
	}
	err = model.PushModel.CreateOrUpdate(db, &push)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// send push
	pushSDK := api.ForContextOnlyPush(ctx)
	go func(pushSDK *sdk.Push, title, body map[string]string, jumpURL string) {
		data := api.GenerateCustomNotificationParams(jumpURL)
		err := pushSDK.PushAll(title, body, data)
		if err != nil {
			log.Error().Msgf("push to all failed: %s", err)
		}
	}(pushSDK, map[string]string{api.LanguageZH: req.Title, api.LanguageEN: req.Title}, map[string]string{api.LanguageZH: req.Content, api.LanguageEN: req.Content}, req.JumpURL)

	ctx.JSON(http.StatusOK, api.Success(nil))
}

// List
// `GET /push?status=1&page=1&size=10&sort_field=created_at&sort_order=desc`
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
