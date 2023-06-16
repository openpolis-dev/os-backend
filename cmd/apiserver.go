package main

import (
	"flag"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/application"
	"github.com/theseed-labs/os-backend/internal/api/project"
	"github.com/theseed-labs/os-backend/internal/api/user"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/storage"
)

func main() {
	// read config data
	cfgPath := flag.String("config", "config.yml", "Configuration file path, should be yaml or json format")
	casbinModelConfPath := flag.String("casbin-model", "rbac_model.conf", "casbin model conf file path, should be conf format")
	flag.Parse()
	cfg := config.LoadConfig(*cfgPath)

	// setup permission system
	adapter, err := gormadapter.NewAdapter(cfg.Casbin.DriverName, cfg.DataSource.Dsn, true)
	if err != nil {
		panic(err)
	}
	enforcer, err := casbin.NewEnforcer(*casbinModelConfPath, adapter)
	if err != nil {
		panic(err)
	}
	// add default policies
	defaultPolicies := [][]string{
		{api.RoleHall, "*", "*"}, // `p, hall, *, *` hall can do anything
	}
	_, err = enforcer.AddPolicies(defaultPolicies)
	if err != nil {
		panic(err)
	}
	// add default users
	groupPolicies := lo.Map[string, []string](cfg.Casbin.SuperUsers, func(user string, _ int) []string {
		return []string{user, api.RoleHall}
	})
	_, err = enforcer.AddGroupingPolicies(groupPolicies) // add default hall wallets
	if err != nil {
		panic(err)
	}
	// save modify to database
	err = enforcer.SavePolicy()
	if err != nil {
		panic(err)
	}
	// load policy from database
	err = enforcer.LoadPolicy()
	if err != nil {
		panic(err)
	}

	// setup database
	storage.InitGormDB(cfg.DataSource.Dsn)
	db := storage.GetGormDB()

	r := gin.Default()

	// setup cors refer: https://github.com/gin-contrib/cors
	corsCfg := cors.DefaultConfig()
	corsCfg.AllowAllOrigins = true
	corsCfg.AllowHeaders = []string{"Origin", "Accept", "Content-Type", "Authorization"}
	r.Use(cors.New(corsCfg))

	// setup basic middleware for database connection and config data
	r.Use(func(ctx *gin.Context) {
		ctx.Set(middleware.DBKey, db)
		ctx.Set(middleware.CfgKey, cfg)
		ctx.Set(middleware.EnforcerKey, enforcer)

		// <-- before
		ctx.Next()
		// --> after
	})

	// v1
	v1 := r.Group("/v1")
	// --> no auth required
	{
		// user routers
		userGroup := v1.Group("/user")
		userGroup.POST("/login", user.Login)
		userGroup.GET("/users", user.Users)

		// project routers
		projGroup := v1.Group("/projects")
		projGroup.GET("/", project.List)
		projGroup.GET("/:id", project.Detail)

		// foo routers
	}
	// --> auth required
	{
		authorizedGroup := v1.Group("/", middleware.AuthRequired)

		// user routers
		userGroup := authorizedGroup.Group("/user")
		userGroup.GET("/me", user.Detail)
		userGroup.PUT("/me", user.Update)
		userGroup.POST("/logout", user.Logout)

		// project routers
		projGroup := authorizedGroup.Group("/projects")
		projGroup.POST("/", project.Create)
		projGroup.PUT("/:id", project.Update)
		projGroup.POST("/:id/close", project.Close)
		projGroup.GET("/my", project.MyProjects)
		projGroup.POST("/:id/update_sponsors", project.UpdateSponsors)
		projGroup.POST("/:id/update_members", project.UpdateMembers)
		projGroup.POST("/:id/update_budget", project.UpdateBudget)
		projGroup.POST("/:id/add_related_proposal/:proposal_id", project.AddRelatedProposal)

		// application routers
		applicationGroup := authorizedGroup.Group("/applications")
		applicationGroup.GET("/", application.List)
		applicationGroup.POST("/", application.Create)
		applicationGroup.POST("/export", application.Export)
		applicationGroup.POST("/approve", application.BatchApprove)
		applicationGroup.POST("/reject", application.BatchReject)
		applicationGroup.POST("/complete", application.BatchComplete)
		applicationGroup.GET("/:id", application.Detail)
		applicationGroup.POST("/:id/approve", application.Approve)
		applicationGroup.POST("/:id/reject", application.Reject)
		applicationGroup.POST("/:id/complete", application.Complete)
		applicationGroup.POST("/:id/process", application.Process)

		// foo routers
	}

	_ = r.Run()
}
