package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/theseed-labs/os-backend/internal"
	"github.com/theseed-labs/os-backend/internal/common"
	"github.com/theseed-labs/os-backend/internal/model"
	"github.com/theseed-labs/os-backend/internal/sdk"
	"github.com/theseed-labs/os-backend/internal/storage"
	"gorm.io/gorm"
)

// const ScrTaskPendingTime = 24 * time.Hour
const ScrTaskPendingTime = 2 * time.Minute

// Copy struct from task_manager/application_task.go to avoid import cycle
type autoTransferScrItem struct {
	ApplicationId uint   `json:"application_id"`
	TargetWallet  string `json:"target_wallet"`
	ScrAmount     string `json:"scr_amount"`
}

type autoTransferScrParam struct {
	Applicant string                 `json:"applicant"`
	Items     []*autoTransferScrItem `json:"items"`
}

// PreSignedUrlForS3 generates a pre-signed URL for uploading a file to an S3 bucket.
//
//	@summary	Get pre-signed URL for S3 upload
//	@router		/url_for_uploading_s3 [get]
//	@param		filename	query		string	true	"Name of the file"
//	@param		type		query		string	false	"Type of the file"
//	@param		bucket		query		string	true	"S3 bucket name, should be created in advanced"
//	@success	200			{object}	api.Reply{data=string}
func PreSignedUrlForS3(ctx *gin.Context) {
	fileName := ctx.Query("filename")
	contentType := ctx.Query("type")
	bucketName := ctx.Query("bucket")
	if fileName == "" {
		fileName = uuid.NewString()
	}

	if contentType == "" {
		fileExt := path.Ext(fileName)
		switch strings.ToLower(fileExt) {
		case ".svg":
			contentType = "image/svg+xml"
		case ".jpg", ".jpeg":
			contentType = "image/jpg"
		case ".png":
			contentType = "image/png"
		}
	}

	uploadUrl, err := sdk.GetAwsClient().GetS3PreSignedURL(bucketName, fileName, contentType)

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, ServerError(errors.New("upload url error")))
		return
	}

	ctx.JSON(http.StatusOK, Success(uploadUrl))
}

func PrintStructAsJson(object any, prompt string) {
	jsonStr, _ := json.MarshalIndent(object, "  ", "  ")
	fmt.Println("=========================")
	fmt.Printf("%s: %s\n", prompt, jsonStr)
	fmt.Println("=========================")
}

func GetCachedSeepassData(sppClient *sdk.SppClient, wallet string, ignoreCache bool) (*sdk.SeepassResponse, error) {
	_wallet := common.FormatUserWallet(wallet)
	if ignoreCache {
		return RefreshSeepassDataCache(sppClient, wallet)
	} else {
		seepassDataBytes, err := storage.GetCachedData(wallet)
		if err != nil {
			log.Warn().Msgf("get seepass data from cache error: %+v", err)
			return RefreshSeepassDataCache(sppClient, wallet)
		} else {
			var seepassData *sdk.SeepassResponse
			err = json.Unmarshal(seepassDataBytes, &seepassData)
			if err != nil {
				log.Warn().Msgf("unmarshal seepass data from cache error: %+v", err)
			} else {
				err = storage.StoreCachedData(storage.UserSeepassCacheKey(_wallet), seepassDataBytes)
				if err != nil {
					log.Warn().Msgf("write seepass data to cache error: %+v", err)
				}
			}
			return seepassData, nil
		}
	}
}

// RefreshSeepassDataCache fetches seepass data from server and try to write back to cache
// If fetching data error, nil and error message will be returned
// If error occurred at marshal or store to cache part, a warning message will be shown and data will be returned with nil error
func RefreshSeepassDataCache(sppClient *sdk.SppClient, wallet string) (*sdk.SeepassResponse, error) {
	_wallet := common.FormatUserWallet(wallet)
	seepassData, err := sppClient.GetSeepassData(_wallet)
	if err != nil {
		log.Error().Msgf("get seepass data from server error: %+v", err)
		return nil, err
	}

	seepassDataBytes, err := json.Marshal(seepassData)
	if err != nil {
		log.Warn().Msgf("marshal seepass data to bytes error: %+v", err)
		return seepassData, nil
	}

	err = storage.StoreCachedData(storage.UserSeepassCacheKey(_wallet), seepassDataBytes)
	if err != nil {
		log.Warn().Msgf("write seepass data to cache error: %+v", err)
	}
	return seepassData, nil
}

func CreateAutoTransferScrTask(db *gorm.DB, applications []*model.Application) error {
	// Issue send SCR tasks
	scrApplications := lo.Filter(applications, func(app *model.Application, _ int) bool {
		return app.Type == model.ApplicationNewReward && app.AssetName == "SCR"
	})

	if len(scrApplications) > 0 {
		return db.Transaction(func(tx *gorm.DB) error {
			// Group scr applications by applicant
			groupedScrApplications := lo.GroupBy(scrApplications, func(app *model.Application) string {
				return app.Applicant
			})

			currentTs := time.Now().UTC()
			jobExecTs := currentTs.Add(ScrTaskPendingTime)

			for applicant, apps := range groupedScrApplications {
				jobParams := autoTransferScrParam{
					Applicant: applicant,
					Items: lo.Map(apps, func(app *model.Application, _ int) *autoTransferScrItem {
						return &autoTransferScrItem{
							ApplicationId: app.ID,
							TargetWallet:  app.TargetUserWallet,
							ScrAmount:     app.AssetAmount.String(),
						}
					}),
				}

				jobParamsBytes, err := json.Marshal(jobParams)
				if err != nil {
					log.Error().Msgf("marshal auto transfer SCR job params error: %+v", err)
					return err
				}

				sendScrTask := &model.CronJob{
					CreateTs:       currentTs.Unix(),
					UpdateTs:       currentTs.Unix(),
					HandlerName:    internal.TaskAutoTransferSCR,
					LastExecTs:     0,
					NextExecTs:     jobExecTs.Unix(),
					JobParams:      string(jobParamsBytes),
					State:          model.CronJobStateActive,
					LastExecResult: "",
				}

				log.Debug().Msgf("create auto transfer SCR task: %+v", sendScrTask)
				err = tx.Create(&sendScrTask).Error
				if err != nil {
					log.Error().Msgf("create auto transfer SCR task error: %+v", err)
					return err
				}
			}

			return nil
		})
	}

	return nil
}
