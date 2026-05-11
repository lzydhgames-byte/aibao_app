package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCOS_NewValidates(t *testing.T) {
	_, err := NewCOS(COSConfig{})
	require.Error(t, err)
	_, err = NewCOS(COSConfig{Bucket: "b", Region: "ap-shanghai"})
	require.Error(t, err)
}

// Signing happens entirely client-side, so we can verify it without ever
// touching real COS.
func TestCOS_PresignURL(t *testing.T) {
	c, err := NewCOS(COSConfig{
		Bucket: "aibao-test", Region: "ap-shanghai",
		AppID: "1234567890", SecretID: "AKID...", SecretKey: "secret",
	})
	require.NoError(t, err)
	u, exp, err := c.GetPresignedURL(context.Background(), "audio/1/2-x.mp3", 5*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, u, "aibao-test-1234567890.cos.ap-shanghai.myqcloud.com")
	assert.Contains(t, u, "audio/1/2-x.mp3")
	assert.Contains(t, u, "q-sign-algorithm=")
	assert.True(t, exp.After(time.Now()))
}

// TestCOS_BucketAlreadyHasAppIDSuffix guards against the "bucket-appid-appid"
// trap: operators commonly set BUCKET to the full COS name (which already
// ends in -<APPID>). The constructor must not double-suffix.
func TestCOS_BucketAlreadyHasAppIDSuffix(t *testing.T) {
	c, err := NewCOS(COSConfig{
		Bucket: "aibao-test-1234567890", Region: "ap-shanghai",
		AppID: "1234567890", SecretID: "AKID...", SecretKey: "secret",
	})
	require.NoError(t, err)
	u, _, err := c.GetPresignedURL(context.Background(), "audio/k.mp3", time.Minute)
	require.NoError(t, err)
	assert.Contains(t, u, "aibao-test-1234567890.cos.ap-shanghai.myqcloud.com")
	assert.NotContains(t, u, "1234567890-1234567890")
}

// Upload/HeadObject/Delete integration with a stub server is skipped:
// cos-go-sdk-v5 strictly verifies the request Host matches BucketURL, so a
// plain httptest stub causes signing-vs-host mismatches. Real provider smoke
// is deferred to Plan 5 Task 15 (manual).
func TestCOS_UploadAgainstStub(t *testing.T) {
	t.Skip("stub-server based COS upload integration test deferred — real smoke in Task 15")
}
