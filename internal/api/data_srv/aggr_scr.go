package data_srv

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/service"
	"github.com/theseed-labs/os-backend/internal/static_data"
)

const dbQuery = `select season_id,
       target_user_wallet,
       sum(asset_amount) as season_total,
       seasons.name      as season_name,
       seasons.idx       as season_idx
from applications
         join seasons on season_id = seasons.id
where applications.type = 'NEW_REWARD'
  and applications.asset_name = 'SCR'
GROUP by season_id, target_user_wallet`

const MetaforoTotalCreditRatio = "0.05"

// AggregatedSeasonCredit saves scores aggregated by seasons
type AggregatedSeasonCredit struct {
	SeasonId         uint
	TargetUserWallet string
	SeasonTotal      decimal.Decimal
	SeasonName       string
	SeasonIdx        uint
}

type UserCreditRecord struct {
	TargetUserWallet string

	SeasonsCredit map[uint]AggregatedSeasonCredit

	SeedCount int

	MetaforoVoteCount        int
	MetaforoVoteRewardCredit decimal.Decimal

	CurrentSeasonCredit       decimal.Decimal // Credits should be issued in current season
	WeightedPastSeasonsCredit decimal.Decimal // Weighted past seasons credit
	TotalSeasonCredit         decimal.Decimal // Sum of all seasons credit
}

// NodeCalcResponse saves result of credit calculations, here are the formula
// * SeasonsCredit: Individual credit record for each season
// * SeasonTotalCredit: Sum all seasons credit directly
// * MetaforoCredit: Credits gained by Metaforo vote, metaforoVoteUnit * voteCount
// * ActivityCredit: Current season credit plus metaforo vote credit
// * EffectiveCredit: Activity credit plus weighted post seasons credits
// * SeedCount: Seed count fetched from indexer API
type NodeCalcResponse struct {
	Wallet            string                 `json:"wallet"`
	SeasonsCredit     []SeasonCreditResponse `json:"seasons_credit"`
	SeasonTotalCredit string                 `json:"season_total_credit"`
	ActivityCredit    string                 `json:"activity_credit"`
	MetaforoCredit    string                 `json:"metaforo_credit"`
	SeedCount         int                    `json:"seed_count"`
	EffectiveCredit   string                 `json:"effective_credit"`
}

type SeasonCreditResponse struct {
	SeasonIdx  uint   `json:"season_idx"`
	SeasonName string `json:"season_name"`
	Total      string `json:"total"`
}

func getSeedHolderData(endTs int64) map[string]int {
	indexerClient := sdk.GetIndexerClient()
	seedHolderData, err := indexerClient.GetSeedHolderInfo(endTs)
	if err != nil {
		log.Error().Msgf("query seed holder data error: %+v", err)
		return nil
	}

	seedCount := make(map[string]int)

	for _, holderInfo := range seedHolderData {
		model.SetDefaultMapValue(seedCount, model.FormatUserWallet(holderInfo.Owner), 0)
		seedCount[strings.ToLower(holderInfo.Owner)] += 1
	}

	return seedCount
}

func seasonCreditWeight(seasonIdx, currSeasonIdx uint) decimal.Decimal {
	return decimal.NewFromInt(1).Div(decimal.NewFromInt(2).Pow(decimal.NewFromInt(int64(currSeasonIdx - seasonIdx))))
}

// AggrScr returns aggregated credit score and node calculation result
//
//	@router		/data_srv/aggr_scr [get]
//	@summary	returns aggregated credit score and node calculation result
//	@success	200	{object}	[]NodeCalcResponse
func AggrScr(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	// Fetch current season data from database
	currentSeason, err := service.GetCurrentSeason(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	// Get seed holder count via indexer API before current season end timestamp
	// TODO: Confirm whether the timestamp is season end ts or some other timesamp
	seedHolderCount := getSeedHolderData(currentSeason.EndAt)

	// Result for db sql query, which are grouped query
	var aggregatedSeasonCredits []AggregatedSeasonCredit
	db.Raw(dbQuery).Find(&aggregatedSeasonCredits)

	// userCredits saves all credits by user wallet
	userCredits := make(map[string]UserCreditRecord)

	// totalCreditInCurrentSeason saves total reward credits will be issued in current season,
	// which will be used to calculate reward for each metaforo vote
	totalCreditInCurrentSeason := decimal.Zero

	// Category the records with user wallet, and calculate season total credits

	// Load metaforo vote count
	metaforoVoteCount := make(map[string]int)

	switch currentSeason.Idx {
	case 3:
		metaforoVoteCount = static_data.MetaforoVoteCountS3
		break
	case 4:
		metaforoVoteCount = static_data.MetaforoVoteCountS4
	default:
		log.Warn().Msgf("no meatforo voting data for season %d", currentSeason.Idx)
	}

	// Calculate total credit for current season and calculate metaforo vote reward unit

	// Main loop over aggregated records, which contains those logics:
	// * Category data with user wallet and fill data to UserCreditRecord
	// * Calculate total seasons credits
	for _, r := range aggregatedSeasonCredits {
		wallet := model.FormatUserWallet(r.TargetUserWallet)
		model.SetDefaultMapValue(userCredits, wallet, UserCreditRecord{
			TargetUserWallet:          wallet,
			SeedCount:                 model.GetMapValueOrDefault(seedHolderCount, wallet, 0),
			MetaforoVoteCount:         0,
			MetaforoVoteRewardCredit:  decimal.Zero,
			CurrentSeasonCredit:       decimal.Zero,
			WeightedPastSeasonsCredit: decimal.Zero,
			TotalSeasonCredit:         decimal.Zero,
		})

		// Do not handle seasons data further than current season
		if r.SeasonIdx > currentSeason.Idx {
			log.Warn().Msgf("season in record is %d, current season is %d, ignore the future records", r.SeasonName, currentSeason.Name)
			continue
		}

		if creditRcd, ok := userCredits[wallet]; ok {
			// Save season record
			if creditRcd.SeasonsCredit == nil {
				creditRcd.SeasonsCredit = make(map[uint]AggregatedSeasonCredit)
			}
			creditRcd.SeasonsCredit[r.SeasonIdx] = r

			// Sum total credits for current seasons, and set current season credit to user
			if r.SeasonIdx == currentSeason.Idx {
				totalCreditInCurrentSeason = totalCreditInCurrentSeason.Add(r.SeasonTotal)
				creditRcd.CurrentSeasonCredit = r.SeasonTotal
				creditRcd.TotalSeasonCredit = creditRcd.TotalSeasonCredit.Add(r.SeasonTotal)
			}

			// Add weighted season credit
			if r.SeasonIdx != currentSeason.Idx {
				creditRcd.WeightedPastSeasonsCredit = creditRcd.WeightedPastSeasonsCredit.Add(r.SeasonTotal.Mul(seasonCreditWeight(r.SeasonIdx, currentSeason.Idx)))
			}

			userCredits[wallet] = creditRcd
		}
		log.Error().Msgf("TTT: data: %+v", userCredits[wallet])
	}

	// Calculate total metaforo votes
	totalMetaforoVotes := lo.Sum(lo.Values(metaforoVoteCount))

	// TotalCurrentSeasonCredit * MetaforoCreditRatio / TotalMetaforoActions
	metaforoVoteRewardUnit := totalCreditInCurrentSeason.Mul(decimal.RequireFromString(MetaforoTotalCreditRatio)).Div(decimal.NewFromInt(int64(totalMetaforoVotes)))

	log.Debug().Msgf("Total vote count for season %d is %d, and vote rewards unit is %s", currentSeason.Idx, totalMetaforoVotes, metaforoVoteRewardUnit.String())

	var resp []NodeCalcResponse

	for wallet, record := range userCredits {
		seasonsCredit := lo.MapToSlice(record.SeasonsCredit, func(seasonIdx uint, aggrSeasonCredit AggregatedSeasonCredit) SeasonCreditResponse {
			return SeasonCreditResponse{
				SeasonIdx:  seasonIdx,
				SeasonName: aggrSeasonCredit.SeasonName,
				Total:      aggrSeasonCredit.SeasonTotal.String(),
			}
		})

		userMetaforoVoteCount := model.GetMapValueOrDefault[string, int](metaforoVoteCount, wallet, 0)
		metaforoVoteReward := metaforoVoteRewardUnit.Mul(decimal.NewFromInt(int64(userMetaforoVoteCount)))

		//		lo.Map(lo.Values[int, AggregatedSeasonCredit](record.SeasonsCredit), )
		resp = append(resp, NodeCalcResponse{
			Wallet:            wallet,
			SeasonsCredit:     seasonsCredit,
			SeasonTotalCredit: record.TotalSeasonCredit.String(),
			ActivityCredit:    record.CurrentSeasonCredit.Add(metaforoVoteReward).String(),
			MetaforoCredit:    metaforoVoteReward.String(),
			SeedCount:         record.SeedCount,
			EffectiveCredit:   record.CurrentSeasonCredit.Add(metaforoVoteReward).Add(record.WeightedPastSeasonsCredit).String(),
		})
	}

	ctx.JSON(http.StatusOK, api.Success(&resp))
}
