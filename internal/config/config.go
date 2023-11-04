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
	DataSource       dataSource      `json:"dataSource" yaml:"dataSource"`
	Jwt              jwt             `json:"jwt" yaml:"jwt"`
	Auth             auth            `json:"auth" yaml:"auth"`
	PreviewMode      previewMode     `json:"previewMode" yaml:"previewMode"`
	Casbin           casbin          `json:"casbin" yaml:"casbin"`
	Push             push            `json:"push" yaml:"push"`
	AwsConfig        awsConfig       `json:"awsConfig" yaml:"awsConfig"`
	ExternalServices externalService `json:"externalServices" yaml:"externalServices"`
	PublicData       publicData      `json:"publicData" yaml:"publicData"`
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
	push struct {
		BaseURI string `json:"baseURI" yaml:"baseURI"`
		Token   string `json:"token" yaml:"token"`

		OneSignalAppId  string `json:"oneSignalAppId" yaml:"oneSignalAppId"`
		OneSignalAppKey string `json:"oneSignalAppKey" yaml:"oneSignalAppKey"`
	}
	awsConfig struct {
		AccessKey  string `json:"accessKey" yaml:"accessKey"`
		SecretKey  string `json:"secretKey" yaml:"secretKey"`
		Region     string `json:"region" yaml:"region"`
		BucketName string `json:"bucketName" yaml:"bucketName"`
	}
	externalService struct {
		SeedaoSppBase string `json:"SeedaoSppBase" yaml:"SeedaoSppBase"`
	}
	publicData struct {
		Discord struct {
			Token   string `json:"token" yaml:"token"`
			GuildID string `json:"guildID" yaml:"guildID"`
		} `json:"discord" yaml:"discord"`
		Contracts struct {
			SCR  string `json:"scr" yaml:"scr"`
			Seed string `json:"seed" yaml:"seed"`
			Node string `json:"node" yaml:"node"`
		} `json:"contracts" yaml:"contracts"`
		CacheInSeconds int64  `json:"cacheInSeconds" yaml:"cacheInSeconds"`
		MainnetRPC     string `json:"mainnetRPC" yaml:"mainnetRPC"`
		SppIndexerHost string `json:"sppIndexerHost" yaml:"sppIndexerHost"`
		Notion         struct {
			APIToken string `json:"APIToken" yaml:"APIToken"`
		} `json:"notion" yaml:"notion"`
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
