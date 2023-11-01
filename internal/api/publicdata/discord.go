package publicdata

import (
	"github.com/bwmarrin/discordgo"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
)

type discord struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	ApproximateMemberCount   int    `json:"approximate_member_count"`
	ApproximatePresenceCount int    `json:"approximate_presence_count"`
}

var discordCache dataCache[discordgo.Session, discord]

// DiscordData returns the data of the discord server
// @Summary DiscordData returns the data of the discord server
// @Tags PublicData
// @Accept json
// @Produce json
// @Success 200 {object} discord
// @Router /public_data/discord_member_count [get]
func DiscordData(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	cacheLogic[discordgo.Session, discord](ctx, &discordCache, cfg.PublicData.CacheInSeconds, func() (*discordgo.Session, error) {
		return discordgo.New("Bot " + cfg.PublicData.Discord.Token)
	}, func() (*discord, error) {
		guild, err := discordCache.client.GuildWithCounts(cfg.PublicData.Discord.GuildID)
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
