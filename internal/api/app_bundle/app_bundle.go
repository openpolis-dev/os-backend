package app_bundle

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"gorm.io/gorm"
)

type AppBundleResponseRecord struct {
	ID         uint                               `json:"id"`
	SeasonName string                             `json:"season_name"`
	Records    []*model.FrontendApplicationRecord `json:"records"`

	Entity struct {
		Id   uint   `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"entity"`
	Applicant string                 `json:"applicant"`
	ApplyTime time.Time              `json:"apply_time"`
	ApplyTs   int64                  `json:"apply_ts"`
	Reviewer  string                 `json:"reviewer"`
	Comment   string                 `json:"comment"`
	State     model.ApplicationState `json:"state"`
	Assets    []struct {
		Name   string `json:"name"`
		Amount string `json:"amount"`
	} `json:"assets"`
}

type ListAvailableProjectAndGuildResp struct {
	Guilds             []*model.Guild              `json:"guilds"`
	Projects           []*model.Project            `json:"projects"`
	CommonBudgetSource []*model.CommonBudgetSource `json:"common_budget_source"`
}

// ListAvailableProjectsAndGuilds returns available projects and guilds for current user
//
// @summary	List available projects and guilds for current user, and all common budget sources
// @router		/available_projects_guilds [get]
// @tags		AppBundle
// @success	200	{object}	api.Reply{data=ListAvailableProjectAndGuildResp}
func ListAvailableProjectsAndGuilds(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)

	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}

	var guilds []*model.Guild
	var projects []*model.Project

	if ok {
		guilds, _, err = model.GuildModel.List(db, nil)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
			return
		}

		projects, _, err = model.ProjectModel.List(db, "open", nil, false)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list projects error")))
			return
		}
	} else {
		guilds, _, err = model.GuildModel.ListBySponsor(db, common.FormatUserWallet(user.Wallet), nil)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list guilds error")))
			return
		}

		projects, _, err = model.ProjectModel.ListBySponsor(db, common.FormatUserWallet(user.Wallet), "open", nil, false)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list projects error")))
			return
		}
	}

	var commonBudgetSources []*model.CommonBudgetSource
	err = db.Model(&model.CommonBudgetSource{}).Find(&commonBudgetSources).Error
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("list common budget sources error")))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(&ListAvailableProjectAndGuildResp{
		guilds,
		projects,
		commonBudgetSources,
	}))
}

// ListAppBundle returns application bundles with passed in query types
//
//	@summary	List all application bundles match the query params
//	@router		/app_bundles [get]
//	@tags		AppBundle
//	@param		page		query		string	false	"which page"
//	@param		size		query		string	false	"size of each page"
//	@param		sort_field	query		string	false	"sort by which field"
//	@param		sort_order	query		string	false	"order of sort"			Enum(asc desc)
//	@param		state		query		string	false	"state of app bundle"	Enum(open approved rejected)
//	@param		applicant	query		string	false	"applicant of app bundle"
//	@param		season_id	query		int		false	"season id"
//	@param		entity		query		string	false	"entity name to be filter"	Enum(project guild)
//	@param		entity_id	query		string	false	"entity id"
//	@success	200			{object}	AppBundleResponseRecord
func ListAppBundle(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)

	queryParams := model.ListAppBundleQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("query params error: %+v", err)))
		return
	}

	appBundleRecords, total, err := model.QueryAppBundleRecords(db, &queryParams)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("query result error")))
		return
	}

	respRcds := lo.Map(appBundleRecords, func(jointAppBundleEntityRcd model.JointAppBundleEntityRslt, index int) AppBundleResponseRecord {
		assetSummary := make(map[string]decimal.Decimal)

		var appRcds []*model.Application
		err = db.Where("bundle_id = ?", jointAppBundleEntityRcd.AppBundle.ID).
			Find(&appRcds).
			Error
		if err != nil {
			log.Error().Msgf("query application error: %+v", err)
			return AppBundleResponseRecord{}
		}

		// Summarize the assets in app bundle
		for _, appRcd := range appRcds {
			model.SetDefaultMapValue(assetSummary, appRcd.AssetName, decimal.Zero)
			assetSummary[appRcd.AssetName] = assetSummary[appRcd.AssetName].Add(appRcd.AssetAmount)
		}

		appIds := lo.Map(appRcds, func(appRcd *model.Application, index int) uint {
			return appRcd.ID
		})

		frontApplicationRecords, err := model.GenerateFrontendApplicationRecordsByIds(db, appIds)
		if err != nil {
			log.Error().Msgf("query application error: %+v", err)
			return AppBundleResponseRecord{}
		}

		return AppBundleResponseRecord{
			ID:         jointAppBundleEntityRcd.AppBundle.ID,
			SeasonName: jointAppBundleEntityRcd.SeasonName,
			Records:    frontApplicationRecords,
			Entity: struct {
				Id   uint   `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			}{
				Id:   jointAppBundleEntityRcd.AppBundle.EntityId,
				Name: jointAppBundleEntityRcd.EntityName,
				Type: jointAppBundleEntityRcd.AppBundle.EntityType,
			},
			Applicant: jointAppBundleEntityRcd.AppBundle.Applicant,
			ApplyTime: jointAppBundleEntityRcd.AppBundle.CreatedAt,
			ApplyTs:   jointAppBundleEntityRcd.AppBundle.CreateTs,
			Comment:   jointAppBundleEntityRcd.AppBundle.Comment,
			State:     jointAppBundleEntityRcd.AppBundle.State,
			Assets: lo.MapToSlice(assetSummary, func(assetName string, amount decimal.Decimal) struct {
				Name   string `json:"name"`
				Amount string `json:"amount"`
			} {
				return struct {
					Name   string `json:"name"`
					Amount string `json:"amount"`
				}{
					Name:   assetName,
					Amount: amount.String(),
				}
			}),
		}
	})

	ctx.JSON(http.StatusOK, api.Success(api.ListReplyData{
		Page:  queryParams.Page,
		Size:  queryParams.Size,
		Total: total,
		Rows:  respRcds,
	}))
}

// CreateAppBundle returns application bundles with passed in query types
//
//	@summary	Create app bundles based on request data
//	@router		/app_bundles [post]
//	@tags		AppBundle
//	@param		request	body		model.NewAppBundleRequest	true	"New application bundle request"
//	@success	201		{string}	AppBundleResponseRecord
func CreateAppBundle(ctx *gin.Context) {
	var newAppBundleReq model.NewAppBundleRequest
	if err := ctx.BindJSON(&newAppBundleReq); err != nil {
		if err != nil {
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request error: %+v", err)))
		}
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	ok, err := enforcer.HasRoleForUser(common.FormatUserWallet(user.Wallet), internal.RoleHall)
	if err != nil {
		log.Error().Msgf("check permission error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}

	if !ok {
		log.Debug().Msgf("user %s has no hall permission, check whether user is sponsor", user.Wallet)

		// Check permission, using ActCreateApplication for now, can be changed to new permission if required
		// TODO: Merge to separated functions
		obj := lo.
			If(newAppBundleReq.Entity == "project", fmt.Sprintf("%s%d", internal.ObjProjPrefix, newAppBundleReq.EntityId)).
			ElseIf(newAppBundleReq.Entity == "guild", fmt.Sprintf("%s%d", internal.ObjGuildPrefix, newAppBundleReq.EntityId)).
			Else("")
		ok, err = enforcer.Enforce(common.FormatUserWallet(user.Wallet), obj, internal.ActCreateApplication)
		if err != nil {
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
			return
		}
		if !ok {
			sdk.LogForbiddenError(ctx, user.Wallet, obj, internal.ActCreateApplication)
			ctx.JSON(http.StatusForbidden, api.Forbidden())
			return
		}
	} else {
		log.Debug().Msgf("user %s has hall permission, can create app bundle", user.Wallet)
	}

	seasonRecord, err := model.GetCurrentSeason(db)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("get current season error")))
		return
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

	err = db.Transaction(func(tx *gorm.DB) error {
		err = tx.Model(model.AppBundle{}).Create(&appBundle).Error
		if err != nil {
			log.Error().Msgf("Create app bundle records error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
			ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create app bundle record error")))
			return err
		}

		appBundle.AppRecords = lo.Map(newAppBundleReq.Records, func(appRcdRequest *model.NewApplicationRequest, index int) *model.Application {
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
				TargetUserWallet: appRcdRequest.TargetUserWallet,
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

		return tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
			AppBundleId: appBundle.ID,
			AppBundle:   appBundle,
			LogTs:       model.GetCurrentUtcEpochSecond(),
			Operation:   model.AuditActionNew,
			Operator:    common.FormatUserWallet(user.Wallet),
			PreState:    "",
			PostState:   model.ApplicationStateOpen,
			ExtraData:   "",
		}).Error
	})

	if err != nil {
		log.Error().Msgf("Transaction error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create application error")))
		return
	}

	ctx.JSON(http.StatusCreated, api.Success(nil))
}

// ApproveAppBundles approve appBundle and associated applications
//
//	@summary	Approve app bundle and associated applications
//	@router		/app_bundle_approve [post]
//	@tags		AppBundle
//	@param		JsonBody	body		[]string	true	"app bundle IDs"
//	@success	200			{string}	nil
func ApproveAppBundles(ctx *gin.Context) {
	updateAppBundleToNewState(ctx, model.ApplicationStateApproved)
}

// RejectAppBundles reject appBundle and associated applications
//
//	@summary	Reject app bundle and associated applications
//	@router		/app_bundles_reject [post]
//	@tags		AppBundle
//	@param		JsonBody	body		[]string	true	"app bundle IDs"
//	@success	200			{string}	nil
func RejectAppBundles(ctx *gin.Context) {
	updateAppBundleToNewState(ctx, model.ApplicationStateRejected)
}

func updateAppBundleToNewState(ctx *gin.Context, newState model.ApplicationState) {
	db, cfg := api.ForContextDBAndConfig(ctx)
	var idList []int
	err := ctx.Bind(&idList)
	if err != nil {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("parse request error: %+v", err)))
		return
	}

	var appBundleRcds []model.AppBundle
	err = db.Preload("AppRecords").Find(&appBundleRcds, idList).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusNotFound, api.BadRequest(errors.New("app bundle record not found")))
		return
	}

	for _, r := range appBundleRcds {
		if r.State != model.ApplicationStateOpen && r.State != model.ApplicationStateRejected {
			err := fmt.Errorf("app bundle %+v is at processable state", r)
			sdk.LogUserSideError(ctx, err)
			ctx.JSON(http.StatusBadRequest, api.BadRequest(err))
			return
		}
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), internal.ObjProjAndGuild, internal.ActAuditApplication)
	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
		return
	}
	if !ok {
		sdk.LogForbiddenError(ctx, user.Wallet, internal.ObjProjAndGuild, internal.ActCreateApplication)
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	push := api.ForContextOnlyPush(ctx)

	var action model.AuditActionType
	switch newState {
	case model.ApplicationStateApproved:
		action = model.AuditActionApprove
	case model.ApplicationStateRejected:
		action = model.AuditActionReject
	default:
		ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("unknown new state %s", newState)))
		return
	}

	// send to QuickAccounting
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
		}
		return nil
	})

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("update app bundle state error")))
		return
	}

	// send to QuickAccounting
	if newState == model.ApplicationStateApproved {
		// TODO when error occur should retry
		err = sdk.SubmitToQuickAccounting(qaInputs, cfg)
		if err != nil {
			log.Error().Msgf("sumbit application to QuickAccounting error: %+v", err)
			sdk.LogServerErrorToSentry(ctx, err)
		}
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
