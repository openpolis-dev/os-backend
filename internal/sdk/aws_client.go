package sdk

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type AwsClient struct {
	Region     string
	BucketName string

	Uploader *s3manager.Uploader // s3 uploaeder client
	Svc      *s3.S3              // s3 service client

	isActivated bool // Indicate whether this client is activated, client will be marked as activated only when accessKey, secret, region and bucketName are all non-empty
}

var awsClient *AwsClient

func InitAwsClient(accessKey, secret, region, bucketName string) error {
	if accessKey == "" || secret == "" || region == "" || bucketName == "" {
		awsClient = &AwsClient{
			isActivated: false,
		}
		return nil
	}

	awsSession, err := session.NewSession(&aws.Config{
		Credentials: credentials.NewStaticCredentials(accessKey, secret, ""),
		Region:      aws.String(region)},
	)

	if err != nil {
		return err
	}

	uploader := s3manager.NewUploader(awsSession)
	svc := s3.New(awsSession)

	awsClient = &AwsClient{
		Region:      region,
		BucketName:  bucketName,
		Uploader:    uploader,
		Svc:         svc,
		isActivated: true,
	}
	return nil
}

func GetAwsClient() *AwsClient {
	return awsClient
}

// UploadEntityLogo uploads entity logo passed from frontend in base64 format to AWS S3 and return URL
// Note: The passed in base64 image string contains header, such as `data:image/png;base64,XXXX`
func (c *AwsClient) UploadEntityLogo(entityId uint, entityType string, b64ImgSrcWithType string) (string, error) {
	// Passed in data is not a b64 image string, a possible data is image url.
	// Return the passed in data directly and no error
	if !strings.HasPrefix(b64ImgSrcWithType, "data:image") {
		return b64ImgSrcWithType, nil
	}

	// For non-activated client, return b64 string directly
	if !c.isActivated {
		return b64ImgSrcWithType, nil
	}

	imageData, contentType, fileExt, err := c.parseB64ImageStringWithType(b64ImgSrcWithType)

	if err != nil {
		return "", err
	}

	fileKey := fmt.Sprintf("%s-%d/logo.%s", entityType, entityId, fileExt)

	return c.uploadB64Image(imageData, contentType, fileKey)
}

func (c *AwsClient) UploadUserAvatar(userWallet string, b64ImgSrcWithType string) (string, error) {
	// Passed in data is not a b64 image string, a possible data is image url.
	// Return the passed in data directly and no error
	if !strings.HasPrefix(b64ImgSrcWithType, "data:image") {
		return b64ImgSrcWithType, nil
	}

	// For non-activated client, return b64 string directly
	if !c.isActivated {
		return b64ImgSrcWithType, nil
	}

	imageData, contentType, fileExt, err := c.parseB64ImageStringWithType(b64ImgSrcWithType)

	if err != nil {
		return "", err
	}

	fileKey := fmt.Sprintf("user_avatars/%s.%s", userWallet, fileExt)

	return c.uploadB64Image(imageData, contentType, fileKey)
}

func (c *AwsClient) GetS3PreSignedURL(fileName string, contentType string) (string, error) {
	req, _ := c.Svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket:      aws.String(c.BucketName),
		Key:         aws.String(fileName),
		ContentType: aws.String(contentType),
	})
	urlStr, err := req.Presign(5 * time.Minute)
	if err != nil {
		return "", err
	}
	return urlStr, nil
}

func (c *AwsClient) parseB64ImageStringWithType(b64ImgSrcWithType string) ([]byte, string, string, error) {
	imgData := strings.Split(b64ImgSrcWithType, ";base64,")
	imageData, err := base64.StdEncoding.DecodeString(imgData[1])

	if err != nil {
		return nil, "", "", err
	}

	// Parse file extension and content type from passed in params. The default file format is PNG file
	fileExt := "png"
	contentType := "image/png"
	if strings.Contains(imgData[0], "image/jpeg") || strings.Contains(imgData[0], "image/jpg") {
		fileExt = "jpg"
		contentType = "image/jpeg"
	} else if strings.Contains(imgData[0], "image/svg") {
		fileExt = "svg"
		contentType = "image/svg+xml"
	}

	return imageData, contentType, fileExt, nil
}

func (c *AwsClient) uploadB64Image(imageData []byte, contentType string, fileKey string) (string, error) {
	_, err := c.Uploader.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(c.BucketName),
		Key:         aws.String(fileKey),
		Body:        bytes.NewReader(imageData),
		ContentType: aws.String(contentType),
	})

	if err != nil {
		return "", err
	}

	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", c.BucketName, c.Region, fileKey), nil
}
