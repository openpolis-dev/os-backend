package datasrv_inject

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/api/proposal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

type DataSrvService struct {
	// inject
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

// GetSeasonVoteRecords returns a map of user wallet to their vote count of current season
// The source data is gathered from proposal_user_vote_records table
func (s *DataSrvService) GetSeasonVoteRecords(seasonRcd *model.Season) (map[string]int, error) {
	var currentSeasonMintUserVoteRecords []*model.ProposalUserVoteRecord
	err := s.Db.Raw(getCurrentSeasonMintableUserVoteRecordsQuery, seasonRcd.ID).Find(&currentSeasonMintUserVoteRecords).Error
	if err != nil {
		log.Error().Msgf("get current season mintable user vote records error: %+v", err)
		return nil, err
	}

	metaforoUserVoteCount := make(map[string]int)
	for _, record := range currentSeasonMintUserVoteRecords {
		metaforoUserVoteCount[common.FormatUserWallet(record.UserWallet)] += 1
	}
	return metaforoUserVoteCount, nil
}

func (s *DataSrvService) FetchSingleProposalUserVoteRecord(ctx *gin.Context) {
	proposalId, err := strconv.ParseUint(ctx.Param("proposal_id"), 10, 64)
	if err != nil {
		log.Error().Msgf("parse proposal id error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse proposal id error: %+v", err)))
		return
	}

	err = proposal.UpdateUserVoteRecordViaMetaforo(s.Db, s.Cfg.MetaforoData.GroupName, uint(proposalId))
	if err != nil {
		log.Error().Msgf("update user vote record via metaforo error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.BadRequest(fmt.Errorf("update user vote record via metaforo error: %+v", err)))
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *DataSrvService) GetSeedHolderData(endTs int64) map[string]int {
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

func (s *DataSrvService) SeasonCreditWeight(seasonIdx, currSeasonIdx uint) decimal.Decimal {
	return decimal.NewFromInt(1).Div(decimal.NewFromInt(2).Pow(decimal.NewFromInt(int64(currSeasonIdx - seasonIdx))))
}

func (s *DataSrvService) CalcMintRewards(ctx *gin.Context, currentSeason *model.Season) (mintResult *MintResult, err error) {
	mintResult = &MintResult{
		TotalSeasonCreditWithoutMint: decimal.Zero,
		TotalMetaforoCredits:         decimal.Zero,
		UserCredits:                  make(map[string]UserCreditRecord),
		ActivateWalletCount:          0,
		DetailRecords:                []*CreditDetail{},
	}
	// Result for db sql query, which are grouped query
	var aggregatedSeasonCredits []AggregatedSeasonCredit
	if err = s.Db.Raw(dbQueryForSeasonTotalRewards).Find(&aggregatedSeasonCredits).Error; err != nil {
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
	metaforoVoteCount, err := s.GetSeasonVoteRecords(currentSeason)
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
		seedHolderCount = s.GetSeedHolderData(currentSeason.SeedSnapshotAt)
	} else {
		seedHolderCount = s.GetSeedHolderData(currentSeason.EndAt)
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
				creditRcd.WeightedPastSeasonsCredit = creditRcd.WeightedPastSeasonsCredit.Add(r.SeasonTotal.Mul(s.SeasonCreditWeight(r.SeasonIdx, currentSeason.Idx)))
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
