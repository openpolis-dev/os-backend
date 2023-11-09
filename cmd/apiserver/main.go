package main

import (
	"flag"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/theseed-labs/os-backend/internal/api/data_srv"

	"github.com/casbin/casbin/v2"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	_ "github.com/theseed-labs/os-backend/docs"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/app_bundle"
	"github.com/theseed-labs/os-backend/internal/api/application"
	"github.com/theseed-labs/os-backend/internal/api/city_hall"
	"github.com/theseed-labs/os-backend/internal/api/event"
	"github.com/theseed-labs/os-backend/internal/api/guild"
	"github.com/theseed-labs/os-backend/internal/api/permission"
	"github.com/theseed-labs/os-backend/internal/api/project"
	"github.com/theseed-labs/os-backend/internal/api/publicdata"
	"github.com/theseed-labs/os-backend/internal/api/push"
	"github.com/theseed-labs/os-backend/internal/api/season"
	"github.com/theseed-labs/os-backend/internal/api/treasury"
	"github.com/theseed-labs/os-backend/internal/api/user"
	"github.com/theseed-labs/os-backend/internal/api/webhook"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/middleware"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/storage"
)

//	@title			OS Backend API service
//	@version		1.0
//	@license.name	MIT
//	@host			https://test-api.seedao.tech
//	@basePath		/v1

func main() {
	// read config data
	cfgPath := flag.String("config", "config.yml", "Configuration file path, should be yaml or json format")
	casbinModelConfPath := flag.String("casbin-model", "rbac_model.conf", "casbin model conf file path, should be conf format")
	flag.Parse()
	cfg := config.LoadConfig(*cfgPath)
	log.Debug().Msgf("application configuration: %+v", cfg)

	// setup push sdk
	//pushSDK := sdk.NewFCM(cfg.Push.BaseURI, cfg.Push.Token)
	pushSDK := sdk.NewOneSignal(cfg.Push.OneSignalAppId, cfg.Push.OneSignalAppKey)

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
		{api.RoleTreasuryManager, api.ObjTreasury, api.ActUpdateAssertBudget}, // `p, treasury_manager, treasury, u_assert_budget`
		{api.RoleEventManager, api.ObjEvent, api.ActCreateEvent},              // `p, event_manager, event, create_event`
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
	storage.MigrateTables()
	storage.SeedDbRecords()
	db := storage.GetGormDB()
	if err != nil {
		panic(err)
	}

	// setup cache
	// Currently the cache is only used by saving aggregated data, may be extended to other data in future
	storage.InitCache()

	// setup S3 uploader manager
	err = sdk.InitAwsClient(cfg.AwsConfig.AccessKey, cfg.AwsConfig.SecretKey, cfg.AwsConfig.Region, cfg.AwsConfig.BucketName)
	if err != nil {
		panic(err)
	}

	// setup Spp API client
	err = sdk.InitSppClient(cfg.ExternalServices.SeedaoSppBase)
	if err != nil {
		panic(err)
	}

	// setup Indexer API Client
	err = sdk.InitIndexerClient(cfg.ExternalServices.SeedaoEventIndexerBase)
	if err != nil {
		panic(err)
	}

	r := gin.Default()
	r.Use(gzip.Gzip(gzip.DefaultCompression))

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
		ctx.Set(middleware.PushKey, pushSDK)

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
		userGroup.POST("/refresh_nonce", user.RefreshNonce)
		userGroup.GET("/retrieve_nonce", user.RetrieveNonce)
		userGroup.POST("/login", user.Login)
		userGroup.GET("/users", user.Users)
		userGroup.GET("/casbin", user.GetFrontendPermission)

		// project routers
		projGroup := v1.Group("/projects")
		projGroup.GET("/", project.List)
		projGroup.GET("/:id", project.Detail)

		// guild routers
		guildGroup := v1.Group("/guilds")
		guildGroup.GET("/", guild.List)
		guildGroup.GET("/:id", guild.Detail)

		// application routers
		applicationGroup := v1.Group("/applications")
		applicationGroup.GET("/:id", application.Detail)
		applicationGroup.GET("/", application.List)

		v1.GET("/apps_applicants", application.ListApplicants)
		v1.GET("/download_applications", application.Download)
		v1.GET("/get_applications_upload_template", application.DownloadUploadTemplate)

		// SeeDAO assets routers
		treasuryGroup := v1.Group("/treasury")
		treasuryGroup.GET("/current", treasury.GetOrCreateCurrentAssetRecords)

		// SeeDAO events routers
		eventsGroup := v1.Group("/events")
		eventsGroup.GET("/", event.List)
		eventsGroup.GET("/:id", event.Detail)

		// pre-signed s3 upload url
		v1.GET("/url_for_uploading_s3", api.PreSignedUrlForS3)

		cityHallGroup := v1.Group("/cityhall")
		cityHallGroup.GET("/info", city_hall.Info)

		// public data
		publicData := v1.Group("/public_data")
		publicData.GET("/discord_member_count", publicdata.DiscordData)
		publicData.GET("/contract/seed", publicdata.SeedDataFromIndexer)
		publicData.GET("/contract/scr", publicdata.SCRDataFromIndexer)
		publicData.GET("/contract/node", publicdata.NodeDataFromIndexer)
		publicData.GET("/notion/database/:id", publicdata.NotionDatabase)
		publicData.GET("/notion/page/:id", publicdata.NotionPage)
		publicData.GET("/notion/user/:id", publicdata.NotionUser)

		// webhook routers
		webhookGroup := v1.Group("/webhook")
		webhookGroup.POST("/tally", webhook.Tally)

		// season data
		seasonsData := v1.Group("/seasons")
		seasonsData.GET("/", season.List)

		// some data service
		// TODO: Move to authorized group
		dataSrv := v1.Group("/data_srv")
		dataSrv.GET("/aggr_scr", data_srv.AggrScr)

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
		projGroup.POST("/:id/update_staffs", project.UpdateStaffs)
		projGroup.POST("/:id/update_budget", project.UpdateBudget)
		projGroup.POST("/:id/add_related_proposal", project.AddRelatedProposal)
		// my projects
		authorizedGroup.GET("/my_projects", project.MyProjects)

		// guild routers
		guildGroup := authorizedGroup.Group("/guilds")
		guildGroup.POST("/", guild.Create)
		guildGroup.PUT("/:id", guild.Update)
		guildGroup.POST("/:id/update_staffs", guild.UpdateStaffs)
		guildGroup.POST("/:id/update_budget", guild.UpdateBudget)
		guildGroup.POST("/:id/add_related_proposal", guild.AddRelatedProposal)
		// my guilds
		authorizedGroup.GET("/my_guilds", guild.MyGuilds)

		// application endpoints
		// Note: Most NEW_REWARD applications has been moved to app_bundle part
		applicationGroup := authorizedGroup.Group("/applications")
		applicationGroup.POST("/", application.Create)

		appBundleGroup := authorizedGroup.Group("/app_bundles")
		appBundleGroup.GET("/", app_bundle.ListAppBundle)
		appBundleGroup.POST("/", app_bundle.CreateAppBundle)

		authorizedGroup.POST("/app_bundle_approve", app_bundle.ApproveAppBundles)
		authorizedGroup.POST("/app_bundle_reject", app_bundle.RejectAppBundles)

		// batch application routers
		authorizedGroup.POST("/apps_approve", application.BatchApprove)
		authorizedGroup.POST("/apps_reject", application.BatchReject)
		authorizedGroup.POST("/apps_process", application.BatchProcess)
		authorizedGroup.POST("/apps_complete", application.BatchComplete)

		// SeeDAO assets routers
		treasuryGroup := authorizedGroup.Group("/treasury")
		treasuryGroup.POST("/update_assets", treasury.UpdateAssets)

		// SeeDAO events routers
		eventsGroup := authorizedGroup.Group("/events")
		eventsGroup.POST("/", event.Create)
		eventsGroup.PUT("/:id", event.Update)
		eventsGroup.DELETE("/:id", event.Delete)

		// my events
		authorizedGroup.GET("/my_events", event.MyList)

		// permission routers
		permissionGroup := authorizedGroup.Group("/permission")
		permissionGroup.POST("/grant_role", permission.GrantRole)
		permissionGroup.POST("/revoke_role", permission.RevokeRole)

		// city hall
		cityHallGroup := authorizedGroup.Group("/cityhall")
		cityHallGroup.POST("/update_budget", city_hall.UpdateBudget)
		cityHallGroup.POST("/update_members", city_hall.UpdateMember)

		// push routers
		pushGroup := authorizedGroup.Group("/push")
		pushGroup.POST("/", push.Create)
		pushGroup.GET("/", push.List)

		// foo routers
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	_ = r.Run()
}
