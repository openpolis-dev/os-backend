package main

import (
	"flag"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/theseed-labs/os-backend/internal/storage"
)

func main() {
	cfgPath := flag.String("config", "config.yml", "Configuration file path")
	outputPath := flag.String("output", "./export/sns_chain_registry.sql", "Output SQL file path")
	flag.Parse()

	cfg := config.LoadConfig(*cfgPath)
	storage.InitGormDB(cfg.DataSource.Dsn, cfg.Casbin.DriverName)
	db := storage.GetGormDB()

	if err := model.MigrateTables(db); err != nil {
		log.Fatal().Err(err).Msg("migrate tables failed")
	}

	if err := service.ExportSnsChainRegistrySQL(db, *outputPath); err != nil {
		log.Fatal().Err(err).Msg("export sns registry failed")
	}
}
