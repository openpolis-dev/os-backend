package publicdata

import (
	"net/http"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/bwmarrin/discordgo"
	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
)

type discordMemberCountReply struct {
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	ApproximateMemberCount   int    `json:"approximate_member_count"`
	ApproximatePresenceCount int    `json:"approximate_presence_count"`
}

// DiscordMemberCount returns the number of members in the discord server
// `/public_data/discord_member_count`
func DiscordMemberCount(ctx *gin.Context) {
	_, cfg := api.ForContextDBAndConfig(ctx)

	// get Mutex lock
	dataWrapper.lock.Lock()
	// unlock Mutex
	defer dataWrapper.lock.Unlock()

	var err error
	if dataWrapper.discord == nil {
		log.Debug().Msgf("creating discord instance...")

		dataWrapper.discord, err = discordgo.New("Bot " + cfg.PublicData.Discord.Token)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	if time.Now().Unix()-dataWrapper.updateTime > cfg.PublicData.Discord.CacheInSeconds {
		log.Debug().Msgf("querying guild info...")

		guild, err := dataWrapper.discord.Guild(cfg.PublicData.Discord.GuildID)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
		log.Debug().Msgf("%+v", dataWrapper.data)

		dataWrapper.data = &discordMemberCountReply{
			ID:                       guild.ID,
			Name:                     guild.Name,
			ApproximateMemberCount:   guild.ApproximateMemberCount,
			ApproximatePresenceCount: guild.ApproximatePresenceCount,
		}
		dataWrapper.updateTime = time.Now().Unix()
	}

	ctx.JSON(http.StatusOK, api.Success(dataWrapper.data))
}

type discordData struct {
	discord    *discordgo.Session
	data       *discordMemberCountReply
	updateTime int64 // unit is seconds
	lock       sync.Mutex
}

var dataWrapper discordData
