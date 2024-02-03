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
	MetaforoData     metaforoData    `json:"metaforoData" yaml:"metaforoData"`
	Admin            adminData       `json:"admin" yaml:"admin"`
	ProposalData     proposalData    `json:"proposalData" yaml:"proposalData"`
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
		Desktop pushOneSignalConfig `json:"desktop" yaml:"desktop"`
		Mobile  pushOneSignalConfig `json:"mobile" yaml:"mobile"`
	}
	pushOneSignalConfig struct {
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
		SeedaoSppBase          string `json:"SeedaoSppBase" yaml:"SeedaoSppBase"`
		SeedaoEventIndexerBase string `json:"SeedaoEventIndexerBase" yaml:"SeedaoEventIndexerBase"`
		SentryDsn              string `json:"SentryDsn" yaml:"SentryDsn"`
	}
	publicData struct {
		Discord struct {
			Token   string `json:"token" yaml:"token"`
			GuildID string `json:"guildID" yaml:"guildID"`
		} `json:"discord" yaml:"discord"`
		CacheInSeconds int64 `json:"cacheInSeconds" yaml:"cacheInSeconds"`
		Notion         struct {
			APIToken string `json:"APIToken" yaml:"APIToken"`
		} `json:"notion" yaml:"notion"`
		SafeVaults []struct {
			ChainId int    `json:"chainId" yaml:"chainId"`
			Wallet  string `json:"wallet" yaml:"wallet"`
		} `json:"safeVaults" yaml:"safeVaults"`
	}

	metaforoData struct {
		GroupName   string `json:"groupName" yaml:"groupName"`
		GroupID     int    `json:"groupID" yaml:"groupID"`
		AccessToken string `json:"accessToken" yaml:"accessToken"`

		Assets []struct {
			ID   uint   `json:"id" yaml:"id"`
			Name string `json:"name" yaml:"name"`
		} `json:"assets" yaml:"assets"`

		// ProposalPrefix adds a prefix to all proposals created in OS system
		ProposalPrefix string `json:"proposalPrefix" yaml:"proposalPrefix"`
	}

	adminData struct {
		AuthToken string `json:"authToken" yaml:"authToken"`
	}

	proposalData struct {
		SipInitNumber int `json:"sipInitNumber" yaml:"sipInitNumber"`
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
