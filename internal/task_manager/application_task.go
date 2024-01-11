package task_manager

import (
	"github.com/theseed-labs/os-backend/internal/model"
	"gorm.io/gorm"
)

type NewRewardTaskParam struct {
	Applicant   string `json:"applicant"`
	Description string `json:"description"`
	EntityInfo  struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	} `json:"entity_info"`
	ProposalId string `json:"proposal_id"`
	Records    []struct {
		Amount    string `json:"amount"`
		AssetName struct {
			Id   int    `json:"id"`
			Name string `json:"name"`
		} `json:"asset_name"`
		Comment          string `json:"comment"`
		TargetUserWallet string `json:"target_user_wallet"`
	} `json:"records"`
}

func CreateAppBundleTask(db *gorm.DB, job *model.CronJob, jobParams string) {
	//// Update the job state to done so it will not be launched again
	//log.Debug().Msgf("start create app bundle task: %+v", job)
	//err := db.Model(&job).Updates(model.CronJob{State: model.CronJobStateRunning}).Error
	//if err != nil {
	//	log.Warn().Msgf("update cron job error: %+v", err)
	//	return
	//}
	//
	//execResult := ""
	//jobFailed := false
	//
	//// Query entity type from
	//
	//var params NewRewardTaskParam
	//err = json.Unmarshal([]byte(jobParams), &params)
	//if err != nil {
	//	log.Warn().Msgf("create app bundle job params error: %+v", err)
	//	execResult = err.Error()
	//	jobFailed = true
	//} else {
	//	// TODO: Duplicated code *NewAppBundleAndApplication*
	//	// Create AppBundle
	//	appBundle := model.AppBundle{
	//		Comment:      params.Description,
	//		Applicant:    common.FormatUserWallet(params.Applicant),
	//		EntityType:   newAppBundleReq.Entity,
	//		EntityId:     newAppBundleReq.EntityId,
	//		SeasonId:     seasonRecord.ID,
	//		Season:       *seasonRecord,
	//		State:        model.ApplicationStateOpen,
	//		ShadowRecord: false,
	//		CreatedAt:    time.Now().In(internal.ProjectTimezone),
	//		UpdatedAt:    time.Now().In(internal.ProjectTimezone),
	//		CreateTs:     model.GetCurrentUtcEpochSecond(),
	//		UpdateTs:     model.GetCurrentUtcEpochSecond(),
	//		Type:         "NEW_REWARD",
	//	}
	//	err = db.Model(model.AppBundle{}).Create(&appBundle).Error
	//	if err != nil {
	//		log.Error().Msgf("Create app bundle records error: %+v", err)
	//		sdk.LogServerErrorToSentry(ctx, err)
	//		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create app bundle record error")))
	//		return
	//	}
	//
	//	// Create Applications inside the bundle
	//	err = db.Transaction(func(tx *gorm.DB) error {
	//		appBundle.AppRecords = lo.Map(newAppBundleReq.Records, func(appRcdRequest *model.NewApplicationRequest, index int) *model.Application {
	//			return &model.Application{
	//				Type:             model.ApplicationNewReward,
	//				Applicant:        common.FormatUserWallet(user.Wallet),
	//				State:            model.ApplicationStateOpen,
	//				CreatedAt:        time.Now().In(internal.ProjectTimezone),
	//				UpdatedAt:        time.Now().In(internal.ProjectTimezone),
	//				CreateTs:         model.GetCurrentUtcEpochSecond(),
	//				UpdateTs:         model.GetCurrentUtcEpochSecond(),
	//				DetailedType:     appRcdRequest.DetailedType,
	//				Comment:          appRcdRequest.Comment,
	//				TargetUserWallet: appRcdRequest.TargetUserWallet,
	//				AssetName:        appRcdRequest.AssetName,
	//				AssetAmount:      appRcdRequest.Amount,
	//				EntityType:       newAppBundleReq.Entity,
	//				EntityId:         newAppBundleReq.EntityId,
	//				SeasonId:         seasonRecord.ID,
	//				Season:           seasonRecord,
	//			}
	//		})
	//
	//		err = tx.Save(&appBundle).Error
	//		if err != nil {
	//			log.Error().Msgf("update app_bundle record error: %+v", err)
	//			return err
	//		}
	//
	//		// Create application audit logs
	//		appAuditLogs := lo.Map(appBundle.AppRecords, func(app *model.Application, _ int) *model.ApplicationAuditLog {
	//			return &model.ApplicationAuditLog{
	//				ApplicationID: app.ID,
	//				LogTs:         model.GetCurrentUtcEpochSecond(),
	//				Operation:     model.AuditActionNew,
	//				Operator:      common.FormatUserWallet(user.Wallet),
	//				PreState:      "",
	//				PostState:     model.ApplicationStateOpen,
	//			}
	//		})
	//		err = tx.Model(model.ApplicationAuditLog{}).Create(&appAuditLogs).Error
	//		if err != nil {
	//			log.Error().Msgf("Create application audit log records error: %+v", err)
	//			return err
	//		}
	//
	//		return tx.Model(model.AppBundleAuditLog{}).Create(&model.AppBundleAuditLog{
	//			AppBundleId: appBundle.ID,
	//			AppBundle:   appBundle,
	//			LogTs:       model.GetCurrentUtcEpochSecond(),
	//			Operation:   model.AuditActionNew,
	//			Operator:    common.FormatUserWallet(user.Wallet),
	//			PreState:    "",
	//			PostState:   model.ApplicationStateOpen,
	//			ExtraData:   "",
	//		}).Error
	//	})
	//
	//	if err != nil {
	//		log.Error().Msgf("Transaction error: %+v", err)
	//		sdk.LogServerErrorToSentry(ctx, err)
	//		ctx.JSON(http.StatusInternalServerError, api.ServerError(errors.New("create application error")))
	//		return
	//	}
	//
	//	// TODO: update permission
	//}
	//
	//job.LastExecTs = model.GetCurrentUtcEpochSecond()
	//job.LastExecResult = execResult
	//job.LastExecutionFailed = jobFailed
	//job.State = model.CronJobStateDone
	//db.Updates(&job)
}
