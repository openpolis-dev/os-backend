package data_srv

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/service"
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

	SeasonsCredit  map[uint]AggregatedSeasonCredit
	MetaforoCredit decimal.Decimal

	SeedCount uint

	ActivityCredit  decimal.Decimal // credits should be issued in current season, plus metaforo credit
	EffectiveCredit decimal.Decimal // activity credit plus weighted pre-seasons credit
}

type SeasonCreditResponse struct {
	SeasonIdx  uint   `json:"season_idx"`
	SeasonName string `json:"season_name"`
	Total      string `json:"total"`
}
type NodeCalcResponse struct {
	Wallet          string                 `json:"wallet"`
	SeasonsCredit   []SeasonCreditResponse `json:"seasons_credit"`
	ActivityCredit  string                 `json:"activity_credit"`
	MetaforoCredit  string                 `json:"metaforo_credit"`
	SeedCount       uint                   `json:"seed_count"`
	EffectiveCredit string                 `json:"effective_credit"`
}

// AggrScr returns aggregated credit score and node calculation result
//
//	@router		/data_srv/aggr_scr [get]
//	@summary	returns aggregated credit score and node calculation result
//	@success	200	{object}	[]NodeCalcResponse
func AggrScr(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)
	var aggregatedSeasonCredits []AggregatedSeasonCredit
	db.Raw(dbQuery).Find(&aggregatedSeasonCredits)

	// userCredits category all credits by user wallet
	userCredits := make(map[string]UserCreditRecord)

	currentSeason, err := service.GetCurrentSeason(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	totalCreditInCurrentSeason := decimal.Zero

	// TODO: Get seed count for each wallet with specified time
	for _, r := range aggregatedSeasonCredits {
		model.SetDefaultMapValue(userCredits, r.TargetUserWallet, UserCreditRecord{
			TargetUserWallet: r.TargetUserWallet,
			SeasonsCredit:    make(map[uint]AggregatedSeasonCredit),
			MetaforoCredit:   decimal.Zero,
			SeedCount:        0,
			ActivityCredit:   decimal.Zero,
			EffectiveCredit:  decimal.Zero,
		})

		userCredits[r.TargetUserWallet].SeasonsCredit[r.SeasonIdx] = r
		if r.SeasonIdx == currentSeason.Idx {
			totalCreditInCurrentSeason = totalCreditInCurrentSeason.Add(r.SeasonTotal)
		}
	}

	log.Error().Msgf("Current season %s total: %s", currentSeason.Name, totalCreditInCurrentSeason)

	var resp []NodeCalcResponse

	// TODO: metaforo credit is not populated yet
	for wallet, record := range userCredits {
		var respSeasonsCredit []SeasonCreditResponse
		for seasonIdx, seasonCredit := range record.SeasonsCredit {
			if seasonIdx > currentSeason.Idx {
				continue
			}

			if seasonIdx == currentSeason.Idx {
				record.ActivityCredit = record.MetaforoCredit.Add(seasonCredit.SeasonTotal)
			}

			record.EffectiveCredit = record.EffectiveCredit.Add(seasonCredit.SeasonTotal.Div(decimal.NewFromInt(2).Pow(decimal.NewFromInt(int64(currentSeason.Idx - seasonIdx)))))
			respSeasonsCredit = append(respSeasonsCredit, SeasonCreditResponse{
				SeasonIdx:  seasonCredit.SeasonIdx,
				SeasonName: seasonCredit.SeasonName,
				Total:      seasonCredit.SeasonTotal.String(),
			})
		}

		record.EffectiveCredit = record.EffectiveCredit.Add(record.ActivityCredit)

		resp = append(resp, NodeCalcResponse{
			Wallet:          wallet,
			SeasonsCredit:   respSeasonsCredit,
			ActivityCredit:  record.ActivityCredit.String(),
			MetaforoCredit:  record.MetaforoCredit.String(),
			SeedCount:       record.SeedCount,
			EffectiveCredit: record.EffectiveCredit.String(),
		})
	}

	ctx.JSON(200, &resp)
}
