package publicdata_inject

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

type PublicDataController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	PublicDataService *PublicDataService `inject:""`
}

const getSeasonPropsalsSQL = `WITH max_version_proposals AS (
    SELECT
        proposals.proposal_record_id,
        MAX(version) AS max_version
    FROM
        proposals
    GROUP BY
        proposals.proposal_record_id
)
select concat('https://app.seedao.xyz/proposal/thread/', p.id::text) as link, s.name as season, pc.name as category, p.title as title, p.create_ts as create, p.state as state, u.wallet as applicant, u.name as name, u.avatar as avatar
from proposals p
         join max_version_proposals mvp on p.proposal_record_id = mvp.proposal_record_id
         join proposal_categories pc on p.proposal_category_id = pc.id
         join proposal_vote_gates pg on p.vote_gate_id = pg.id
         join seasons s on s.id = pg.season_id
		 join users u on p.applicant = u.wallet
where pc.id in (21, 22, 24) and p.sip is not null and p.sip > 0`

const getSeasonUsersSQL = `select wallet, name, avatar from users where wallet in `

const (
	seasonNodesLogComponent = "public_data"
	seasonNodesLogOperation = "get_season_nodes"
	// seasonNodesLogFixme is a stable tag for grep/alerting until spp-indexer season snapshot is healthy again.
	seasonNodesLogFixme          = "FIXME:restore-spp-indexer-season-nodes"
	seasonNodeIndexerFetchTimeout = 10 * time.Second
)

func Register(fatherGroup *gin.RouterGroup) {
	g := global_object.GetGlobalObject()

	var publicData PublicDataController

	err := inject.Populate(&publicData, g.Gin, g.Db, g.Cfg)

	if err != nil {
		panic(err)
	}

	var publicDataGroup *gin.RouterGroup

	if fatherGroup != nil {
		publicDataGroup = fatherGroup.Group("/public_data")
	} else {
		publicDataGroup = publicData.Gin.Group("/public_data")
	}

	publicDataGroup.GET("/discord_member_count", publicData.DiscordData)
	publicDataGroup.GET("/notion/database/:id", publicData.NotionDatabase)
	publicDataGroup.GET("/notion/page/:id", publicData.NotionPage)
	publicDataGroup.GET("/notion/user/:id", publicData.NotionUser)
	publicDataGroup.GET("/safe_vault", publicData.SafeVault)
	publicDataGroup.GET("/node_sbt_count", publicData.NodeSbtCount)
	publicDataGroup.GET("/get_season_proposals/:seasonIdx", publicData.GetSeasonProposals)
	publicDataGroup.GET("/get_season_nodes/:seasonIdx", publicData.GetSeasonNodes)
}

func seasonNodesLog(seasonIdx, stage string) *zerolog.Event {
	return log.Warn().
		Str("component", seasonNodesLogComponent).
		Str("operation", seasonNodesLogOperation).
		Str("season_idx", seasonIdx).
		Str("stage", stage).
		Str("fixme", seasonNodesLogFixme)
}

// GetSeasonNodes returns season node holders enriched from the users table.
//
// Steps:
//  1. Read in-memory cache (season_nodes.<idx>).
//  2. On cache miss, call spp-indexer ensoul SBT snapshot and filter by season token id.
//  3. On indexer failure/timeout, degrade to an empty list (HTTP 200) — see seasonNodesLogFixme.
//  4. Load wallet/name/avatar from Postgres for matched wallets.
func (c *PublicDataController) GetSeasonNodes(ctx *gin.Context) {
	seasonIdx := ctx.Param("seasonIdx")

	csNodeWallets, fromCache := c.loadSeasonNodeWallets(seasonIdx)
	if !fromCache {
		csNodeWallets = c.fetchSeasonNodeWalletsFromIndexer(seasonIdx)
	}

	if len(csNodeWallets) == 0 {
		ctx.JSON(http.StatusOK, api.Success([]*SeasonUsers{}))
		return
	}

	users := strings.Join(csNodeWallets, "','")
	findSql := fmt.Sprintf("%s ('%s')", getSeasonUsersSQL, users)
	var seasonUsers []*SeasonUsers
	err := c.Db.Raw(findSql).Find(&seasonUsers).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		seasonNodesLog(seasonIdx, "db_users").Err(err).Msg("query users for season nodes failed")
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("get season nodes error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(seasonUsers))
}

func (c *PublicDataController) loadSeasonNodeWallets(seasonIdx string) (wallets []string, ok bool) {
	cachedData, err := storage.GetCachedData(storage.SeasonNodeCacheKey(seasonIdx))
	if err != nil {
		log.Debug().
			Str("component", seasonNodesLogComponent).
			Str("operation", seasonNodesLogOperation).
			Str("season_idx", seasonIdx).
			Str("stage", "cache_miss").
			Msg("season node cache miss, will try indexer")
		return nil, false
	}

	if err = json.Unmarshal(cachedData, &wallets); err != nil {
		seasonNodesLog(seasonIdx, "cache_unmarshal").Err(err).Msg("season node cache corrupt, will try indexer")
		return nil, false
	}
	return wallets, true
}

func (c *PublicDataController) fetchSeasonNodeWalletsFromIndexer(seasonIdx string) []string {
	indexerClient := sdk.GetIndexerClient()
	if indexerClient == nil {
		seasonNodesLog(seasonIdx, "indexer_client").Msg("indexer client not initialized, returning empty season nodes")
		return nil
	}

	wallets, err := indexerClient.GetCurrentSeasonNodeListWithTimeout(seasonIdx, seasonNodeIndexerFetchTimeout)
	if err != nil {
		seasonNodesLog(seasonIdx, "indexer_fetch").
			Err(err).
			Dur("indexer_timeout", seasonNodeIndexerFetchTimeout).
			Msg("spp-indexer season node snapshot failed or timed out, returning empty list (degraded)")
		return nil
	}

	csNodeBytes, err := json.Marshal(wallets)
	if err != nil {
		seasonNodesLog(seasonIdx, "cache_marshal").Err(err).Msg("marshal season node list for cache failed, returning fetched wallets")
		return wallets
	}
	if err = storage.StoreCachedData(storage.SeasonNodeCacheKey(seasonIdx), csNodeBytes); err != nil {
		log.Debug().
			Str("component", seasonNodesLogComponent).
			Str("operation", seasonNodesLogOperation).
			Str("season_idx", seasonIdx).
			Str("stage", "cache_store").
			Err(err).
			Msg("store season node cache failed")
	}
	return wallets
}

func (c *PublicDataController) GetSeasonProposals(ctx *gin.Context) {
	seasonIdx := ctx.Param("seasonIdx")

	findSql := fmt.Sprintf("%s and s.idx = %s order by p.id desc;", getSeasonPropsalsSQL, seasonIdx)

	var seasonProposals []*SeasonProposals
	err := c.Db.Raw(findSql).Find(&seasonProposals).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("get season proposals error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(seasonProposals))
}

func (c *PublicDataController) DiscordData(ctx *gin.Context) {
	cacheLogic[discordgo.Session, discord](ctx, &discordCache, c.Cfg.PublicData.CacheInSeconds, func() (*discordgo.Session, error) {
		return discordgo.New("Bot " + c.Cfg.PublicData.Discord.Token)
	}, func() (*discord, error) {
		guild, err := discordCache.client.GuildWithCounts(c.Cfg.PublicData.Discord.GuildID)
		if err != nil {
			return nil, err
		}
		return &discord{
			ID:                       guild.ID,
			Name:                     guild.Name,
			ApproximateMemberCount:   guild.ApproximateMemberCount,
			ApproximatePresenceCount: guild.ApproximatePresenceCount}, nil
	})
}

func (c *PublicDataController) NotionDatabase(ctx *gin.Context) {
	databaseId := ctx.Param("id")

	body, err := ctx.GetRawData()
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.ServerError(errors.New("get raw data error detail:"+err.Error())))
		return
	}

	data, err := sdk.NotionDatabase(databaseId, c.Cfg.PublicData.Notion.APIToken, body)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion database error detail:"+err.Error())))
		return
	}

	var databaseData sdk.NotionDatabaseData
	err = json.Unmarshal(data, &databaseData)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion database error detail:"+err.Error())))
		return
	}

	page := api.ParseAndConvertPageParam(ctx)
	// [0, 10)
	start := (page.Page - 1) * page.Size
	end := page.Page * page.Size

	total := len(databaseData.Result)
	if total < start {
		ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
			Page:  page.Page,
			Size:  page.Size,
			Total: int64(total),
			Rows:  []any{},
		}))
		return
	}
	if total < end {
		end = total
	}

	ctx.JSON(http.StatusOK, api.Success(&api.ListReplyData{
		Page:  page.Page,
		Size:  page.Size,
		Total: int64(total),
		Rows:  databaseData.Result[start:end],
	}))
}

func (c *PublicDataController) NotionPage(ctx *gin.Context) {
	pageId := ctx.Param("id")

	data, err := sdk.NotionPage(pageId, c.Cfg.PublicData.Notion.APIToken)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion page error detail:"+err.Error())))
		return
	}

	var pageData map[string]any
	err = json.Unmarshal(data, &pageData)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion page error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(pageData))
}

func (c *PublicDataController) NotionUser(ctx *gin.Context) {
	userId := ctx.Param("id")

	data, err := sdk.NotionUser(userId, c.Cfg.PublicData.Notion.APIToken)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion user error detail:"+err.Error())))
		return
	}

	var pageData map[string]any
	err = json.Unmarshal(data, &pageData)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("notion user error detail:"+err.Error())))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(pageData))
}

func (c *PublicDataController) SafeVault(ctx *gin.Context) {
	cacheLogic[http.Client, Vault](ctx, &vaultCache, c.Cfg.PublicData.CacheInSeconds, func() (*http.Client, error) {
		return http.DefaultClient, nil
	}, func() (*Vault, error) {
		vault := &Vault{}

		wg := sync.WaitGroup{}
		wg.Add(len(c.Cfg.PublicData.SafeVaults))
		for _, vv := range c.Cfg.PublicData.SafeVaults {
			v := vv
			go func() {
				threshold, owners, err := c.PublicDataService.Safe(v.ChainId, v.Wallet, vaultCache.client)
				if err != nil {
					log.Error().Msgf("safe api failed: %s", err.Error())
				}

				fiatTotal, err := c.PublicDataService.SafeBalance(v.ChainId, v.Wallet, vaultCache.client)
				if err != nil {
					log.Error().Msgf("safe balance api failed: %s", err.Error())
				}

				vault.Wallets = append(vault.Wallets, &Wallet{
					ChainId:   v.ChainId,
					Wallet:    v.Wallet,
					FiatTotal: fiatTotal,
					Threshold: threshold,
					Owners:    owners,
				})
				wg.Done()
			}()
		}
		wg.Wait()

		return vault, nil
	})
}

func (c *PublicDataController) NodeSbtCount(ctx *gin.Context) {
	var data []*model.SystemVariable
	err := c.Db.Model(&model.SystemVariable{}).Where("name like ?", "compute_%").Find(&data).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		log.Error().Msgf("node sbt count error: %s", err.Error())
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("node sbt count error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(data))
}
