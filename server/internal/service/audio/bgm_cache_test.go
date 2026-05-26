package audio_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/audio"
	"github.com/stretchr/testify/require"
)

type fakeStorage struct {
	downloads int64
	body      []byte
	err       error
	delay     time.Duration
}

func (f *fakeStorage) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	atomic.AddInt64(&f.downloads, 1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(bytes.NewReader(f.body)), nil
}

func (f *fakeStorage) Upload(_ context.Context, _ storage.UploadInput) (int64, error) {
	return 0, nil
}
func (f *fakeStorage) HeadObject(_ context.Context, _ string) (*storage.ObjectMeta, error) {
	return nil, storage.ErrNotFound
}
func (f *fakeStorage) Delete(_ context.Context, _ string) error { return nil }
func (f *fakeStorage) GetPresignedURL(_ context.Context, _ string, _ time.Duration) (string, time.Time, error) {
	return "", time.Time{}, nil
}

func TestBGMCache_DownloadsOnFirstUse(t *testing.T) {
	dir := t.TempDir()
	fs := &fakeStorage{body: []byte("FAKEMP3")}
	c := audio.NewBGMCache(fs, dir)
	asset := &model.BGMAsset{Filename: "warm_01.mp3", ObjectKey: "bgm/warm/warm_01.mp3"}

	p1, err := c.GetLocalPath(context.Background(), asset)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "warm_01.mp3"), p1)

	b, _ := os.ReadFile(p1)
	require.Equal(t, []byte("FAKEMP3"), b)
	require.EqualValues(t, 1, atomic.LoadInt64(&fs.downloads))

	p2, err := c.GetLocalPath(context.Background(), asset)
	require.NoError(t, err)
	require.Equal(t, p1, p2)
	require.EqualValues(t, 1, atomic.LoadInt64(&fs.downloads))
}

func TestBGMCache_CacheHit_PrecreatedFile(t *testing.T) {
	dir := t.TempDir()
	asset := &model.BGMAsset{Filename: "warm_02.mp3", ObjectKey: "bgm/warm/warm_02.mp3"}
	require.NoError(t, os.WriteFile(filepath.Join(dir, asset.Filename), []byte("PRE"), 0o644))

	fs := &fakeStorage{body: []byte("FAKEMP3")}
	c := audio.NewBGMCache(fs, dir)
	p, err := c.GetLocalPath(context.Background(), asset)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, asset.Filename), p)
	require.EqualValues(t, 0, atomic.LoadInt64(&fs.downloads))
}

func TestBGMCache_ConcurrentSameFile_OnlyOneDownload(t *testing.T) {
	dir := t.TempDir()
	fs := &fakeStorage{body: []byte("FAKEMP3"), delay: 50 * time.Millisecond}
	c := audio.NewBGMCache(fs, dir)
	asset := &model.BGMAsset{Filename: "warm_03.mp3", ObjectKey: "bgm/warm/warm_03.mp3"}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.GetLocalPath(context.Background(), asset)
			require.NoError(t, err)
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, atomic.LoadInt64(&fs.downloads))
}

func TestBGMCache_DownloadError_Propagates(t *testing.T) {
	dir := t.TempDir()
	fs := &fakeStorage{err: errors.New("net down")}
	c := audio.NewBGMCache(fs, dir)
	_, err := c.GetLocalPath(context.Background(), &model.BGMAsset{
		Filename: "x.mp3", ObjectKey: "bgm/x.mp3",
	})
	require.Error(t, err)
}
