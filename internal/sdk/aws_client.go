package sdk

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type AwsClient struct {
	Region     string
	BucketName string

	Uploader *s3manager.Uploader
}

var awsClient *AwsClient

func InitAwsClient(accessKey, secret, region, bucketName string) error {
	awsSession, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(accessKey, secret, ""),
		Region:      aws.String(region)},
	)

	if err != nil {
		return err
	}

	uploader := s3manager.NewUploader(awsSession)

	awsClient = &AwsClient{
		Region:     region,
		BucketName: bucketName,
		Uploader:   uploader,
	}
	return nil
}

func GetAwsClient() *AwsClient {
	return awsClient
}

func (c *AwsClient) UploadEntityLogo(entityId uint, entityType string, b64ImageStr string) (string, error) {
	decode, err := base64.StdEncoding.DecodeString(b64ImageStr)

	if err != nil {
		return "", err
	}

	// Parse file extension from passed in params
	fileExt := "png"
	if strings.Contains(b64ImageStr, "image/jpeg") {
		fileExt = "jpg"
	}

	fileKey := fmt.Sprintf("%s-%d/logo.%s", entityType, entityId, fileExt)

	_, err = c.Uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(c.BucketName),
		Key:    aws.String(fileKey),
		Body:   bytes.NewReader(decode),
	})

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%s.%s.amazonaws.com/%s", c.BucketName, c.Region, fileKey), nil
}
