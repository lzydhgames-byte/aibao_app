package storage

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

// COSConfig holds settings for the Tencent COS client.
type COSConfig struct {
	Bucket        string
	Region        string
	AppID         string
	SecretID      string
	SecretKey     string
	UploadTimeout time.Duration
}

// COSClient implements Client over Tencent Cloud COS.
type COSClient struct {
	c   *cos.Client
	cfg COSConfig
}

// NewCOS constructs a COSClient.
func NewCOS(cfg COSConfig) (*COSClient, error) {
	if cfg.Bucket == "" || cfg.Region == "" {
		return nil, errors.New("cos: bucket and region are required")
	}
	if cfg.SecretID == "" || cfg.SecretKey == "" {
		return nil, errors.New("cos: secret_id/secret_key required (set AIBAO_STORAGE_COS_SECRET_*)")
	}
	if cfg.UploadTimeout == 0 {
		cfg.UploadTimeout = 30 * time.Second
	}
	bucketHost := cfg.Bucket
	if cfg.AppID != "" {
		bucketHost = fmt.Sprintf("%s-%s", cfg.Bucket, cfg.AppID)
	}
	bucketURL := &url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.cos.%s.myqcloud.com", bucketHost, cfg.Region),
	}
	b := &cos.BaseURL{BucketURL: bucketURL}
	c := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  cfg.SecretID,
			SecretKey: cfg.SecretKey,
		},
		Timeout: cfg.UploadTimeout,
	})
	return &COSClient{c: c, cfg: cfg}, nil
}

// Upload PUTs the object body.
func (s *COSClient) Upload(ctx context.Context, in UploadInput) error {
	opts := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentType: in.ContentType,
		},
	}
	if in.Size > 0 {
		opts.ObjectPutHeaderOptions.ContentLength = in.Size
	}
	_, err := s.c.Object.Put(ctx, in.Key, in.Body, opts)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return nil
}

// HeadObject inspects object metadata.
func (s *COSClient) HeadObject(ctx context.Context, key string) (*ObjectMeta, error) {
	resp, err := s.c.Object.Head(ctx, key, nil)
	if err != nil {
		var cerr *cos.ErrorResponse
		if errors.As(err, &cerr) && cerr.Response != nil && cerr.Response.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return &ObjectMeta{
		Key:          key,
		Size:         resp.ContentLength,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: time.Now(),
	}, nil
}

// Delete removes the object.
func (s *COSClient) Delete(ctx context.Context, key string) error {
	_, err := s.c.Object.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return nil
}

// GetPresignedURL returns a presigned GET URL valid for ttl.
func (s *COSClient) GetPresignedURL(ctx context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	u, err := s.c.Object.GetPresignedURL(ctx, http.MethodGet, key, s.cfg.SecretID, s.cfg.SecretKey, ttl, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: presign: %v", ErrUpstream, err)
	}
	return u.String(), time.Now().Add(ttl), nil
}
