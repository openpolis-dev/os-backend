package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type Config struct {
	DataSource   dataSource   `json:"dataSource" yaml:"dataSource"`
	Jwt          jwt          `json:"jwt" yaml:"jwt"`
	Auth         auth         `json:"auth" yaml:"auth"`
	PreviewMode  previewMode  `json:"previewMode" yaml:"previewMode"`
	Casbin       casbin       `json:"casbin" yaml:"casbin"`
	Notification notification `json:"notification" yaml:"notification"`
}

type (
	dataSource struct {
		Dsn string `json:"dsn" yaml:"dsn"`
	}
	jwt struct {
		Secret string `json:"secret" yaml:"secret" env:"JWT_SECRET,overwrite"`
		Exp    int    `json:"exp" yaml:"exp" env:"JWT_EXP,overwrite"`
	}
	auth struct {
		PolygonRPC    string `json:"polygonRPC" yaml:"polygonRPC"`
		NonceLifespan int64  `json:"nonceLifespan" yaml:"nonceLifespan"`
	}
	previewMode struct {
		Wallet string `json:"wallet" yaml:"wallet"`
	}
	casbin struct {
		DriverName string   `json:"driverName" yaml:"driverName"`
		SuperUsers []string `json:"superUsers" yaml:"superUsers"`
	}
	notification struct {
		AppID  string `json:"appID" yaml:"appID"`
		AppKey string `json:"appKey" yaml:"appKey"`
	}
)

func LoadConfig(configPath string) *Config {
	_, err := os.Stat(configPath)

	if errors.Is(err, os.ErrNotExist) {
		log.Panic().Msg(fmt.Sprintf("File %s not existing", configPath))
	}

	configContent, err := os.ReadFile(configPath)
	if err != nil {
		log.Err(err).Msg("")
	}

	var config Config

	switch path.Ext(configPath) {
	case ".yml", ".yaml":
		err := yaml.Unmarshal(configContent, &config)
		if err != nil {
			log.Err(err).Msg("")
		}
	case ".json":
		err := json.Unmarshal(configContent, &config)
		if err != nil {
			log.Err(err).Msg("")
		}
	default:
		log.Panic().Msg("Unknown config format, only json or yaml are supported")
	}

	return &config
}
