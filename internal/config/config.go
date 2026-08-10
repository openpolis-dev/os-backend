package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/internal"
	"gopkg.in/yaml.v2"
	"gorm.io/gorm"
)

type Config struct {
	DataSource       dataSource      `json:"dataSource" yaml:"dataSource"`
	Jwt              jwt             `json:"jwt" yaml:"jwt"`
	Auth             auth            `json:"auth" yaml:"auth"`
	PreviewMode      previewMode     `json:"previewMode" yaml:"previewMode"`
	Casbin           casbin          `json:"casbin" yaml:"casbin"`
	CronJob          cronJob         `json:"cronJob" yaml:"cronJob"`
	Push             push            `json:"push" yaml:"push"`
	AwsConfig        awsConfig       `json:"awsConfig" yaml:"awsConfig"`
	ExternalServices externalService `json:"externalServices" yaml:"externalServices"`
	PublicData       publicData      `json:"publicData" yaml:"publicData"`
	MetaforoData     metaforoData    `json:"metaforoData" yaml:"metaforoData"`
	Admin            adminData       `json:"admin" yaml:"admin"`
	QuickAccounting  QuickAccounting `json:"quickAccounting" yaml:"quickAccounting"`
	SnsInvite        SnsInvite       `json:"snsInvite" yaml:"snsInvite"`
	SnsChainSync     SnsChainSync    `json:"snsChainSync" yaml:"snsChainSync"`
	DsChatConfig     DsChatConfig    `json:"dsChatConfig" yaml:"dsChatConfig"`
	// ProposalInitiateAllowlist: emergency bypass for proposal initiate/vote permission when SPP/Seepass is unavailable. Remove after spp-backend is deployed.
	ProposalInitiateAllowlist []string `json:"proposalInitiateAllowlist" yaml:"proposalInitiateAllowlist"`
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
	cronJob struct {
		CheckAndUpdateUnverifiedSnsInvite string `json:"checkAndUpdateUnverifiedSnsInvite" yaml:"checkAndUpdateUnverifiedSnsInvite"`
		SyncSnsChainRegistry              string `json:"syncSnsChainRegistry" yaml:"syncSnsChainRegistry"`
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
		// SeedaoEventIndexerHTTPTimeoutSeconds is the total timeout for each HTTP call to the indexer (snapshot, computenodesbt, etc.). Zero means use apiserver default (55s), still bounded so gateways do not 504 first when possible.
		SeedaoEventIndexerHTTPTimeoutSeconds int `json:"SeedaoEventIndexerHTTPTimeoutSeconds" yaml:"SeedaoEventIndexerHTTPTimeoutSeconds"`
		SentryDsn                            string `json:"SentryDsn" yaml:"SentryDsn"`
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

		ProposalAssets []struct {
			ID   uint   `json:"id" yaml:"id"`
			Name string `json:"name" yaml:"name"`
		} `json:"proposal_assets" yaml:"proposal_assets"`

		ApplicationAssets []struct {
			ID   uint   `json:"id" yaml:"id"`
			Name string `json:"name" yaml:"name"`
		} `json:"application_assets" yaml:"application_assets"`

		// ProposalPrefix adds a prefix to all proposals created in OS system
		ProposalPrefix string `json:"proposalPrefix" yaml:"proposalPrefix"`
	}

	adminData struct {
		AuthToken string `json:"authToken" yaml:"authToken"`
	}

	QuickAccounting struct {
		Url          string `json:"url" yaml:"url"`
		WorkspaceId  int    `json:"workspaceId" yaml:"workspaceId"`
		CategoryId   int    `json:"categoryId" yaml:"categoryId"`
		CategoryName string `json:"categoryName" yaml:"categoryName"`
	}
	SnsInvite struct {
		EntityType string `json:"entityType" yaml:"entityType"`
		EntityId   uint   `json:"entityId" yaml:"entityId"`
		EntityName string `json:"entityName" yaml:"entityName"`
		Applicant  string `json:"applicant" yaml:"applicant"`
	}

	SnsChainSync struct {
		// IndexerDataDbPath is optional fallback when GET /sns/all returns empty (spp-indexer sqlite mount).
		IndexerDataDbPath string `json:"indexerDataDbPath" yaml:"indexerDataDbPath"`
		// IndexerDataDbQuery overrides auto-discovered sqlite query.
		IndexerDataDbQuery string `json:"indexerDataDbQuery" yaml:"indexerDataDbQuery"`
		// ExportPath writes PostgreSQL UPSERT SQL after each successful sync (offline import to seedao-api-server).
		ExportPath string `json:"exportPath" yaml:"exportPath"`
	}

	DsChatConfig struct {
		BaseUrl string `json:"baseUrl" yaml:"baseUrl"`
		AuthKey string `json:"authKey" yaml:"authKey"`
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

// PopulateMetaforoDataFromDB loads metaforo data configured in system variables table into the config object
func (c *Config) PopulateMetaforoDataFromDB(db *gorm.DB, mfData map[string]string) error {
	var err error
	// Metaforo related data
	c.MetaforoData.AccessToken = mfData[internal.SysVarMfAdminToken]
	c.MetaforoData.GroupName = mfData[internal.SysVarMfGroupName]
	c.MetaforoData.GroupID, err = strconv.Atoi(mfData[internal.SysVarMfGroupId])
	if err != nil {
		log.Error().Msgf("parse metaforo group ID error: %+v", err)
		return err
	} else {
		return nil
	}
}
