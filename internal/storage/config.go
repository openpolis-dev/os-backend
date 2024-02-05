package storage

import (
	"github.com/theseed-labs/os-backend/internal/config"
)

var cfg *config.Config

func GetConfig() *config.Config {
	return cfg
}

func SetConfig(c *config.Config) {
	cfg = c
}
