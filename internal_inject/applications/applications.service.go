package applications_inject

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/api"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/config"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/task_manager"
	"gorm.io/gorm"
)

type ApplicationsService struct {
	Db *gorm.DB `inject:""`

	Cfg *config.Config `inject:""`
}

func (s *ApplicationsService) Create(ctx *gin.Context) (httpCode int, reply *api.Reply) {
	var newApplicationReqs []model.NewApplicationRequest
	if err := ctx.BindJSON(&newApplicationReqs); err != nil {
		sdk.LogUserSideError(ctx, err)
		// ctx.JSON(http.StatusBadRequest, api.BadRequest(fmt.Errorf("passed in data error: %+v", err)))

		return http.StatusBadRequest, api.BadRequest(fmt.Errorf("passed in data error: %+v", err))
	}

	user, enforcer, _, _ := api.ForContext(ctx)

	err := s.Db.Transaction(func(tx *gorm.DB) error {
		for _, req := range newApplicationReqs {
			// TODO: Verify user wallet and project/guild existing

			// Parse application type
			appType, err := model.ParseApplicationType(req.Type)
			if err != nil {
				return fmt.Errorf("unknown application entity %s", req.Entity)
			}

			if appType == model.ApplicationNewReward {
				return fmt.Errorf("NEW_REWARD application should be submitted via app_bundle")
			}

			if req.Entity != "guild" && req.Entity != "project" {
				return fmt.Errorf("unknown application entity %s", req.Entity)
			}

			//  check permission: `(0x..., proj_1, create_app)` (0x..., guild_1, create_app)
			obj := lo.
				If(req.Entity == "project", fmt.Sprintf("%s%d", internal.ObjProjPrefix, req.EntityId)).
				ElseIf(req.Entity == "guild", fmt.Sprintf("%s%d", internal.ObjGuildPrefix, req.EntityId)).
				Else("")
			ok, err := enforcer.Enforce(common.FormatUserWallet(user.Wallet), obj, internal.ActCreateApplication)
			if err != nil {
				log.Error().Msgf("check permission error: %+v", err)
				sdk.LogServerErrorToSentry(ctx, err)
				// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("check permission error")))
				httpCode, reply = http.StatusBadRequest, api.BadRequest(errors.New("check permission error detail:"+err.Error()))
				return err
			}
			if !ok {
				sdk.LogForbiddenError(ctx, user.Wallet, obj, internal.ActCreateApplication)
				// ctx.JSON(http.StatusForbidden, api.Forbidden())
				httpCode, reply = http.StatusForbidden, api.Forbidden()
				return err
			}

			seasonRecord, err := model.GetCurrentSeason(tx)
			if err != nil {
				return err
			}

			appBundle := model.AppBundle{
				Applicant:    common.FormatUserWallet(user.Wallet),
				EntityType:   req.Entity,
				EntityId:     req.EntityId,
				SeasonId:     seasonRecord.ID,
				State:        model.ApplicationStateOpen,
				CreatedAt:    time.Now().In(internal.ProjectTimezone),
				UpdatedAt:    time.Now().In(internal.ProjectTimezone),
				CreateTs:     model.GetCurrentUtcEpochSecond(),
				UpdateTs:     model.GetCurrentUtcEpochSecond(),
				ShadowRecord: true,
				Type:         "CLOSE_PROJECT",
			}
			err = tx.Save(&appBundle).Error
			if err != nil {
				return err
			}

			app := &model.Application{
				Type:         appType,
				Applicant:    common.FormatUserWallet(user.Wallet),
				State:        model.ApplicationStateOpen,
				EntityType:   req.Entity,
				EntityId:     req.EntityId,
				SeasonId:     seasonRecord.ID,
				DetailedType: req.DetailedType,
				Comment:      req.Comment,
				CreatedAt:    time.Now().In(internal.ProjectTimezone),
				UpdatedAt:    time.Now().In(internal.ProjectTimezone),
				CreateTs:     model.GetCurrentUtcEpochSecond(),
				UpdateTs:     model.GetCurrentUtcEpochSecond(),
				BundleId:     appBundle.ID,
			}

			err = model.NewApplicationRecord(tx, app)
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		log.Error().Msgf("creation application records error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		// ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("creation application records error")))
		if httpCode != 0 {
			return
		} else {
			return http.StatusInternalServerError, api.ServerError(errors.New("creation application records error detail:" + err.Error()))
		}
	}

	// ctx.JSON(http.StatusCreated, api.Success(nil))
	return http.StatusCreated, api.Success(nil)
}

func (s *ApplicationsService) GetBatchApplicationsOrReturnError(ctx *gin.Context, applications *[]*model.Application) error {
	var idList []int
	err := ctx.Bind(&idList)
	if err != nil {
		return fmt.Errorf("passed in ID list error")
	}

	db := api.ForContextOnlyDB(ctx)
	db.Find(&applications, idList)
	return nil
}

func (s *ApplicationsService) GetRecordOrReturnNotFound(ctx *gin.Context, application *model.Application) {
	id := ctx.Param("id")
	db := api.ForContextOnlyDB(ctx)
	tx := db.First(&application, id)

	if errors.Is(tx.Error, gorm.ErrRecordNotFound) {
		err := fmt.Errorf("application with id %s not found", id)
		sdk.LogUserSideError(ctx, err)
		ctx.JSON(http.StatusNotFound, api.BadRequest(err))
		return
	}
}

func (s *ApplicationsService) GenerateAutoXferTaskResponse(task model.CronJob) *AutoXferTaskResponse {
	var taskParams *task_manager.AutoTransferScrParam
	err := json.Unmarshal([]byte(task.JobParams), &taskParams)
	if err != nil {
		log.Error().Msgf("parse auto transfer SCR params error: %+v", err)
		return nil
	}

	resp := AutoXferTaskResponse{
		ID:       task.ID,
		CreateTs: task.CreateTs,
		UpdateTs: task.UpdateTs,

		State: task.StateName(),

		TransactionItems: taskParams.Items,

		IsFinished: task.State == model.CronJobStateDone,
	}

	var execResult task_manager.AutoTransferScrTaskResult
	if task.LastExecResult != "" {
		err = json.Unmarshal([]byte(task.LastExecResult), &execResult)
		if err != nil {
			log.Error().Msgf("parse auto transfer SCR task result error: %+v", err)
		}
		resp.TransactionHash = execResult.Data.TxHash
	}

	return &resp
}

func (s *ApplicationsService) GetCronJobRecordFromStrId(taskId string) (model.CronJob, error) {
	var task model.CronJob
	err := s.Db.Model(&model.CronJob{}).Where("id = ?", taskId).Find(&task).Error
	if err != nil {
		return model.CronJob{}, err
	}

	return task, nil
}

func (s *ApplicationsService) SumAssetAmount(ctx *gin.Context, stat string, assetName string, seasonId int) (float64, error) {
	var value float64

	err := s.Db.Model(&model.Application{}).
		Select("sum(cast(asset_amount as decimal)) as total").
		Where("state = ?", stat).Where("asset_name = ?", assetName).
		Where("season_id = ?", seasonId).
		Scan(&value).Error
	if err != nil {
		log.Error().Msgf("get sum asset amount error: %+v", err)
		sdk.LogServerErrorToSentry(ctx, err)
		return 0, nil
	}

	return value, nil
}
