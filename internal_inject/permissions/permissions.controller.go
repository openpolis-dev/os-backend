package permissions_inject

import (
	"errors"
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type PermissionsController struct {
	// inject
	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	PermissionsService *PermissionsService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var permissions PermissionsController

	err := inject.Populate(&permissions, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var permissionsAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		permissionsAuthGroup = fatherGroup.Group("/", middleware.AuthRequired).Group("/permission")
	} else {
		permissionsAuthGroup = permissions.Gin.Group("/", middleware.AuthRequired).Group("/permission")
	}

	// auth
	permissionsAuthGroup.POST("/grant_role", permissions.GrantRole)
	permissionsAuthGroup.POST("/revoke_role", permissions.RevokeRole)
}

func (c *PermissionsController) GrantRole(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)
	ok, err := enforcer.HasRoleForUser(formattedWallet, internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	req := GrantRoleReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	policies := lo.Map(req.Grants, func(r role, _ int) []string {
		// g, 0xc13..1283 event_manager
		return []string{common.FormatUserWallet(r.Wallet), r.Role}
	})
	_, err = enforcer.AddGroupingPolicies(policies)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("grant role error")))
		return
	}
	err = enforcer.SavePolicy()
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("grant role error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

func (c *PermissionsController) RevokeRole(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.RoleHall, "access")
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	req := RevokeRoleReq{}
	err = ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	policies := lo.Map(req.Revokes, func(r role, _ int) []string {
		// g, 0xc13..1283 event_manager
		return []string{common.FormatUserWallet(r.Wallet), r.Role}
	})
	_, err = enforcer.RemoveGroupingPolicies(policies)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("revoke role error")))
		return
	}
	err = enforcer.SavePolicy()
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("revoke role error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
