package storage

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// InMemory is an in-process stub used in unit tests.
type InMemory struct {
	mu      sync.Mutex
	objects map[string]*ObjectInfo
}

func NewInMemory() *InMemory {
	return &InMemory{objects: make(map[string]*ObjectInfo)}
}

// Store simulates a client uploading bytes, for use in tests.
func (s *InMemory) Store(key, contentType string, sizeBytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = &ObjectInfo{
		Key:          key,
		SizeBytes:    sizeBytes,
		ContentType:  contentType,
		ETag:         "fake-etag",
		LastModified: time.Now(),
	}
}

func (s *InMemory) PutPresignedURL(_ context.Context, key, _ string, _ int64, ttl time.Duration) (string, time.Time, error) {
	return fmt.Sprintf("http://localhost:9000/test/%s", key), time.Now().Add(ttl), nil
}

func (s *InMemory) GetPresignedURL(_ context.Context, key string, ttl time.Duration) (string, time.Time, error) {
	return fmt.Sprintf("http://localhost:9000/test/%s?download=1", key), time.Now().Add(ttl), nil
}

func (s *InMemory) HeadObject(_ context.Context, key string) (*ObjectInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, ok := s.objects[key]
	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}
	return info, nil
}

func (s *InMemory) DeleteObject(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *InMemory) DeleteObjects(_ context.Context, keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range keys {
		delete(s.objects, k)
	}
	return nil
}
