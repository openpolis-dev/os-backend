package main

import (
	"flag"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
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
	flag.Parse()
	cfg := config.LoadConfig(*cfgPath)

	// setup database
	storage.InitGormDB(cfg.DataSource.Dsn)
	db := storage.GetGormDB()

	r := gin.Default()

	// setup cors refer: https://github.com/gin-contrib/cors
	corsCfg := cors.DefaultConfig()
	corsCfg.AllowAllOrigins = true
	r.Use(cors.New(corsCfg))

	// setup basic middleware for database connection and config data
	r.Use(func(ctx *gin.Context) {
		ctx.Set(middleware.DBKey, db)
		ctx.Set(middleware.CfgKey, cfg)

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
		userGroup.POST("/users", user.Users)

		// foo routers
	}
	// --> auth required
	{
		authorizedGroup := v1.Group("/", middleware.AuthRequired)

		// user routers
		userGroup := authorizedGroup.Group("/user")
		userGroup.GET("/logout", user.Logout)

		// project routers
		projGroup := authorizedGroup.Group("/project")
		projGroup.POST("/projects", project.Create)
		projGroup.PUT("/projects/:id", project.Update)
		projGroup.GET("/projects/close", project.Close)
		projGroup.GET("/projects/:id", project.Detail)
		projGroup.GET("/projects", project.List)
		projGroup.POST("/projects/:id/update_sponsors", project.UpdateSponsors)
		projGroup.POST("/projects/:id/update_members", project.UpdateMembers)
		projGroup.POST("/projects/:id/update_budget", project.UpdateBudget)
		projGroup.POST("/projects/:id/add_related_proposal/:proposal_id", project.AddRelatedProposal)

		// application routers
		applicationGroup := authorizedGroup.Group("/applications")
		applicationGroup.GET("/", application.List)
		applicationGroup.POST("/", application.Create)
		applicationGroup.POST("/approve", application.BatchApprove)
		applicationGroup.POST("/reject", application.BatchReject)
		applicationGroup.POST("/complete", application.BatchComplete)
		applicationGroup.POST("/process", application.BatchProcess)
		applicationGroup.GET("/:id", application.Detail)
		applicationGroup.POST("/:id/approve", application.Approve)
		applicationGroup.POST("/:id/reject", application.Reject)
		applicationGroup.POST("/:id/complete", application.Complete)
		applicationGroup.POST("/:id/process", application.Process)

		// foo routers
	}

	_ = r.Run()
}
