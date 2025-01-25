package appbundles_inject

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type AppBundlesService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (s *AppBundlesService) CreateAppBundle(ctx *gin.Context, newAppBundleReq *model.NewAppBundleRequest) (int, *api.Reply) {
	user, enforcer, _, _ := api.ForContext(ctx)
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return http.StatusBadRequest, api.BadRequest(errors.New("check permission error detail:" + err.Error()))
	}

	isCityHallProject := false

	if !ok {
		log.Debug().Msgf("user %s has no hall permission, check whether user is sponsor", user.Wallet)

		// 2024.4.10
		// casbin based check has got some problems and have no time to find solution,
		// so change to a simple check of sponsor address stored in entity record
		//// Check permission, using ActCreateApplication for now, can be changed to new permission if required
		//obj := lo.
		//	If(newAppBundleReq.Entity == "project", fmt.Sprintf("%s%d", internal.ObjProjPrefix, newAppBundleReq.EntityId)).
		//	ElseIf(newAppBundleReq.Entity == "guild", fmt.Sprintf("%s%d", internal.ObjGuildPrefix, newAppBundleReq.EntityId)).
		//	Else("")
		//ok, err = enforcer.Enforce(common.FormatUserWallet(user.Wallet), obj, internal.ActCreateApplication)
		//if err != nil {
		//	sdk.LogServerErrorToSentry(ctx, err)
		//	ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		//	return
		//}
		//if !ok {
		//	sdk.LogForbiddenError(ctx, user.Wallet, obj, internal.ActCreateApplication)
		//	ctx.JSON(http.StatusForbidden, api.Forbidden())
		//	return
		//}

		// Verify whether user is in sponsor list
		var sponsorsList []string
		switch newAppBundleReq.Entity {
		case "project":
			var projectRecord *model.Project
			err = s.Db.Model(&model.Project{}).Where("id = ?", newAppBundleReq.EntityId).First(&projectRecord).Error
			if err != nil {
				log.Error().Msgf("check permission for project %d error: %+v", newAppBundleReq.EntityId, err)
				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
				return http.StatusBadRequest, api.BadRequest(errors.New("check permission error detail:" + err.Error()))
			}
			if projectRecord.IsSpecial && projectRecord.SpecialType == model.SpecialProjectCityHall {
				isCityHallProject = true
			}
			sponsorsList = projectRecord.Sponsors
		case "guild":
			var guildRecord *model.Guild
			err = s.Db.Model(&model.Guild{}).Where("id = ?", newAppBundleReq.EntityId).First(&guildRecord).Error
			if err != nil {
				log.Error().Msgf("check permission for guild %d error: %+v", newAppBundleReq.EntityId, err)
				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
				return http.StatusBadRequest, api.BadRequest(errors.New("check permission error detail:" + err.Error()))
			}
			sponsorsList = guildRecord.Sponsors
		default:
			sdk.LogUserSideError(ctx, errors.New("invalid entity type"))
			ctx.JSON(http.StatusBadRequest, api.BadRequest(errors.New("invalid entity type")))
		}

		if lo.ContainsBy(sponsorsList, func(sponsorWallet string) bool {
			return strings.EqualFold(sponsorWallet, user.Wallet)
		}) {
			log.Debug().Msgf("user %s is sponsor of %s_%d, can create app bundle", user.Wallet, newAppBundleReq.Entity, newAppBundleReq.EntityId)
		} else {
			log.Error().Msgf("user %s is not sponsor, cannot create app bundle", user.Wallet)
			sdk.LogUserSideError(ctx, errors.New("user is not sponsor"))
			// ctx.JSON(http.StatusForbidden, api.Forbidden())
			return http.StatusForbidden, api.Forbidden()
		}
	} else {
		log.Debug().Msgf("user %s has hall permission, can create app bundle", user.Wallet)
	}

	seasonRecord, err := model.GetCurrentSeason(s.Db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("get current season error"))
	}

	// Variable to save used asset amount for the project if this app bundle created successfully, which will be used to update project budget records
	assetAmountUsedInThisRequest := make(map[string]decimal.Decimal)

	// Validate project budgets
	if isCityHallProject {
		log.Debug().Msgf("project %d is city hall project, ignore checking of budget", newAppBundleReq.EntityId)
	} else if newAppBundleReq.Entity == "project" {
		projectBudgets, err := model.ProjectBudgetModel.ListByProjectId(s.Db, newAppBundleReq.EntityId)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get project budgets error")))
			return http.StatusInternalServerError, api.ServerError(errors.New("get project budgets"))
		}

		if len(projectBudgets) == 0 {
			log.Debug().Msgf("project %d has no budget record, ignore checking of amount submitted", newAppBundleReq.EntityId)
		} else {
			// Only advance remain amount can be applied here
			advanceRemainAmountRecord := lo.SliceToMap(projectBudgets, func(budget *model.ProjectBudget) (string, decimal.Decimal) {
				return budget.AssetName, budget.RemainAdvanceAmount
			})

			requestAssetAmount := make(map[string]decimal.Decimal)

			for _, appRecord := range newAppBundleReq.Records {
				remainAdvanceAmount, found := advanceRemainAmountRecord[appRecord.AssetName]
				if !found {
					err := fmt.Errorf("incorrect asset name %s for project %d", appRecord.AssetName, newAppBundleReq.EntityId)
					log.Error().Msg(err.Error())
					sdk.LogUserSideError(ctx, err)
					// ctx.JSON(http.StatusBadRequest, api.ServerError(err))
					return http.StatusBadRequest, api.ServerError(err)
				}

				if remainAdvanceAmount.LessThan(appRecord.Amount) {
					err := fmt.Errorf("project %d has insufficient advance amount for asset %s", newAppBundleReq.EntityId, appRecord.AssetName)
					log.Error().Msg(err.Error())
					sdk.LogUserSideError(ctx, err)
					// ctx.JSON(http.StatusBadRequest, api.ServerError(err))
					return http.StatusBadRequest, api.ServerError(err)
				}

				// Sum total amount for each asset
				if _, found := requestAssetAmount[appRecord.AssetName]; !found {
					requestAssetAmount[appRecord.AssetName] = appRecord.Amount
				} else {
					requestAssetAmount[appRecord.AssetName] = requestAssetAmount[appRecord.AssetName].Add(appRecord.Amount)
				}
			}

			// Check total amount
			for assetName, amount := range requestAssetAmount {
				if amount.GreaterThan(advanceRemainAmountRecord[assetName]) {
					err := fmt.Errorf("project %d has insufficient advance amount for asset %s", newAppBundleReq.EntityId, assetName)
					log.Error().Msg(err.Error())
					sdk.LogUserSideError(ctx, err)
					// ctx.JSON(http.StatusBadRequest, api.ServerError(err))
					return http.StatusBadRequest, api.ServerError(err)
				} else {
					// Save asset amount will be used.
					assetAmountUsedInThisRequest[assetName] = amount
				}
			}
		}
	} else if newAppBundleReq.Entity == "guild" {
		guildBudgets, err := model.GuildBudgetModel.ListByGuildId(s.Db, newAppBundleReq.EntityId)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get guild budgets error")))
			return http.StatusInternalServerError, api.ServerError(errors.New("get guild budgets error"))
		}

		if len(guildBudgets) == 0 {
			log.Debug().Msgf("guild %d has no budget record, ignore checking of amount submitted", newAppBundleReq.EntityId)
		} else {
			// Only advance remain amount can be applied here
			guildRemainBudgets := lo.SliceToMap(guildBudgets, func(budget *model.GuildBudget) (string, decimal.Decimal) {
				return budget.AssetName, budget.RemainAmount
			})

			requestAssetAmount := make(map[string]decimal.Decimal)

			for _, appRecord := range newAppBundleReq.Records {
				remainAmount, found := guildRemainBudgets[appRecord.AssetName]
				if !found {
					err := fmt.Errorf("incorrect asset name %s for project %d", appRecord.AssetName, newAppBundleReq.EntityId)
					log.Error().Msg(err.Error())
					sdk.LogUserSideError(ctx, err)
					// ctx.JSON(http.StatusBadRequest, api.ServerError(err))
					return http.StatusBadRequest, api.ServerError(err)
				}

				if remainAmount.LessThan(appRecord.Amount) {
					err := fmt.Errorf("project %d has insufficient advance amount for asset %s", newAppBundleReq.EntityId, appRecord.AssetName)
					log.Error().Msg(err.Error())
					sdk.LogUserSideError(ctx, err)
					// ctx.JSON(http.StatusBadRequest, api.ServerError(err))
					return http.StatusBadRequest, api.ServerError(err)
				}

				// Sum total amount for each asset
				if _, found := requestAssetAmount[appRecord.AssetName]; !found {
					requestAssetAmount[appRecord.AssetName] = appRecord.Amount
				} else {
					requestAssetAmount[appRecord.AssetName] = requestAssetAmount[appRecord.AssetName].Add(appRecord.Amount)
				}
			}

			// Check total amount
			for assetName, amount := range requestAssetAmount {
				if amount.GreaterThan(guildRemainBudgets[assetName]) {
					err := fmt.Errorf("project %d has insufficient advance amount for asset %s", newAppBundleReq.EntityId, assetName)
					log.Error().Msg(err.Error())
					sdk.LogUserSideError(ctx, err)
					// ctx.JSON(http.StatusBadRequest, api.ServerError(err))
					return http.StatusBadRequest, api.ServerError(err)
				} else {
					// Save asset amount will be used.
					assetAmountUsedInThisRequest[assetName] = amount
				}
			}
		}
	}

	// TODO: Duplicated code *NewAppBundleAndApplication*
	appBundle := model.AppBundle{
		Comment:      newAppBundleReq.Comment,
		Applicant:    common.FormatUserWallet(user.Wallet),
		EntityType:   newAppBundleReq.Entity,
		EntityId:     newAppBundleReq.EntityId,
		SeasonId:     seasonRecord.ID,
		Season:       *seasonRecord,
		State:        model.ApplicationStateOpen,
		ShadowRecord: false,
		CreatedAt:    time.Now().In(internal.ProjectTimezone),
		UpdatedAt:    time.Now().In(internal.ProjectTimezone),
		CreateTs:     model.GetCurrentUtcEpochSecond(),
		UpdateTs:     model.GetCurrentUtcEpochSecond(),
		Type:         "NEW_REWARD",
	}

	err = s.Db.Transaction(func(tx *gorm.DB) error {
		err = tx.Model(model.AppBundle{}).Create(&appBundle).Error
		if err != nil {
			log.Error().Msgf("Create app bundle records error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create app bundle record error detail:"+err.Error())))
			return err
		}

		appBundle.AppRecords = lo.Map(newAppBundleReq.Records, func(appRcdRequest *model.NewApplicationRequest, index int) *model.Application {
			formattedTargetWallet := common.FormatUserWallet(appRcdRequest.TargetUserWallet)

			return &model.Application{
				Type:             model.ApplicationNewReward,
				Applicant:        common.FormatUserWallet(user.Wallet),
				State:            model.ApplicationStateOpen,
				CreatedAt:        time.Now().In(internal.ProjectTimezone),
				UpdatedAt:        time.Now().In(internal.ProjectTimezone),
				CreateTs:         model.GetCurrentUtcEpochSecond(),
				UpdateTs:         model.GetCurrentUtcEpochSecond(),
				DetailedType:     appRcdRequest.DetailedType,
				Comment:          appRcdRequest.Comment,
				TargetUserWallet: formattedTargetWallet,
				AssetName:        appRcdRequest.AssetName,
				AssetAmount:      appRcdRequest.Amount,
				EntityType:       newAppBundleReq.Entity,
				EntityId:         newAppBundleReq.EntityId,
				SeasonId:         seasonRecord.ID,
				Season:           seasonRecord,
			}
		})

		err = tx.Save(&appBundle).Error
		if err != nil {
			log.Error().Msgf("update app_bundle record error: %+v", err)
			return err
		}

		// Create application audit logs
		appAuditLogs := lo.Map(appBundle.AppRecords, func(app *model.Application, _ int) *model.ApplicationAuditLog {
			return &model.ApplicationAuditLog{
				ApplicationID: app.ID,
				LogTs:         model.GetCurrentUtcEpochSecond(),
				Operation:     model.AuditActionNew,
				Operator:      common.FormatUserWallet(user.Wallet),
				PreState:      "",
				PostState:     model.ApplicationStateOpen,
			}
		})
		err = tx.Model(model.ApplicationAuditLog{}).Create(&appAuditLogs).Error
		if err != nil {
			log.Error().Msgf("Create application audit log records error: %+v", err)
			return err
		}

		err = tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
			AppBundleId: appBundle.ID,
			AppBundle:   appBundle,
			LogTs:       model.GetCurrentUtcEpochSecond(),
			Operation:   model.AuditActionNew,
			Operator:    common.FormatUserWallet(user.Wallet),
			PreState:    "",
			PostState:   model.ApplicationStateOpen,
			ExtraData:   "",
		}).Error

		if err != nil {
			log.Error().Msgf("Create app bundle audit log records error: %+v", err)
			return err
		}

		if newAppBundleReq.Entity == "project" && !isCityHallProject {
			for assetName, usedAmount := range assetAmountUsedInThisRequest {
				err = model.ProjectBudgetModel.WithdrawSingleAsset(tx, newAppBundleReq.EntityId, assetName, usedAmount)
				if err != nil {
					log.Error().Msgf("withdraw budget asset %s error: %+v", assetName, err)
					return err
				}
			}
		} else if newAppBundleReq.Entity == "guild" {
			for assetName, usedAmount := range assetAmountUsedInThisRequest {
				err = model.GuildBudgetModel.WithdrawSingleAsset(tx, newAppBundleReq.EntityId, assetName, usedAmount)
				if err != nil {
					log.Error().Msgf("withdraw budget asset %s error: %+v", assetName, err)
					return err
				}
			}
		}

		return err
	})

	if err != nil {
		log.Error().Msgf("Transaction error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create application error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("create application error detail:" + err.Error()))
	}

	// ctx.JSON(http.StatusCreated, api.Success(nil))
	return http.StatusCreated, api.Success(nil)
}

func (s *AppBundlesService) UpdateAppBundleToNewState(ctx *gin.Context, newState model.ApplicationState) (int, *api.Reply) {
	var idList []int
	err := ctx.Bind(&idList)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request error: %+v", err)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request error: %+v", err))
	}

	var appBundleRcds []model.AppBundle
	err = s.Db.Preload("AppRecords").Find(&appBundleRcds, idList).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		sdk.LogUserSideError(ctx, err)
		// ctx.JSON(http.StatusNotFound, api.BadRequest(errors.New("app bundle record not found")))
		return http.StatusNotFound, api.BadRequest(errors.New("app bundle record not found detail:" + err.Error()))
	}

	for _, r := range appBundleRcds {
		if r.State != model.ApplicationStateOpen && r.State != model.ApplicationStateRejected {
			err := fmt.Errorf("app bundle %+v is at processable state", r)
			sdk.LogUserSideError(ctx, err)
			// ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return http.StatusBadRequest, api.BadRequest(err)
		}
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProjAndGuild, internal.ActAuditApplication)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return http.StatusBadRequest, api.BadRequest(errors.New("check permission error detail:" + err.Error()))
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProjAndGuild, internal.ActCreateApplication)
		// ctx.JSON(http.StatusForbidden, api.Forbidden())
		return http.StatusForbidden, api.Forbidden()
	}

	push := api.ForContextOnlyPush(ctx)

	var action model.AuditActionType
	switch newState {
	case model.ApplicationStateApproved:
		action = model.AuditActionApprove
	case model.ApplicationStateRejected:
		action = model.AuditActionReject
	default:
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("unknown new state %s", newState)))
		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("unknown new state %s", newState))
	}

	// prepare QuickAccounting records
	var qaInputs []*sdk.QAInput
	now := time.Now().In(internal.ProjectTimezone).Format(time.DateTime)

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, appBundleRcd := range appBundleRcds {
			appBundleRcd.State = newState
			appBundleRcd.UpdateTs = model.GetCurrentUtcEpochSecond()
			appBundleRcd.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			if err = tx.Updates(&appBundleRcd).Error; err != nil {
				log.Error().Msgf("update application state error: %+v, app bundle: %+v", err, appBundleRcd)
				return err
			}

			err = tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
				AppBundleId: appBundleRcd.ID,
				AppBundle:   appBundleRcd,
				LogTs:       model.GetCurrentUtcEpochSecond(),
				Operation:   action,
				Operator:    common.FormatUserWallet(user.Wallet),
				PreState:    model.ApplicationStateOpen,
				PostState:   newState,
				ExtraData:   "",
			}).Error
			if err != nil {
				return err
			}

			for _, appRcd := range appBundleRcd.AppRecords {
				err = model.AuditApplication(tx, common.FormatUserWallet(user.Wallet), appRcd, action, "", enforcer, push)
				if err != nil {
					log.Error().Msgf("update application state error: %+v, app bundle: %+v", err, appBundleRcd)
					tx.Rollback()
					return err
				}

				err = tx.Model(model.ApplicationAuditLog{}).Create(&model.ApplicationAuditLog{
					ApplicationID: appRcd.ID,
					LogTs:         model.GetCurrentUtcEpochSecond(),
					Operation:     action,
					Operator:      common.FormatUserWallet(user.Wallet),
					PreState:      model.ApplicationStateOpen,
					PostState:     newState,
					ExtraData:     "",
				}).Error
				if err != nil {
					log.Error().Msgf("create app bundle audit log record error: %+v, app bundle: %+v", err, appBundleRcd)
					tx.Rollback()
					return err
				}

				if err != nil {
					log.Error().Msgf("create auto xfer task error: %+v, app bundle: %+v", err, appBundleRcd)
					tx.Rollback()
					return err
				}

				// send to QuickAccounting
				if newState == model.ApplicationStateApproved {
					// only send support token to QuickAccounting
					if dc, exist := internal.AssertDecimalsAndContractAddr[appRcd.AssetName]; exist {
						budgetSource := "unknown budget source"
						if appRcd.EntityType == "guild" {
							guild, _ := model.GuildModel.Detail(db, appRcd.EntityId)
							if guild != nil {
								budgetSource = guild.Name
							}
						} else if appRcd.EntityType == "project" {
							proj, _ := model.ProjectModel.Detail(db, appRcd.EntityId)
							if proj != nil {
								budgetSource = proj.Name
							}
						}

						var seasonRecord *model.Season
						if err = tx.Find(&seasonRecord, appRcd.SeasonId).Error; err != nil {
							log.Error().Msgf("find season error: %+v", err)
							tx.Rollback()
							return err
						}

						qaInputs = append(qaInputs, &sdk.QAInput{
							Recipient:               appRcd.TargetUserWallet,
							Amount:                  appRcd.AssetAmount.String(),
							Decimals:                dc.Decimals,
							CurrencyName:            appRcd.AssetName,
							CurrencyContractAddress: dc.Addr,
							BudgetSource:            budgetSource,
							Session:                 seasonRecord.Name,
							Item:                    appRcd.DetailedType,
							Comment:                 appRcd.Comment,
							Applicant:               appRcd.Applicant,
							ApplyComment:            appBundleRcd.Comment,
							Reviewer:                user.Wallet,
							ReviewDate:              now,
						})
					}
				}
			}

			err = api.CreateAutoTransferScrTask(tx, appBundleRcd.AppRecords, 0)
			if err != nil {
				log.Error().Msgf("create auto xfer task error: %+v, app bundle: %+v", err, appBundleRcd)
				tx.Rollback()
				return err
			}
		}
		return nil
	})

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update app bundle state error")))
		return http.StatusInternalServerError, api.ServerError(errors.New("update app bundle state error detail:" + err.Error()))
	}

	//// send to QuickAccounting
	//if newState == model.ApplicationStateApproved {
	//	// TODO when error occur should retry
	//	err = sdk.SubmitToQuickAccounting(qaInputs, cfg)
	//	if err != nil {
	//		log.Error().Msgf("sumbit application to QuickAccounting error: %+v", err)
	//		sdk.LogServerErrorToSentry(ctx, err)
	//	}
	//}

	// ctx.JSON(http.StatusOK, api.Success(nil))
	return http.StatusOK, api.Success(nil)
}
