package permission

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
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
//	@summary	Grant role to user
//	@tags		Permission
//	@accept		json
//	@produce	json
//	@param		grants	body		GrantRoleReq	true	"request json body"
//	@success	200		{object}	api.Reply
//	@router		/permission/grant_role [post]
func GrantRole(ctx *gin.Context) {
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

type RevokeRoleReq struct {
	Revokes []role `json:"revokes"`
}

// RevokeRole role from user
//
//	@summary	Revoke role from user
//	@tags		Permission
//	@accept		json
//	@produce	json
//	@param		revokes	body		RevokeRoleReq	true	"request json body"
//	@success	200		{object}	api.Reply
//	@router		/permission/revoke_role [post]
func RevokeRole(ctx *gin.Context) {
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
