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
	Jwt        jwt        `json:"jwt" yaml:"jwt"`
	DataSource dataSource `json:"dataSource" yaml:"dataSource"`
}

type (
	dataSource struct {
		Dsn string `json:"dsn" yaml:"dsn"`
	}
	jwt struct {
		Secret string `json:"secret" yaml:"secret" env:"JWT_SECRET,overwrite"`
		Exp    int    `json:"exp" yaml:"exp" env:"JWT_EXP,overwrite"`
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
