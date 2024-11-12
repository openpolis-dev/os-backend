package data_srv

import (
	"bytes"
	"encoding/gob"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

// Pg version
// 20241111: Change filter state to completed only
const dbQueryForSeasonTotalRewards = `select season_id,
       target_user_wallet,
       sum(asset_amount::Decimal(20, 8)) as season_total,
       seasons.name      as season_name,
       seasons.idx       as season_idx
from applications
         join seasons on season_id = seasons.id
where applications.type = 'NEW_REWARD'
  and applications.asset_name = 'SCR'
  and applications.state in ('completed')
  and applications.sub_type IN (NULL ,'')
GROUP by season_id, target_user_wallet, seasons.name, season_idx`

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

type NodeCalcResponse struct {
	// Summarized credits data
	SeasonName                   string `json:"season_name"`
	SeasonTotalCreditWithoutMint string `json:"season_total_credit_without_mint"`
	SeasonTotalMintCredit        string `json:"season_total_mint_credit"`
	TotalWalletCount             int    `json:"total_wallet_count"`
	ActivateWalletCount          int    `json:"activate_wallet_count"`

	// Some flags
	MintRewardConfirmed bool `json:"metaforo_confirmed"`
	SeedSnapshoted      bool `json:"seed_snapshoted"`

	// Credit detail records
	Records []*CreditDetail `json:"records"`
}

// CreditDetail saves result of credit calculations, here are the formula
// * SeasonsCredit: Individual credit record for each season
// * SeasonTotalCredit: Sum all seasons credit directly
// * MetaforoCredit: Credits gained by Metaforo vote, metaforoVoteUnit * voteCount
// * ActivityCredit: Current season credit plus metaforo vote credit
// * EffectiveCredit: Activity credit plus weighted post seasons credits
// * SeedCount: Seed count fetched from indexer API
type CreditDetail struct {
	Wallet            string                 `json:"wallet"`
	SeasonsCredit     []SeasonCreditResponse `json:"seasons_credit"`
	SeasonTotalCredit string                 `json:"season_total_credit"`
	ActivityCredit    string                 `json:"activity_credit"`
	MetaforoVoteCount int                    `json:"metaforo_vote_count"`
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
		seedCount[common.FormatUserWallet(holderInfo.Wallet)] += len(holderInfo.Ids)
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
//	@tags		DataService
//	@success	200	{object}	[]CreditDetail
func AggrScr(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	// Fetch current season data from database
	currentSeason, err := model.GetCurrentSeason(db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season error")))
		return
	}
	log.Debug().Msgf("current season: %+v", currentSeason)

	mintResult, err := CalcMintRewards(ctx, db, currentSeason)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("calc mint rewards error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&NodeCalcResponse{
		SeasonName:                   currentSeason.Name,
		SeasonTotalCreditWithoutMint: mintResult.TotalSeasonCreditWithoutMint.String(),
		SeasonTotalMintCredit:        mintResult.TotalMetaforoCredits.String(),
		TotalWalletCount:             len(mintResult.UserCredits),
		ActivateWalletCount:          mintResult.ActivateWalletCount,
		MintRewardConfirmed:          currentSeason.MintRewardConfirmed,
		SeedSnapshoted:               currentSeason.SeedSnapshotSaved,
		Records:                      mintResult.DetailRecords,
	}))
}

func CalcMintRewards(ctx *gin.Context, db *gorm.DB, currentSeason *model.Season) (mintResult *MintResult, err error) {
	mintResult = &MintResult{
		TotalSeasonCreditWithoutMint: decimal.Zero,
		TotalMetaforoCredits:         decimal.Zero,
		UserCredits:                  make(map[string]UserCreditRecord),
		ActivateWalletCount:          0,
		DetailRecords:                []*CreditDetail{},
	}
	// Result for db sql query, which are grouped query
	var aggregatedSeasonCredits []AggregatedSeasonCredit
	if err = db.Raw(dbQueryForSeasonTotalRewards).Find(&aggregatedSeasonCredits).Error; err != nil {
		log.Error().Msgf("query aggregated credit score error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query aggregated credit score error")))
		return
	}

	// userCredits saves all credits by user wallet
	mintResult.UserCredits = make(map[string]UserCreditRecord)

	// totalSeasonCreditWithoutMint saves total credits (without mint) will be issued in current season,
	// which will be used to calculate reward for each metaforo vote
	mintResult.TotalSeasonCreditWithoutMint = decimal.Zero

	// Category the records with user wallet, and calculate season total credits

	// Load metaforo vote count
	metaforoVoteCount, err := GetSeasonVoteRecords(db, currentSeason)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get metaforo vote count error")))
		return
	}

	mintResult.ActivateWalletCount = 0

	// If season has been snapshoted, get snapshot data with timestamp saved in DB,
	// otherwise the season end timestamp will be used for event end data
	// After getting the timestamp, invoke Indexer service to get seed count
	var seedHolderCount map[string]int
	if currentSeason.SeedSnapshotSaved {
		seedHolderCount = getSeedHolderData(currentSeason.SeedSnapshotAt)
	} else {
		seedHolderCount = getSeedHolderData(currentSeason.EndAt)
	}

	// Calculate total credit for current season and calculate metaforo vote reward unit

	// Main loop over aggregated records, which contains those logics:
	// * Category data with user wallet and fill data to UserCreditRecord
	// * Calculate total seasons credits
	for _, r := range aggregatedSeasonCredits {
		wallet := common.FormatUserWallet(r.TargetUserWallet)
		model.SetDefaultMapValue(mintResult.UserCredits, wallet, UserCreditRecord{
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
			log.Warn().Msgf("season in record is %s, current season is %s, ignore the future records", r.SeasonName, currentSeason.Name)
			continue
		}

		if creditRcd, ok := mintResult.UserCredits[wallet]; ok {
			// Save season record
			if creditRcd.SeasonsCredit == nil {
				creditRcd.SeasonsCredit = make(map[uint]AggregatedSeasonCredit)
			}
			creditRcd.SeasonsCredit[r.SeasonIdx] = r
			creditRcd.TotalSeasonCredit = creditRcd.TotalSeasonCredit.Add(r.SeasonTotal)

			// Sum total credits for current seasons, and set current season credit to user
			if r.SeasonIdx == currentSeason.Idx {
				mintResult.TotalSeasonCreditWithoutMint = mintResult.TotalSeasonCreditWithoutMint.Add(r.SeasonTotal)
				creditRcd.CurrentSeasonCredit = r.SeasonTotal
				mintResult.ActivateWalletCount += 1
			}

			// Add weighted season credit
			if r.SeasonIdx != currentSeason.Idx {
				creditRcd.WeightedPastSeasonsCredit = creditRcd.WeightedPastSeasonsCredit.Add(r.SeasonTotal.Mul(seasonCreditWeight(r.SeasonIdx, currentSeason.Idx)))
			}

			mintResult.UserCredits[wallet] = creditRcd
		}
	}

	// Calculate total metaforo votes
	totalMetaforoVotes := lo.Sum(lo.Values(metaforoVoteCount))

	// TotalCurrentSeasonCredit * MetaforoCreditRatio / TotalMetaforoActions
	mintResult.TotalMetaforoCredits = mintResult.TotalSeasonCreditWithoutMint.Mul(decimal.RequireFromString(MetaforoTotalCreditRatio))
	metaforoVoteRewardUnit := decimal.Zero
	if totalMetaforoVotes > 0 {
		metaforoVoteRewardUnit = mintResult.TotalMetaforoCredits.Div(decimal.NewFromInt(int64(totalMetaforoVotes)))
	}

	mintResult.MintRewardData = make(map[string]string)

	for wallet, record := range mintResult.UserCredits {
		seasonsCredit := lo.MapToSlice(record.SeasonsCredit, func(seasonIdx uint, aggrSeasonCredit AggregatedSeasonCredit) SeasonCreditResponse {
			return SeasonCreditResponse{
				SeasonIdx:  seasonIdx,
				SeasonName: aggrSeasonCredit.SeasonName,
				Total:      aggrSeasonCredit.SeasonTotal.String(),
			}
		})

		userMetaforoVoteCount := model.GetMapValueOrDefault[string, int](metaforoVoteCount, wallet, 0)
		metaforoVoteReward := metaforoVoteRewardUnit.Mul(decimal.NewFromInt(int64(userMetaforoVoteCount)))
		if !metaforoVoteReward.Equal(decimal.Zero) {
			mintResult.MintRewardData[wallet] = metaforoVoteReward.String()
		}

		mintResult.DetailRecords = append(mintResult.DetailRecords, &CreditDetail{
			Wallet:            wallet,
			SeasonsCredit:     seasonsCredit,
			SeasonTotalCredit: record.TotalSeasonCredit.Add(metaforoVoteReward).String(),
			ActivityCredit:    record.CurrentSeasonCredit.Add(metaforoVoteReward).String(),
			MetaforoVoteCount: userMetaforoVoteCount,
			MetaforoCredit:    metaforoVoteReward.String(),
			SeedCount:         record.SeedCount,
			EffectiveCredit:   record.CurrentSeasonCredit.Add(metaforoVoteReward).Add(record.WeightedPastSeasonsCredit).String(),
		})
	}

	// Buffer for metaforo data
	var buffer bytes.Buffer
	bufEncoder := gob.NewEncoder(&buffer)
	err = bufEncoder.Encode(mintResult.MintRewardData)
	log.Error().Msgf("TTT: Write buf size: %d", buffer.Len())
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("encode metaforo data error")))
		return
	}
	err = storage.StoreCachedData(storage.MetaforoRewardCacheKey(currentSeason.Idx), buffer.Bytes())
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("store metaforo data error")))
		return
	}

	return mintResult, nil
}
