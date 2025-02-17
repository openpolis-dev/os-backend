package admin

import (
	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api/cron_jobs"
	"github.com/theseed-labs/os-backend/internal/api/data_srv"
	"github.com/theseed-labs/os-backend/internal/api/user"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"

	proposal_inject "github.com/theseed-labs/os-backend/internal_inject/proposal"

	"gorm.io/gorm"
)

type AdminController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	AdminService *AdminService `inject:""`

	ProposalSrv *proposal_inject.ProposalService `inject:""`
}

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var adminController AdminController

	err := inject.Populate(&adminController, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var adminAuthGroup *gin.RouterGroup

	if fatherGroup != nil {
		adminAuthGroup = fatherGroup.Group("/", middleware.AdminPermissionRequired).Group("/admin")
	} else {
		adminAuthGroup = adminController.Gin.Group("/", middleware.AdminPermissionRequired).Group("/admin")
	}

	proposalTmplAdminRouter := adminAuthGroup.Group("/proposal_tmpl")
	proposalTmplAdminRouter.POST("/update", adminController.ProposalSrv.UpdateTemplate)

	userAdminRouter := adminAuthGroup.Group("/user")
	userAdminRouter.POST("/check_vote_permission", user.CheckVotePermission)

	// Schedule jobs routers
	jobsRouter := adminAuthGroup.Group("/jobs")
	jobsRouter.GET("/list", cron_jobs.List)

	// TODO: New tasks required
	//metaforoRouter := adminGroup.Group("/mf")
	//metaforoRouter.POST("/update_mf_admin_token", TBD)
	//metaforoRouter.POST("/sync_perm_group", TBD)

	// Fetch single proposal user vote record
	adminAuthGroup.POST("/update_proposal_vote_record/:proposal_id", data_srv.FetchSingleProposalUserVoteRecord)

}
