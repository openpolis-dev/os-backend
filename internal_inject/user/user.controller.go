package user_inject

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/sdk/metaforo"
	"gorm.io/gorm"

	eth_common "github.com/ethereum/go-ethereum/common"
)

type UserController struct {
	// inject

	Gin *gin.Engine

	Db *gorm.DB

	// user service
	UserSrv *UserService
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var user UserController

	err := inject.Populate(&user, &UserService{}, g.Gin, g.Db)

	if err != nil {
		panic(err)
	}

	var userGroup *gin.RouterGroup
	var userAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		userGroup = fatherGroup.Group("/user")
		userAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/user")
	} else {
		userGroup = user.Gin.Group("/user")
		userAuthGroup = user.Gin.Group("/", middleware.AuthRequired).Group("/user")
	}

	// no auth
	userGroup.POST("/refresh_nonce", user.RefreshNonce)
	userGroup.POST("/login", user.Login)
	userGroup.GET("/users", user.Users)
	userGroup.GET("/casbin", user.GetFrountedPermission)
	userGroup.GET("/metaforo_activities", user.MetaforoActivities)

	// auth
	userAuthGroup.GET("/me", user.Detail)
	userAuthGroup.PUT("/me", user.Update)
	userAuthGroup.POST("/logout", user.Logout)
	userAuthGroup.POST("/join_metaforo_group", user.JoinMetaforoGroup)
	userAuthGroup.POST("/leave_metaforo_group", user.LeaveMetaforoGroup)
	userAuthGroup.POST("/prepare_metaforo", user.PrepareMetaforoData)
	userAuthGroup.GET("/level", user.UserLvl)
}

func (ctrl *UserController) RefreshNonce(ctx *gin.Context) {
	req := RefreshNonceReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	userNonce, err := model.UserNonceModel.Detail(ctrl.Db, common.FormatUserWallet(req.Wallet))
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("nonce not found")))
		return
	}

	err = ctrl.UserSrv.RefreshNonce(ctx, req.Wallet, userNonce)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update nonce error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&RefreshNonceReply{Nonce: userNonce.Nonce}))
}

func (ctrl *UserController) Login(ctx *gin.Context) {
	req := LoginReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := ctrl.UserSrv.Login(ctx, &req)

	ctx.JSON(httpCode, reply)
}

func (ctrl *UserController) Users(ctx *gin.Context) {
	wallets := ctx.QueryArray("wallets")

	sppClient := sdk.GetSppClient()

	users, err := model.UserModel.List(ctrl.Db, wallets)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query users error")))
		return
	}

	if len(users) != len(wallets) {
		existUsers := lo.Map[*model.User](users, func(item *model.User, _ int) string {
			return item.Wallet
		})
		for _, w := range wallets {
			if !lo.Contains(existUsers, w) {
				users = append(users, &model.User{Wallet: w})
			}
		}
	}

	var rslt []UserModelWithSomeSeepassData

	// TODO: Query SeePASS to get user SBT and SEED info
	for _, user := range users {
		seepassResp, err := api.GetCachedSeepassData(sppClient, user.Wallet, false)
		user.Wallet = common.ToFrontendWallet(user.Wallet)
		if err != nil {
			log.Warn().Msgf("query seepass data error, wallet: %s, error: %+v", user.Wallet, err)
		}
		if seepassResp != nil {
			rslt = append(rslt, UserModelWithSomeSeepassData{
				*user,
				seepassResp,
			})
		} else {
			rslt = append(rslt, UserModelWithSomeSeepassData{
				*user,
				nil,
			})
		}

	}

	ctx.JSON(http.StatusOK, api.Success(rslt))
}

func (ctrl *UserController) GetFrountedPermission(ctx *gin.Context) {
	enforcer := api.ForContextOnlyEnforcer(ctx)

	//sub, _ := ctx.GetQuery("casbin_subject")
	//data, err := casbin.CasbinJsGetPermissionForUser(enforcer, strings.ToLower(sub))
	// --> casbin.CasbinJsGetPermissionForUser()'s logic is not correct, should use the following logic instead:
	eModel := enforcer.GetModel()
	m := map[string]interface{}{}
	m["m"] = eModel.ToText()
	policies := make([][]string, 0)
	for ptype := range eModel["p"] {
		policy := eModel.GetPolicy("p", ptype)
		for i := range policy {
			policies = append(policies, append([]string{ptype}, policy[i]...))
		}
	}
	for ptype := range eModel["g"] {
		role := eModel.GetPolicy("g", ptype)
		for i := range role {
			// Copy role to a temporary slice to avoid changing the original policies
			tmpRole := make([]string, len(role[i]))
			copy(tmpRole, role[i])
			if eth_common.IsHexAddress(tmpRole[0]) {
				tmpRole[0] = common.ToFrontendWallet(tmpRole[0])
			}
			policies = append(policies, append([]string{ptype}, tmpRole...))
		}
	}
	m["p"] = policies
	result := bytes.NewBuffer([]byte{})
	encoder := json.NewEncoder(result)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(m)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query frontend permission error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(result.String()))
}

func (ctrl *UserController) MetaforoActivities(ctx *gin.Context) {
	var err error

	userId := ctx.Query("userId")
	size := ctx.Query("size")
	session := ctx.Query("session")

	if userId == "" {
		log.Error().Msgf("missing metaforo user id")
		sdk.LogUserSideError(ctx, errors.New("missing metaforo user id"))
		ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("missing metaforo user id")))
		return
	}

	sizeVal := 0
	if size != "" {
		sizeVal, err = strconv.Atoi(size)
		if err != nil {
			log.Error().Msgf("parse size error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("parse size error")))
			return
		}
	}

	if sizeVal == 0 {
		sizeVal = internal.DefaultPageSize
	}

	activities, newSession, err := metaforo.UserActivities(userId, "all", fmt.Sprintf("%d", sizeVal), session)
	if err != nil {
		log.Error().Msgf("get metaforo activities error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get activities error")))
		return
	}

	// Save activity records to
	userIdActivityMap := make(map[int]*metaforo.UserActivity)
	var metaforoUserIds []int
	for _, activity := range activities {
		userIdActivityMap[activity.UserId] = activity
		metaforoUserIds = append(metaforoUserIds, activity.UserId)
		metaforoUserIds = append(metaforoUserIds, activity.ThreadPosterId)
	}

	db, cfg := api.ForContextDBAndConfig(ctx)
	// Get proposalIds from metaforo thread id in activities
	proposalRecordIds := lo.Map(activities, func(r *metaforo.UserActivity, _ int) string {
		return model.BuildProposalRecordIdFromMetaforoThreadId(r.ThreadId)
	})

	var touchedProposals []*model.Proposal
	err = db.Model(model.Proposal{}).Where("proposal_record_id in ?", proposalRecordIds).Find(&touchedProposals).Error
	if err != nil {
		log.Error().Msgf("get proposal error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get proposal error")))
		return
	}

	metaforoThreadIdToProposalIdMapping := make(map[int]uint)
	for _, p := range touchedProposals {
		metaforoThreadIdToProposalIdMapping[p.GetMetaforoThreadId()] = p.ID
	}

	userRecords, err := proposal.GetOsUserFromMetaforoUserId(db, metaforoUserIds)
	if err != nil {
		log.Error().Msgf("get os user error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get os user error")))
		return
	}

	metaforoUidUserMapping := lo.KeyBy(userRecords, func(r *proposal.JointMetaforoAndOsUser) int {
		return r.MetaforoUserID
	})

	rsltRcds := make([]*MetaforoActivityRecord, 0)
	for idx := range activities {
		r := activities[idx]
		if r.GroupId != cfg.MetaforoData.GroupID {
			continue
		}

		action := "unknown"
		if r.LikeId != 0 {
			// Do not handle like action in OS side
			action = "like"
			continue
		} else if r.FirstPostId == r.Id {
			action = "create"
		} else {
			action = "comment"
		}

		selfWallet := ""
		if selfUserRcd, found := metaforoUidUserMapping[r.UserId]; found {
			selfWallet = selfUserRcd.Wallet
		}

		replyToWallet := ""
		if replyToUserRcd, found := metaforoUidUserMapping[r.ThreadPosterId]; found {
			replyToWallet = replyToUserRcd.Wallet
		}

		rsltRcds = append(rsltRcds, &MetaforoActivityRecord{
			Wallet:           selfWallet,
			ReplyToWallet:    replyToWallet,
			ProposalID:       metaforoThreadIdToProposalIdMapping[r.ThreadId],
			Action:           action,
			ThreadTitle:      r.ThreadTitle,
			MetaforoThreadId: r.ThreadId,
			ActionTs:         r.CreatedAt.UTC().Unix(),
		})
	}

	ctx.JSON(http.StatusOK, api.Success(&MetaforoActivityResponse{
		Session: newSession,
		Records: rsltRcds,
	}))
}

func (ctrl *UserController) Detail(ctx *gin.Context) {
	user, _ := api.ForContextUserAndDB(ctx)

	sppClient := sdk.GetSppClient()
	seepassResp, err := api.GetCachedSeepassData(sppClient, user.Wallet, false)
	if err == nil {
		ctx.JSON(http.StatusOK, api.Success(seepassResp))
		return
	}

	log.Warn().Msgf("query seepass data error, wallet: %s, error: %+v", user.Wallet, err)

	u, err := model.UserModel.Detail(ctrl.Db, user.Wallet)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
		return
	}
	if u == nil {
		u = &model.User{Wallet: user.Wallet}
	}

	seepassResp = &sdk.SeepassResponse{
		Roles:    make([]string, 0),
		Wallet:   user.Wallet,
		Nickname: u.Name,
		Avatar:   u.Avatar,
		Bio:      u.Bio,
		Email:    u.Email,
	}

	seepassResp.Scr.Amount = "0"

	// TODO: Move the hardcoded data to some const data or configuration service
	seepassResp.Level.CurrentLv = "0"
	seepassResp.Level.NextLv = "1"
	seepassResp.Level.ScrToNextLv = "5000"
	seepassResp.Level.UpgradePercent = "0"

	ctx.JSON(http.StatusOK, api.Success(seepassResp))
}

func (ctrl *UserController) Update(ctx *gin.Context) {
	req := UpdateReq{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	user, _ := api.ForContextUserAndDB(ctx)

	httpCode, reply := ctrl.UserSrv.Update(ctx, user, &req)

	ctx.JSON(httpCode, reply)
}

func (ctrl *UserController) Logout(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (ctrl *UserController) JoinMetaforoGroup(ctx *gin.Context) {
	var req JoinOrLeaveGroupReq
	err := ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("parse join group request error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	err = metaforo.JoinGroup(req.MetaforoAccessToken, req.GroupName)
	if err != nil {
		log.Error().Msgf("join group error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("join group error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (ctrl *UserController) LeaveMetaforoGroup(ctx *gin.Context) {
	var req JoinOrLeaveGroupReq
	err := ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("parse leave group request error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	err = metaforo.LeaveGroup(req.MetaforoAccessToken, req.GroupName)
	if err != nil {
		log.Error().Msgf("leave group error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("leave group error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))

}

func (ctrl *UserController) PrepareMetaforoData(ctx *gin.Context) {
	// The struct type LoginResponse here means this is the response from metaforo for user login.
	// The data is passed from frontend directly to the backend so the struct name is not Response
	var req metaforo.LoginResponse
	err := ctx.BindJSON(&req)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		log.Error().Msgf("parse prepare metaforo user request error: %+v", err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	userGroupsBytes, err := json.Marshal(req.User.GroupProfiles)
	if err != nil {
		log.Error().Msgf("serialize metaforo user group info error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("processing user info error")))
		return
	}

	user, _, db, cfg := api.ForContext(ctx)
	var metaforoUser model.MetaforoUser
	err = db.Model(model.MetaforoUser{}).Where(model.MetaforoUser{
		MetaforoUserId: req.User.Id,
		UserWallet:     common.FormatUserWallet(user.Wallet),
	}).Attrs(model.MetaforoUser{
		Groups: userGroupsBytes,
	}).FirstOrCreate(&metaforoUser).Error

	if err != nil {
		log.Error().Msgf("update metaforo user error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update user info error")))
		return
	}

	err = metaforo.JoinGroup(req.ApiToken, cfg.MetaforoData.GroupName)
	if err != nil {
		log.Error().Msgf("join group error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("join group error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (ctrl *UserController) UserLvl(ctx *gin.Context) {
	user, _ := api.ForContextUserAndDB(ctx)

	sppClient := sdk.GetSppClient()

	seepassResp, err := api.GetCachedSeepassData(sppClient, user.Wallet, false)
	if err == nil {
		rslt := UserLvlRes{}
		rslt.CurrentLv = seepassResp.Level.CurrentLv
		ctx.JSON(http.StatusOK, api.Success(rslt))
		return
	}

	log.Warn().Msgf("query seepass data error, wallet: %s, error: %+v", user.Wallet, err)

	_, err = model.UserModel.Detail(ctrl.Db, user.Wallet)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("user not found")))
		return
	}

	rslt := UserLvlRes{}
	rslt.CurrentLv = "0"

	ctx.JSON(http.StatusOK, api.Success(rslt))
}
