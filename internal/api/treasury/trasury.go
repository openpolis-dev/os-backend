package treasury

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

func GetOrCreateCurrentAssetRecords(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	currQuarterTreasuryRecord, err := model.TreasuryAssetHelper.GetOrCreateCurrentSeasonRecord(db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get or create current quarter treasury record error")))
		return
	}

	treasuryAssetResp, err := currQuarterTreasuryRecord.ToTreasuryAssetsResponse(db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get or create current quarter treasury record error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(treasuryAssetResp))
}

// UpdateAssets updates asset records of current quarter budget
func UpdateAssets(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjTreasury, internal.ActUpdateAssertBudget)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjTreasury, internal.ActUpdateAssertBudget)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	var updateParams []model.UpdateAssetRequestParams
	err = ctx.Bind(&updateParams)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
		return
	}

	err = db.Transaction(func(tx *gorm.DB) error {
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

	currQuarterTreasuryRecord, err := model.TreasuryAssetHelper.GetOrCreateCurrentSeasonRecord(db)
	ctx.JSON(http.StatusOK, api.Success(currQuarterTreasuryRecord))
}

// helper functions
