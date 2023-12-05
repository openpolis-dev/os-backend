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
	Guilds   []*model.Guild   `json:"guilds"`
	Projects []*model.Project `json:"projects"`
}

// ListAvailableProjectsAndGuilds returns available projects and guilds for current user
//
// @Summary	List available projects and guilds for current user
// @Router		/available_projects_guilds [get]
// @Tags		app_bundle
//
// @Success	200	{object}	api.Reply{data=ListAvailableProjectAndGuildResp}
func ListAvailableProjectsAndGuilds(ctx *gin.Context) {
	user, enforcer, db, _ := api.ForContext(ctx)

	ok, err := enforcer.HasRoleForUser(user.Wallet, api.RoleHall)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	var guilds []*model.Guild
	var projects []*model.Project

	if ok {
		guilds, _, err = model.GuildModel.List(db, nil)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		projects, _, err = model.ProjectModel.List(db, "open", nil, false)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	} else {
		guilds, _, err = model.GuildModel.ListBySponsor(db, user.Wallet, nil)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}

		projects, _, err = model.ProjectModel.ListBySponsor(db, user.Wallet, "open", nil, false)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
			return
		}
	}

	ctx.JSON(http.StatusOK, api.Success(&ListAvailableProjectAndGuildResp{
		guilds,
		projects,
	}))
}

// ListAppBundle returns application bundles with passed in query types
//
//	@Summary	List all application bundles match the query params
//	@Router		/app_bundles [get]
//	@Tags		app_bundle
//	@Param		page		query		string	false	"which page"
//	@Param		size		query		string	false	"size of each page"
//	@Param		sort_field	query		string	false	"sort by which field"
//	@Param		sort_order	query		string	false	"order of sort"			Enum(asc desc)
//	@Param		state		query		string	false	"state of app bundle"	Enum(open approved rejected)
//	@Param		applicant	query		string	false	"applicant of app bundle"
//	@Param		season_id	query		int		false	"season id"
//	@Param		entity		query		string	false	"entity name to be filter"	Enum(project guild)
//	@Param		entity_id	query		string	false	"entity id"
//
//	@Success	200			{object}	AppBundleResponseRecord
func ListAppBundle(ctx *gin.Context) {
	var err error
	db := api.ForContextOnlyDB(ctx)

	queryParams := model.ListAppBundleQueryParams{}
	if err := ctx.Bind(&queryParams); err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query params error: %+v", err),
		})
		return
	}

	appBundleRecords, total, err := model.QueryAppBundleRecords(db, &queryParams)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("query result error: %+v", err),
		})
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
//	@Summary	Create app bundles based on request data
//	@Router		/app_bundles [post]
//	@Tags		app_bundle
//	@Param		request	body		model.NewAppBundleRequest	true	"New application bundle request"
//
//	@Success	201		{string}	AppBundleResponseRecord
func CreateAppBundle(ctx *gin.Context) {
	var newAppBundleReq model.NewAppBundleRequest
	if err := ctx.BindJSON(&newAppBundleReq); err != nil {
		if err != nil {
			ctx.JSON(http.StatusBadRequest, api.Reply{
				Code: -1,
				Msg:  fmt.Sprintf("passed in data error: %+v", err),
			})
		}
		return
	}

	user, enforcer, db, _ := api.ForContext(ctx)

	// Check permission, using ActCreateApplication for now, can be changed to new permission if required
	// TODO: Merge to separated functions
	obj := lo.
		If(newAppBundleReq.Entity == "project", fmt.Sprintf("%s%d", api.ObjProjPrefix, newAppBundleReq.EntityId)).
		ElseIf(newAppBundleReq.Entity == "guild", fmt.Sprintf("%s%d", api.ObjGuildPrefix, newAppBundleReq.EntityId)).
		Else("")
	ok, err := enforcer.Enforce(user.Wallet, obj, api.ActCreateApplication)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
		ctx.JSON(http.StatusForbidden, api.Forbidden())
		return
	}

	// TODO: Need confirm about season number for application bundles
	seasonRecord, err := model.GetCurrentSeason(db)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		appBundle := model.AppBundle{
			Comment:      newAppBundleReq.Comment,
			Applicant:    user.Wallet,
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

		appBundle.AppRecords = lo.Map(newAppBundleReq.Records, func(appRcdRequest *model.NewApplicationRequest, index int) *model.Application {
			return &model.Application{
				Type:             model.ApplicationNewReward,
				Applicant:        user.Wallet,
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

		err = tx.Model(model.AppBundle{}).Create(&appBundle).Error
		if err != nil {
			log.Error().Msgf("Create app_bundle record error: %+v", err)
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
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusCreated, api.Success(nil))
}

// ApproveAppBundles approve appBundle and associated applications
//
//	@Summary	Approve app bundle and associated applications
//	@Router		/app_bundle_approve [post]
//	@Tags		app_bundle
//	@Param		JsonBody	body		[]string	true	"app bundle IDs"
//
//	@Success	200			{string}	nil
func ApproveAppBundles(ctx *gin.Context) {
	updateAppBundleToNewState(ctx, model.ApplicationStateApproved)
}

// RejectAppBundles reject appBundle and associated applications
//
//	@Summary	Reject app bundle and associated applications
//	@Router		/app_bundles_reject [post]
//	@Tags		app_bundle
//	@Param		JsonBody	body		[]string	true	"app bundle IDs"
//
//	@Success	200			{string}	nil
func RejectAppBundles(ctx *gin.Context) {
	updateAppBundleToNewState(ctx, model.ApplicationStateRejected)
}

func updateAppBundleToNewState(ctx *gin.Context, newState model.ApplicationState) {
	db := api.ForContextOnlyDB(ctx)
	var idList []int
	err := ctx.Bind(&idList)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("parse app bundle ids error"),
		})
		return
	}

	var appBundleRcds []model.AppBundle
	err = db.Preload("AppRecords").Find(&appBundleRcds, idList).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		ctx.JSON(http.StatusNotFound, api.Reply{
			Code: -1,
			Msg:  fmt.Sprintf("application with id %+v not found", idList),
		})
		return
	}

	for _, r := range appBundleRcds {
		if r.State != model.ApplicationStateOpen && r.State != model.ApplicationStateRejected {
			ctx.JSON(http.StatusBadRequest, api.Reply{
				Code: -1,
				Msg:  fmt.Sprintf("app bundle %+v is at processable state", r),
			})
			return
		}
	}

	user, enforcer, db, _ := api.ForContext(ctx)
	ok, err := enforcer.Enforce(user.Wallet, api.ObjProjAndGuild, api.ActAuditApplication)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}
	if !ok {
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

	err = db.Transaction(func(tx *gorm.DB) error {
		for _, appBundleRcd := range appBundleRcds {

			appBundleRcd.State = newState
			appBundleRcd.UpdateTs = model.GetCurrentUtcEpochSecond()
			appBundleRcd.UpdatedAt = time.Now().In(internal.ProjectTimezone)
			err = tx.Save(&appBundleRcd).Error
			if err != nil {
				return err
			}

			err = tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
				AppBundleId: appBundleRcd.ID,
				AppBundle:   appBundleRcd,
				LogTs:       model.GetCurrentUtcEpochSecond(),
				Operation:   action,
				Operator:    user.Wallet,
				PreState:    model.ApplicationStateOpen,
				PostState:   newState,
				ExtraData:   "",
			}).Error
			if err != nil {
				return err
			}

			for _, appRcd := range appBundleRcd.AppRecords {
				err = model.AuditApplication(tx, user.Wallet, appRcd, action, "", enforcer, push)
				if err != nil {
					log.Error().Msgf("update application state error: %+v, app bundle: %+v", err, appBundleRcd)
					tx.Rollback()
					return err
				}

				err = tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
					AppBundleId: appBundleRcd.ID,
					AppBundle:   appBundleRcd,
					LogTs:       model.GetCurrentUtcEpochSecond(),
					Operation:   action,
					Operator:    user.Wallet,
					PreState:    model.ApplicationStateOpen,
					PostState:   newState,
					ExtraData:   "",
				}).Error
				if err != nil {
					log.Error().Msgf("create app bundle audit log record error: %+v, app bundle: %+v", err, appBundleRcd)
					tx.Rollback()
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, api.ServerError(err))
		return
	}

	ctx.JSON(http.StatusOK, api.Success(nil))
}
