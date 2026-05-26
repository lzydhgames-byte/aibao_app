package storage

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockStorage_RoundTrip(t *testing.T) {
	var _ Client = NewMock()
	m := NewMock()
	n, err := m.Upload(context.Background(), UploadInput{
		Key: "a.mp3", Body: bytes.NewReader([]byte("hello")), Size: 5, ContentType: "audio/mpeg",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)
	meta, err := m.HeadObject(context.Background(), "a.mp3")
	require.NoError(t, err)
	assert.Equal(t, int64(5), meta.Size)

	url, exp, err := m.GetPresignedURL(context.Background(), "a.mp3", 10*time.Minute)
	require.NoError(t, err)
	assert.Contains(t, url, "a.mp3")
	assert.True(t, exp.After(time.Now()))

	require.NoError(t, m.Delete(context.Background(), "a.mp3"))
	_, err = m.HeadObject(context.Background(), "a.mp3")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestMockStorage_FailNext(t *testing.T) {
	m := NewMock()
	m.FailNext()
	_, err := m.Upload(context.Background(), UploadInput{Key: "x", Body: bytes.NewReader([]byte("y"))})
	assert.ErrorIs(t, err, ErrUpstream)
}

func TestMockStorage_PresignNotFound(t *testing.T) {
	m := NewMock()
	_, _, err := m.GetPresignedURL(context.Background(), "missing", time.Minute)
	assert.ErrorIs(t, err, ErrNotFound)
}
