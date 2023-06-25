package treasury

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

// TODO: How to get passed in token name and budget type?

func GetOrCreateCurrentBudget(ctx *gin.Context) {
	db := api.ForContextOnlyDB(ctx)

	currQuarterTreasuryRecord, err := model.TreasuryAssetHelper.GetOrCreateCurrQuarterRecord(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(currQuarterTreasuryRecord))
}

// UpdateAssets updates asset records of current quarter budget
func UpdateAssets(ctx *gin.Context) {
	// TODO: Validate user permission
	//user, enforcer, db, _ := api.ForContext(ctx)
	user, _, db, _ := api.ForContext(ctx)

	var updateParams []model.UpdateAssetRequestParams
	err := ctx.Bind(&updateParams)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, assetParam := range updateParams {
			err = model.TreasuryAssetHelper.UpsertCQTreasuryDetailedRecord(tx, assetParam.BudgetType, assetParam.AssetName, assetParam.TotalAmount, user.Wallet)
			if err != nil {
				return err
			}
		}
		return nil
	})

	currQuarterTreasuryRecord, err := model.TreasuryAssetHelper.GetOrCreateCurrQuarterRecord(db)
	ctx.JSON(http.StatusOK, api.Success(currQuarterTreasuryRecord))
}

// helper functions
