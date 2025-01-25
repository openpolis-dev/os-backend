package publicdata_inject

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/facebookgo/inject"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/theseed-labs/os-backend/global_object"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type PublicDataController struct {
	// inject

	Gin *gin.Engine `inject:""`

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`

	PublicDataService *PublicDataService `inject:""`
}

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
