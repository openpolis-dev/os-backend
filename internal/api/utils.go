package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/theseed-labs/os-backend/internal/sdk"
)

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
