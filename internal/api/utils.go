package api

import (
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

func PreSignedUrlForS3(ctx *gin.Context) {
	fileName := ctx.Query("filename")
	contentType := ctx.Query("type")
	if fileName == "" {
		fileName = uuid.NewString()
	}

	if contentType == "" {
		fileExt := path.Ext(fileName)
		if fileExt != "" {
			if strings.EqualFold(fileExt, "svg") {
				contentType = "image/svg+xml"
			} else if strings.EqualFold(fileExt, "jpg") || strings.EqualFold(fileExt, "jpeg") {
				contentType = "image/jpg"
			} else if strings.EqualFold(fileExt, "png") {
				contentType = "image/png"
			}
		}
	}

	uploadUrl, err := sdk.GetAwsClient().GetS3PreSignedURL(fileName, contentType)

	if err != nil {
		sdk.LogServerErrorToSentry(ctx, err)
		ctx.JSON(http.StatusInternalServerError, ServerError(err))
	}

	ctx.JSON(http.StatusOK, Success(uploadUrl))
}
