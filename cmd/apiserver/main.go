package main

import (
	"flag"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api/common_budget_sources"
	"github.com/theseed-labs/os-backend/internal/api/cron_jobs"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/api/sns_invite"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/graph/generated"
	"github.com/theseed-labs/os-backend/internal/graph/resolver"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/theseed-labs/os-backend/internal/task_manager"
	"gorm.io/gorm"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/theseed-labs/os-backend/internal/api/data_srv"
	"github.com/theseed-labs/os-backend/internal/api/rewards"

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
	pushSDK := []sdk.Pusher{
		// need pushing to desktop and mobile
		sdk.NewOneSignal(cfg.Push.Desktop.OneSignalAppId, cfg.Push.Desktop.OneSignalAppKey),
		sdk.NewOneSignal(cfg.Push.Mobile.OneSignalAppId, cfg.Push.Mobile.OneSignalAppKey),
	}

	// setup permission system
	adapter, err := gormadapter.NewAdapter(cfg.Casbin.DriverName, cfg.DataSource.Dsn, true)
	if err != nil {
		panic(err)
	}
	enforcer, err := casbin.NewSyncedEnforcer(*casbinModelConfPath, adapter)
	if err != nil {
		panic(err)
	}
	// add default policies
	defaultPolicies := [][]string{
		{internal.RoleHall, "*", "*"}, // `p, hall, *, *` hall can do anything
	}
	_, err = enforcer.AddPolicies(defaultPolicies)
	if err != nil {
		panic(err)
	}
	// add default users
	groupPolicies := lo.Map[string, []string](cfg.Casbin.SuperUsers, func(user string, _ int) []string {
		return []string{common.FormatUserWallet(user), internal.RoleHall}
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
	storage.InitGormDB(cfg.DataSource.Dsn, cfg.Casbin.DriverName)
	db := storage.GetGormDB()
	//err = storage.MigrateTables(db)
	if err != nil {
		panic(err)
	}

	// Load configured metaforo data from DB and save to config
	mfData, err := model.GetMetaforoData(db)
	if err != nil {
		panic(err)
	}
	if err = cfg.PopulateMetaforoDataFromDB(db, mfData); err != nil {
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

	// Setup zerolog
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack
	zerolog.TimestampFieldName = "t"
	zerolog.LevelFieldName = "l"
	zerolog.MessageFieldName = "m"
	log.Logger = log.With().Caller().Logger()

	// setup task manager and start runner
	task_manager.InitTaskManager(db, 5, cfg)
	task_manager.GetTaskManager().StartRunner()

	storage.SetConfig(cfg)

	setupCronJob(cfg, db)

	r := setupRouter(cfg, db, enforcer, pushSDK)
	_ = r.Run()
}

func setupRouter(cfg *config.Config, db *gorm.DB, enforcer *casbin.SyncedEnforcer, pushSDK []sdk.Pusher) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.RequestMetricsRecord())
	r.Use(middleware.ResponseMetricsRecord())

	sdk.InitSentry(r, cfg)

	r.GET("/prometheus_metrics", gin.WrapH(promhttp.Handler()))

	r.Use(gzip.Gzip(gzip.DefaultCompression))

	// setup cors refer: https://github.com/gin-contrib/cors
	corsCfg := cors.DefaultConfig()
	corsCfg.AllowAllOrigins = true
	corsCfg.AllowHeaders = []string{"Origin", "Accept", "Content-Type", "Authorization"}
	r.Use(cors.New(corsCfg))

	storage.SetEnforcer(enforcer)

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
		userGroup.POST("/login", user.Login)
		userGroup.GET("/users", user.Users)
		userGroup.GET("/casbin", user.GetFrontendPermission)
		userGroup.GET("/metaforo_activities", user.MetaforoActivities)

		// SeeAuth apis
		seeAuth := v1.Group("/seeauth")
		seeAuth.GET("/nonce/:wallet", user.SeeAuthNonce)
		seeAuth.POST("/login", user.LoginWithSeeAuth)
		seeAuth.POST("/seeauth_3rd_test", user.SeeAuthTestApi)

		// project routers
		projGroup := v1.Group("/projects")
		projGroup.GET("/", project.List)
		projGroup.GET("/:id", project.Detail)
		projGroup.GET("/:id/budgets", project.ShowBudgets)

		// guild routers
		guildGroup := v1.Group("/guilds")
		guildGroup.GET("/", guild.List)
		guildGroup.GET("/:id", guild.Detail)
		guildGroup.GET("/:id/budgets", guild.ShowBudgets)

		commonBudgetSourceGroup := v1.Group("/common_budget_sources")
		commonBudgetSourceGroup.GET("/", common_budget_sources.List)

		// application routers
		applicationGroup := v1.Group("/applications")
		applicationGroup.GET("/:id", application.Detail)
		applicationGroup.GET("/", application.List)

		v1.GET("/apps_applicants", application.ListApplicants)
		v1.GET("/download_applications", application.Download)

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
		publicData.GET("/notion/database/:id", publicdata.NotionDatabase)
		publicData.GET("/notion/page/:id", publicdata.NotionPage)
		publicData.GET("/notion/user/:id", publicdata.NotionUser)
		publicData.GET("/safe_vault", publicdata.SafeVault)

		// webhook routers
		webhookGroup := v1.Group("/webhook")
		webhookGroup.POST("/tally", webhook.Tally)

		// season data
		seasonsData := v1.Group("/seasons")
		seasonsData.GET("/", season.List)
		seasonsData.GET("/current", season.Current)

		// some data service
		// TODO: Move to authorized group
		dataSrv := v1.Group("/data_srv")
		dataSrv.GET("/aggr_scr", data_srv.AggrScr)

		// Proposal component routers
		componentRouter := v1.Group("/proposal_components")
		componentRouter.GET("/", proposal.ListComponents)
		componentRouter.GET("/:id", proposal.GetComponent)

		proposalPollGateRouter := v1.Group("/proposal_vote_gates")
		proposalPollGateRouter.GET("/", proposal.ListVoteGates)

		// Proposal routers
		proposalGroup := v1.Group("/proposals")
		proposalGroup.GET("/list", proposal.List)
		proposalGroup.GET("/show/:id", proposal.Detail)
		proposalGroup.GET("/vote_detail/:vote_option_id", proposal.ShowVoteDetail)

		// All proposal categories for non login users
		proposalCategoryRouter := v1.Group("/proposal_categories")
		proposalCategoryRouter.GET("/list", proposal.ListAllCategories)

		// Proposal templates router
		proposalTmplRouter := v1.Group("/proposal_tmpl")
		proposalTmplRouter.GET("/list", proposal.ListTemplates)

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
		userGroup.POST("/join_metaforo_group", user.JoinMetaforoGroup)
		userGroup.POST("/leave_metaforo_group", user.LeaveMetaforoGroup)
		userGroup.POST("/prepare_metaforo", user.PrepareMetaforoData)

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
		guildGroup.POST("/:id/close", guild.Close)
		// my guilds
		authorizedGroup.GET("/my_guilds", guild.MyGuilds)

		// application endpoints
		// Note: Most NEW_REWARD applications has been moved to app_bundle part
		applicationGroup := authorizedGroup.Group("/applications")
		applicationGroup.POST("/", application.Create)

		appBundleGroup := authorizedGroup.Group("/app_bundles")
		appBundleGroup.GET("/available_projects_guilds", app_bundle.ListAvailableProjectsAndGuilds)
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
		cityHallGroup.POST("/batch_update_members", city_hall.BatchUpdateMembers)

		// push routers
		pushGroup := authorizedGroup.Group("/push")
		pushGroup.POST("/", push.Create)
		pushGroup.GET("/", push.List)

		// reward routers
		rewardsGroup := authorizedGroup.Group("/rewards")
		rewardsGroup.POST("/approve_mint_reward", rewards.ApproveMintReward)
		rewardsGroup.POST("/snapshot_seed", rewards.SnapshotSeed)

		// proposal routers
		proposalGroup := authorizedGroup.Group("/proposals")
		// Save or update proposal to Local DB. If submit flag in post data is true,
		// the proposal will also be published to Metaforo and convert to Draft state
		proposalGroup.POST("/create", proposal.Create)
		proposalGroup.POST("/update/:id", proposal.Update)
		proposalGroup.POST("/add_comment/:id", proposal.AddComment)
		proposalGroup.POST("/edit_comment/:id", proposal.EditComment)
		proposalGroup.POST("/delete_comment/:id", proposal.DeleteComment)
		proposalGroup.GET("/my", proposal.MyList)
		proposalGroup.GET("/creating_project_proposals", proposal.GetProposalsUsedForCreatingProjects)

		// State change actions for proposals
		proposalGroup.POST("/withdraw/:id", proposal.Withdraw)
		proposalGroup.POST("/approve/:id", proposal.Approve)
		proposalGroup.POST("/reject/:id", proposal.Reject)

		proposalGroup.POST("/can_vote/:id", proposal.CheckVotePermission)
		proposalGroup.POST("/vote/:id", proposal.CastVote)
		proposalGroup.POST("/revoke_vote/:id", proposal.RevokeVote)
		proposalGroup.POST("/close_vote/:id", proposal.CloseVote)

		// Proposal templates router
		proposalTmplRouter := authorizedGroup.Group("/proposal_tmpl")
		proposalTmplRouter.GET("/list_with_perm", proposal.ListTemplatesWithPerm)

		// List proposal categories
		proposalCategoryRouter := authorizedGroup.Group("/proposal_categories")
		proposalCategoryRouter.GET("/list_with_perm", proposal.ListCategoriesWithPerm)

		// Data services API
		dataSrv := authorizedGroup.Group("/data_srv")
		dataSrv.GET("/widget_data", data_srv.WidgetData)

		// SNS invite
		snsInvite := authorizedGroup.Group("/sns_invite")
		snsInvite.GET("/my_sns_invite_code", sns_invite.GetMySnsInviteCode)
		snsInvite.GET("/my_sns_invite_rewards", sns_invite.GetMySnsInviteRewards)
		snsInvite.POST("/invited_by/:invite_code", sns_invite.SnsInvitedBy)
	}
	{
		adminGroup := r.Group("/admin", middleware.AdminPermissionRequired)
		proposalTmplAdminRouter := adminGroup.Group("/proposal_tmpl")
		proposalTmplAdminRouter.POST("/update", proposal.UpdateTemplate)

		userAdminRouter := adminGroup.Group("/user")
		userAdminRouter.POST("/check_vote_permission", user.CheckVotePermission)

		// Schedule jobs routers
		jobsRouter := adminGroup.Group("/jobs")
		jobsRouter.GET("/list", cron_jobs.List)

		// TODO: New tasks required
		//metaforoRouter := adminGroup.Group("/mf")
		//metaforoRouter.POST("/update_mf_admin_token", TBD)
		//metaforoRouter.POST("/sync_perm_group", TBD)

		// metaforo mint count update
		adminGroup.POST("/metaforo_mint_data_update", data_srv.UpdateMetaforoVoteData)
	}

	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	r.POST("/graphql/query", middleware.GqlAuth, middleware.GinContextToContextMiddleware, graphqlHandler())
	r.GET("/graphql", middleware.GqlAuth, middleware.GinContextToContextMiddleware, playgroundHandler())

	return r
}

func setupCronJob(cfg *config.Config, db *gorm.DB) {
	// cron task
	c := cron.New()
	// (Minutes Hours Day-of-Month Month Day-of-Week)
	// "@every 10m"

	if cfg.CronJob.CheckAndUpdateUnverifiedSnsInvite == "" {
		log.Warn().Msg("cron job for checking sns invite not set")
		return
	}

	// CheckAndUpdateUnverifiedSnsInvite Job
	if _, err := c.AddFunc(cfg.CronJob.CheckAndUpdateUnverifiedSnsInvite, func() {
		_ = service.CheckAndUpdateUnverifiedSnsInvite(cfg, db)
	}); err != nil {
		panic(err)
	}

	// other cron jobs

	c.Start()
}

// defining the Graphql handler
func graphqlHandler() gin.HandlerFunc {
	// NewExecutableSchema and Config are in the generated.go file
	// Resolver is in the resolver.go file
	h := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &resolver.Resolver{}}))

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// defining the Playground handler
func playgroundHandler() gin.HandlerFunc {
	h := playground.Handler("GraphQL", "/graphql/query")

	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
