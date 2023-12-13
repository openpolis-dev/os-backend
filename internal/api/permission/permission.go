package permission

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

type GrantRoleReq struct {
	Grants []role `json:"grants"`
}

type role struct {
	Wallet string `json:"wallet"`
	Role   string `json:"role"`
}

// GrantRole role to user
//
//	@Summary	Grant role to user
//	@Tags		Permission
//	@Accept		json
//	@Produce	json
//	@Param		grants	body		GrantRoleReq	true	"request json body"
//	@Success	200		{object}	api.Reply
//	@Router		/permission/grant_role [post]
func GrantRole(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	formattedWallet := common.FormatUserWallet(user.Wallet)
	ok, err := enforcer.HasRoleForUser(formattedWallet, api.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	err = enforcer.SavePolicy()
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}

type RevokeRoleReq struct {
	Revokes []role `json:"revokes"`
}

// RevokeRole role from user
//
//	@Summary	Revoke role from user
//	@Tags		Permission
//	@Accept		json
//	@Produce	json
//	@Param		revokes	body		RevokeRoleReq	true	"request json body"
//	@Success	200		{object}	api.Reply
//	@Router		/permission/revoke_role [post]
func RevokeRole(ctx *gin.Context) {
	user, enforcer, _, _ := api.ForContext(ctx)
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), api.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	err = enforcer.SavePolicy()
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
