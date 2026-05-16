package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Store struct {
	client *minio.Client
	bucket string
}

func NewS3Store(endpoint, bucket, accessKey, secretKey string) (*S3Store, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}

	exists, err := client.BucketExists(context.Background(), bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("bucket %s does not exist", bucket)
	}

	return &S3Store{client: client, bucket: bucket}, nil
}

func (s *S3Store) Put(ctx context.Context, key string, reader io.Reader) error {
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, -1, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (s *S3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	return obj, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (s *S3Store) ListExpired(ctx context.Context, ttl time.Duration) ([]string, error) {
	var expired []string
	cutoff := time.Now().Add(-ttl)

	objects := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{})
	for obj := range objects {
		if obj.Err != nil {
			return nil, fmt.Errorf("list objects: %w", obj.Err)
		}
		if obj.LastModified.Before(cutoff) {
			expired = append(expired, obj.Key)
		}
	}
	return expired, nil
}

func (s *S3Store) PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, reqParams)
	if err != nil {
		return "", fmt.Errorf("generate presigned url: %w", err)
	}
	return presignedURL.String(), nil
}

func (s *S3Store) PresignedUploadURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	presignedURL, err := s.client.PresignedPutObject(ctx, s.bucket, key, ttl)
	if err != nil {
		return "", fmt.Errorf("generate presigned upload url: %w", err)
	}
	return presignedURL.String(), nil
}

var _ Store = (*S3Store)(nil)
var _ Store = (*LocalStore)(nil)
