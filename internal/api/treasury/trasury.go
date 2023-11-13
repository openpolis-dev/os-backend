package treasury

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

func GetOrCreateCurrentAssetRecords(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	currQuarterTreasuryRecord, err := model.TreasuryAssetHelper.GetOrCreateCurrentSeasonRecord(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	treasuryAssetResp, err := currQuarterTreasuryRecord.ToTreasuryAssetsResponse(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(treasuryAssetResp))
}

// UpdateAssets updates asset records of current quarter budget
func UpdateAssets(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)
	//  check permission
	ok, err := enforcer.Enforce(user.Wallet, api.ObjTreasury, api.ActUpdateAssertBudget)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
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
			err = model.TreasuryAssetHelper.UpsertCurrentSeasonTreasuryDetailedRecord(tx, assetParam.AssetName, assetParam.TotalAmount, user.Wallet)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
	}

	currQuarterTreasuryRecord, err := model.TreasuryAssetHelper.GetOrCreateCurrentSeasonRecord(db)
	ctx.JSON(http.StatusOK, api.Success(currQuarterTreasuryRecord))
}

// helper functions
