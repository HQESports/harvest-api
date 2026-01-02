package aws

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog/log"
)

type FileService interface {
	GetAllFiles()
	UploadFile(string, io.Reader) (string, error)
	TestConnection() error
}

type fileService struct {
	s3     *s3.Client
	bucket string
	region string
}

func NewFileService(accessKey, secretKey, bucketName, region string) (FileService, error) {
	// Create custom credentials
	credProvider := aws.CredentialsProviderFunc(func(ctx context.Context) (aws.Credentials, error) {
		return aws.Credentials{
			AccessKeyID:     accessKey,
			SecretAccessKey: secretKey,
		}, nil
	})

	// Create custom config with credentials and region
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credProvider),
	)
	if err != nil {
		return nil, err
	}

	// Create an Amazon S3 service client
	client := s3.NewFromConfig(cfg)

	return &fileService{
		s3:     client,
		bucket: bucketName,
		region: region,
	}, nil
}

func (s *fileService) UploadFile(fileName string, file io.Reader) (string, error) {
	uploader := manager.NewUploader(s.s3)
	_, err := uploader.Upload(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(fileName),
		Body:   file,
	})

	if err != nil {
		return "", err
	}

	// Construct the URL manually
	imageURL := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", s.bucket, s.region, fileName)
	return imageURL, nil
}

func (s *fileService) GetAllFiles() {
	// Get the first page of results for ListObjectsV2 for a bucket
	output, err := s.s3.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {

	}

	for _, object := range output.Contents {
		log.Printf("key=%s size=%d", aws.ToString(object.Key), object.Size)
	}
}

func (s *fileService) TestConnection() error {
	// Try to list objects with max 1 result to test the connection
	_, err := s.s3.ListObjectsV2(context.TODO(), &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		MaxKeys: aws.Int32(1), // Only fetch 1 key to minimize data transfer
	})
	log.Err(err).Msg("AWS S3 Test Connection")

	return err
}
