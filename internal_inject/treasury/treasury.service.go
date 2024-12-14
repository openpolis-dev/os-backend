package treasury_inject

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type TreasuryService struct {
	// inject

	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (s *TreasuryService) UpdateAssets(ctx *gin.Context) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjTreasury, internal.ActUpdateAssertBudget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("check permission error"))
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjTreasury, internal.ActUpdateAssertBudget)
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	var updateParams []model.UpdateAssetRequestParams
	err = ctx.Bind(&updateParams)
	if err != nil {
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return http.StatusBadRequest, api.BadRequest(err)
	}

	err = s.Db.Transaction(func(tx *gorm.DB) error {
		for _, assetParam := range updateParams {
			err = model.TreasuryAssetHelper.UpsertCurrentSeasonTreasuryDetailedRecord(tx, assetParam.AssetName, assetParam.TotalAmount, common.FormatUserWallet(user.Wallet))
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update assets error")))
	}

	currQuarterTreasuryRecord, err := model.TreasuryAssetHelper.GetOrCreateCurrentSeasonRecord(s.Db)
	// ctx.JSON(http.StatusOK, api.Success(currQuarterTreasuryRecord))

	return http.StatusOK, api.Success(currQuarterTreasuryRecord)
}
