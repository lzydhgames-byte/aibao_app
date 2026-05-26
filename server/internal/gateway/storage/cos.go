package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
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
	c          *cos.Client
	cfg        COSConfig
	bucketHost string       // resolved host like "aibao-audio-dev-1356733768.cos.ap-shanghai.myqcloud.com"
	httpClient *http.Client // shared client for direct PUTs (Upload bypasses SDK)
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
	// Plan 10 deploy: cos-go-sdk-v5 PUT to ap-shanghai from 香港 hangs at
	// TLS handshake (root cause unknown, possibly chunked encoding + idle
	// session interaction). A bare net/http PUT with manual COS V5
	// signature consistently completes in ~300ms in the same environment.
	// Use the SDK only for Head/Delete/PresignedGet; Upload goes direct.
	httpClient := &http.Client{
		Transport: inner,
		Timeout:   cfg.UploadTimeout,
	}
	return &COSClient{
		c:          c,
		cfg:        cfg,
		bucketHost: bucketURL.Host,
		httpClient: httpClient,
	}, nil
}

// signV5 computes a Tencent COS V5 signature for the given verb + URI.
// Headers / params lists are left empty: Upload only signs the basic PUT.
func signV5(secretID, secretKey, method, uri string, expireSec int64) string {
	now := time.Now().Unix()
	expire := now + expireSec
	keyTime := fmt.Sprintf("%d;%d", now, expire)

	hSignKey := hmac.New(sha1.New, []byte(secretKey))
	hSignKey.Write([]byte(keyTime))
	signKey := hex.EncodeToString(hSignKey.Sum(nil))

	httpString := strings.ToLower(method) + "\n" + uri + "\n\n\n"
	h1 := sha1.New()
	h1.Write([]byte(httpString))
	stringToSign := "sha1\n" + keyTime + "\n" + hex.EncodeToString(h1.Sum(nil)) + "\n"

	h2 := hmac.New(sha1.New, []byte(signKey))
	h2.Write([]byte(stringToSign))
	sig := hex.EncodeToString(h2.Sum(nil))

	return fmt.Sprintf(
		"q-sign-algorithm=sha1&q-ak=%s&q-sign-time=%s&q-key-time=%s&q-header-list=&q-url-param-list=&q-signature=%s",
		secretID, keyTime, keyTime, sig,
	)
}

// Upload PUTs the object body via direct net/http (bypassing cos-go-sdk-v5).
// SDK path was hanging at TLS handshake from 香港 to ap-shanghai despite a
// custom transport; bare PUT with manual V5 signature completes in ~300ms.
func (s *COSClient) Upload(ctx context.Context, in UploadInput) (int64, error) {
	uri := "/" + strings.TrimPrefix(in.Key, "/")
	urlStr := "https://" + s.bucketHost + uri

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, urlStr, in.Body)
	if err != nil {
		return 0, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	if in.ContentType != "" {
		req.Header.Set("Content-Type", in.ContentType)
	}
	if in.Size > 0 {
		req.ContentLength = in.Size
	}
	req.Header.Set("Authorization", signV5(s.cfg.SecretID, s.cfg.SecretKey, http.MethodPut, uri, 300))

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("%w: HTTP %d: %s", ErrUpstream, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return in.Size, nil
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
