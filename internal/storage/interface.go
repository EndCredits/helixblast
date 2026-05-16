package storage

import (
	"context"
	"io"
	"time"
)

type Store interface {
	Put(ctx context.Context, key string, reader io.Reader) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	ListExpired(ctx context.Context, ttl time.Duration) ([]string, error)
	PresignedGetURL(ctx context.Context, key string, ttl time.Duration) (string, error)
	PresignedUploadURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}
