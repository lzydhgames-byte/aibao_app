// Package storage is the object storage abstraction. Audio worker depends on
// this interface, not on any concrete provider.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrNotFound is returned when an object key does not exist.
var ErrNotFound = errors.New("storage object not found")

// ErrUpstream is returned for any other provider-side error.
var ErrUpstream = errors.New("storage upstream error")

// UploadInput is the input to Upload.
type UploadInput struct {
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
}

// ObjectMeta is what HeadObject returns.
type ObjectMeta struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

// Client is the object storage abstraction.
type Client interface {
	Upload(ctx context.Context, in UploadInput) error
	HeadObject(ctx context.Context, key string) (*ObjectMeta, error)
	Delete(ctx context.Context, key string) error
	GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error)
}
