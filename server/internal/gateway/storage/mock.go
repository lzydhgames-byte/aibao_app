package storage

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// MockClient is an in-memory storage for tests.
type MockClient struct {
	mu       sync.Mutex
	objects  map[string][]byte
	failNext bool
}

// NewMock constructs a MockClient.
func NewMock() *MockClient {
	return &MockClient{objects: map[string][]byte{}}
}

// FailNext makes the next operation return ErrUpstream.
func (m *MockClient) FailNext() { m.failNext = true }

// Upload stores body bytes under key.
func (m *MockClient) Upload(_ context.Context, in UploadInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failNext {
		m.failNext = false
		return ErrUpstream
	}
	b, err := io.ReadAll(in.Body)
	if err != nil {
		return err
	}
	m.objects[in.Key] = b
	return nil
}

// HeadObject returns size + content-type or ErrNotFound.
func (m *MockClient) HeadObject(_ context.Context, key string) (*ObjectMeta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	return &ObjectMeta{
		Key: key, Size: int64(len(b)), ContentType: "audio/mpeg",
		ETag: "mock", LastModified: time.Now(),
	}, nil
}

// Delete removes the object.
func (m *MockClient) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

// GetPresignedURL returns a fake but parsable URL with the expiry baked in.
func (m *MockClient) GetPresignedURL(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[key]; !ok {
		return "", time.Time{}, ErrNotFound
	}
	if ttl == 0 {
		ttl = 15 * time.Minute
	}
	exp := time.Now().Add(ttl)
	return fmt.Sprintf("http://mock-storage.local/%s?expires=%d", key, exp.Unix()), exp, nil
}

// Read is a test-only helper.
func (m *MockClient) Read(key string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	b := m.objects[key]
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// Has is a test-only helper.
func (m *MockClient) Has(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.objects[key]
	return ok
}
