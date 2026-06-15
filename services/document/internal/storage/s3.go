package storage

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Storage implements ObjectStorage backed by S3 or MinIO.
// internalClient is used for HeadObject / DeleteObject / DeleteObjects (service-to-service).
// presignClient uses the public endpoint so generated URLs are reachable by clients.
type S3Storage struct {
	bucket        string
	internalClient *s3.Client
	presignClient  *s3.PresignClient
}

// Config holds the parameters for building an S3Storage.
type Config struct {
	Bucket          string
	InternalEndpoint string // e.g. "http://minio:9000"
	PublicEndpoint   string // e.g. "http://localhost:9000"
	Region          string
	AccessKeyID     string
	SecretAccessKey string
}

func NewS3Storage(ctx context.Context, cfg Config) (*S3Storage, error) {
	staticCreds := credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")

	internalCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(staticCreds),
		awsconfig.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: cfg.InternalEndpoint, HostnameImmutable: true}, nil
			}),
		),
	)
	if err != nil {
		return nil, err
	}
	internalClient := s3.NewFromConfig(internalCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	publicCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(staticCreds),
		awsconfig.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: cfg.PublicEndpoint, HostnameImmutable: true}, nil
			}),
		),
	)
	if err != nil {
		return nil, err
	}
	publicClient := s3.NewFromConfig(publicCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &S3Storage{
		bucket:         cfg.Bucket,
		internalClient: internalClient,
		presignClient:  s3.NewPresignClient(publicClient),
	}, nil
}

func (s *S3Storage) PutPresignedURL(ctx context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (string, time.Time, error) {
	req, err := s.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		ContentType:   aws.String(contentType),
		ContentLength: aws.Int64(sizeBytes),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", time.Time{}, err
	}
	return req.URL, time.Now().Add(ttl), nil
}

func (s *S3Storage) GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", time.Time{}, err
	}
	return req.URL, time.Now().Add(ttl), nil
}

func (s *S3Storage) HeadObject(ctx context.Context, key string) (*ObjectInfo, error) {
	out, err := s.internalClient.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	info := &ObjectInfo{
		Key:          key,
		LastModified: aws.ToTime(out.LastModified),
	}
	if out.ContentLength != nil {
		info.SizeBytes = *out.ContentLength
	}
	if out.ContentType != nil {
		info.ContentType = *out.ContentType
	}
	if out.ETag != nil {
		info.ETag = *out.ETag
	}
	return info, nil
}

func (s *S3Storage) DeleteObject(ctx context.Context, key string) error {
	_, err := s.internalClient.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *S3Storage) DeleteObjects(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	objects := make([]types.ObjectIdentifier, len(keys))
	for i, k := range keys {
		objects[i] = types.ObjectIdentifier{Key: aws.String(k)}
	}
	_, err := s.internalClient.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
	})
	return err
}
