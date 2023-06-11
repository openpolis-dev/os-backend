package main

import (
	"flag"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
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
		user := v1.Group("/user")
		user.POST("/login", api.Login)

		// foo routers
	}
	// --> auth required
	{
		authorized := v1.Group("/", middleware.AuthRequired)

		// user routers
		user := authorized.Group("/user")
		user.GET("/logout", api.Logout)

		// foo routers
	}

	_ = r.Run()
}
