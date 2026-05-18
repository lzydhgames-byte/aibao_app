package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
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
	// Tencent COS requires bucket host to be "<short_name>-<appid>". Operators
	// commonly set BUCKET to the full name (already includes APPID) — detect
	// this and skip the suffix so we don't end up with bucket-appid-appid.
	bucketHost := cfg.Bucket
	if cfg.AppID != "" && !strings.HasSuffix(cfg.Bucket, "-"+cfg.AppID) {
		bucketHost = fmt.Sprintf("%s-%s", cfg.Bucket, cfg.AppID)
	}
	bucketURL := &url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.cos.%s.myqcloud.com", bucketHost, cfg.Region),
	}
	b := &cos.BaseURL{BucketURL: bucketURL}
	// Plan 10 deploy: 香港 → 上海 COS 首次 TLS 握手偶发 > 10s, hitting
	// http.DefaultTransport.TLSHandshakeTimeout (10s default). Explicitly
	// build a transport with 30s TLS timeout + keep-alive so warm connections
	// reuse the TLS session and skip handshake on follow-up uploads.
	inner := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	c := cos.NewClient(b, &http.Client{
		Transport: &cos.AuthorizationTransport{
			Transport: inner,
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

// Download streams the object body. Caller MUST Close the returned reader.
func (s *COSClient) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.c.Object.Get(ctx, key, nil)
	if err != nil {
		var cerr *cos.ErrorResponse
		if errors.As(err, &cerr) && cerr.Response != nil && cerr.Response.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return resp.Body, nil
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
