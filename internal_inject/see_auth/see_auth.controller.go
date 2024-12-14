package seeauth_inject

import (
	"net/http"

	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"gorm.io/gorm"

	seeauth "github.com/Taoist-Labs/see-auth-go"
)

type SeeAuthController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	// see auth service
	SeeAuthSrv *SeeAuthService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var seeAuth SeeAuthController

	err := inject.Populate(&seeAuth, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var seeAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		seeAuthGroup = fatherGroup.Group("/seeauth")
	} else {
		seeAuthGroup = seeAuth.Gin.Group("/seeauth")
	}

	// no auth
	seeAuthGroup.GET("/nonce/:wallet", seeAuth.SeeAuthNonce)
	seeAuthGroup.POST("/login", seeAuth.LoginWithSeeAuth)
	seeAuthGroup.POST("/seeauth_3rd_test", seeAuth.SeeAuthTestApi)
}

func (c *SeeAuthController) SeeAuthNonce(ctx *gin.Context) {
	wallet := ctx.Param("wallet")

	httpCode, reply := c.SeeAuthSrv.SeeAuthNonce(ctx, wallet)

	ctx.JSON(httpCode, reply)
}

func (c *SeeAuthController) LoginWithSeeAuth(ctx *gin.Context) {
	req := seeauth.SeeLogin{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	httpCode, reply := c.SeeAuthSrv.LoginWithSeeAuth(ctx, &req)
	ctx.JSON(httpCode, reply)
}

func (c *SeeAuthController) SeeAuthTestApi(ctx *gin.Context) {
	req := seeauth.SeeAuth{}
	err := ctx.BindJSON(&req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	wallet, err := seeauth.SeeDAOAuth("0x0000000000000000000000000000000000000000", &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, struct {
		Wallet string `json:"wallet"`
		Token  string `json:"token"`
	}{
		Wallet: wallet,
		Token:  "test.jwt.token",
	})
}
