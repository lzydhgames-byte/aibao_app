package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/aibao/server/internal/gateway/storage"
	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/pkg/logger"
)

// BGMCache returns local file paths for BGM assets, downloading from storage
// on first use. Concurrent calls for the same filename block on a single
// download (per-key sync.Once pattern).
type BGMCache interface {
	GetLocalPath(ctx context.Context, asset *model.BGMAsset) (string, error)
}

type bgmCache struct {
	storage storage.Client
	dir     string

	mu      sync.Mutex
	onceMap map[string]*sync.Once
	errMap  map[string]error
}

// NewBGMCache constructs a cache rooted at dir.
func NewBGMCache(s storage.Client, dir string) BGMCache {
	return &bgmCache{
		storage: s, dir: dir,
		onceMap: map[string]*sync.Once{},
		errMap:  map[string]error{},
	}
}

func (c *bgmCache) GetLocalPath(ctx context.Context, asset *model.BGMAsset) (string, error) {
	if asset == nil {
		return "", errors.New("bgm cache: nil asset")
	}
	local := filepath.Join(c.dir, asset.Filename)

	// Fast path: file exists.
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}

	c.mu.Lock()
	once, ok := c.onceMap[asset.Filename]
	if !ok {
		once = &sync.Once{}
		c.onceMap[asset.Filename] = once
	}
	c.mu.Unlock()

	once.Do(func() {
		err := c.download(ctx, asset, local)
		c.mu.Lock()
		c.errMap[asset.Filename] = err
		c.mu.Unlock()
	})

	c.mu.Lock()
	err := c.errMap[asset.Filename]
	c.mu.Unlock()
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(local); statErr != nil {
		return "", fmt.Errorf("bgm cache: file missing after download: %w", statErr)
	}
	return local, nil
}

func (c *bgmCache) download(ctx context.Context, asset *model.BGMAsset, local string) error {
	lg := logger.FromCtx(ctx).With("module", "bgm_cache", "filename", asset.Filename)
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", c.dir, err)
	}
	body, err := c.storage.Download(ctx, asset.ObjectKey)
	if err != nil {
		return fmt.Errorf("download %s: %w", asset.ObjectKey, err)
	}
	defer body.Close()

	tmp := local + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	n, copyErr := io.Copy(f, body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("copy: %w", copyErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close: %w", closeErr)
	}
	if err := os.Rename(tmp, local); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	lg.Info("bgm.cache.downloaded", "bytes", n)
	return nil
}
