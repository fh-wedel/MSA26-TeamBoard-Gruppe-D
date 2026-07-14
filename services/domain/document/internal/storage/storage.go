// Package storage abstracts S3/MinIO object operations.
package storage

import (
	"context"
	"time"
)

// ObjectStorage is the interface used by the domain service.
type ObjectStorage interface {
	// PutPresignedURL generates a URL clients use to upload an object via PUT.
	PutPresignedURL(ctx context.Context, key, contentType string, sizeBytes int64, ttl time.Duration) (url string, expiresAt time.Time, err error)

	// GetPresignedURL generates a URL clients use to download an object via GET.
	GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (url string, expiresAt time.Time, err error)

	// HeadObject checks whether an object exists and returns its metadata.
	HeadObject(ctx context.Context, key string) (*ObjectInfo, error)

	// DeleteObject removes a single object.
	DeleteObject(ctx context.Context, key string) error

	// DeleteObjects removes multiple objects in a single batch request.
	DeleteObjects(ctx context.Context, keys []string) error
}

// ObjectInfo holds metadata returned by HeadObject.
type ObjectInfo struct {
	Key          string
	SizeBytes    int64
	ContentType  string
	ETag         string
	LastModified time.Time
}
