package tempstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	DefaultTTL      = 24 * time.Hour
	MaxFileSize     = 50 * 1024 * 1024 // 50MB
	CleanupInterval = 1 * time.Hour
)

type Store struct {
	baseDir string
	ttl     time.Duration
	mu      sync.RWMutex
	meta    map[string]*FileMeta
	stopCh  chan struct{}
}

type FileMeta struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	MimeType   string    `json:"mime_type"`
	UploadedAt time.Time `json:"uploaded_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func New(baseDir string, ttl time.Duration) (*Store, error) {
	if ttl == 0 {
		ttl = DefaultTTL
	}

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}

	s := &Store{
		baseDir: baseDir,
		ttl:     ttl,
		meta:    make(map[string]*FileMeta),
		stopCh:  make(chan struct{}),
	}

	s.loadExistingMeta()
	go s.cleanupLoop()

	return s, nil
}

func (s *Store) Save(name string, mimeType string, reader io.Reader) (*FileMeta, error) {
	id, err := generateID()
	if err != nil {
		return nil, fmt.Errorf("generate id: %w", err)
	}

	filePath := filepath.Join(s.baseDir, id)
	f, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}

	written, err := io.Copy(f, reader)
	f.Close()
	if err != nil {
		os.Remove(filePath)
		return nil, fmt.Errorf("write file: %w", err)
	}

	if written > MaxFileSize {
		os.Remove(filePath)
		return nil, fmt.Errorf("file too large: %d > %d", written, MaxFileSize)
	}

	now := time.Now()
	meta := &FileMeta{
		ID:         id,
		Name:       name,
		Size:       written,
		MimeType:   mimeType,
		UploadedAt: now,
		ExpiresAt:  now.Add(s.ttl),
	}

	s.mu.Lock()
	s.meta[id] = meta
	s.mu.Unlock()

	s.saveMeta()

	return meta, nil
}

func (s *Store) Get(id string) (*FileMeta, io.ReadCloser, error) {
	s.mu.RLock()
	meta, ok := s.meta[id]
	s.mu.RUnlock()

	if !ok {
		return nil, nil, fmt.Errorf("file not found: %s", id)
	}

	if time.Now().After(meta.ExpiresAt) {
		s.delete(id)
		return nil, nil, fmt.Errorf("file expired: %s", id)
	}

	filePath := filepath.Join(s.baseDir, id)
	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open file: %w", err)
	}

	return meta, f, nil
}

func (s *Store) GetMeta(id string) (*FileMeta, error) {
	s.mu.RLock()
	meta, ok := s.meta[id]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("file not found: %s", id)
	}

	if time.Now().After(meta.ExpiresAt) {
		s.delete(id)
		return nil, fmt.Errorf("file expired: %s", id)
	}

	return meta, nil
}

func (s *Store) List() []*FileMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	result := make([]*FileMeta, 0, len(s.meta))
	for _, m := range s.meta {
		if now.Before(m.ExpiresAt) {
			result = append(result, m)
		}
	}
	return result
}

func (s *Store) Delete(id string) error {
	return s.delete(id)
}

func (s *Store) Close() {
	close(s.stopCh)
}

func (s *Store) delete(id string) error {
	s.mu.Lock()
	delete(s.meta, id)
	s.mu.Unlock()

	filePath := filepath.Join(s.baseDir, id)
	err := os.Remove(filePath)
	s.saveMeta()
	return err
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

func (s *Store) cleanup() {
	now := time.Now()
	s.mu.Lock()
	var expired []string
	for id, m := range s.meta {
		if now.After(m.ExpiresAt) {
			expired = append(expired, id)
		}
	}
	s.mu.Unlock()

	for _, id := range expired {
		s.delete(id)
	}

	if len(expired) > 0 {
		fmt.Printf("[tempstore] cleaned up %d expired files\n", len(expired))
	}
}

func (s *Store) saveMeta() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metaPath := filepath.Join(s.baseDir, "_meta.json")
	data := make(map[string]*FileMeta, len(s.meta))
	for k, v := range s.meta {
		data[k] = v
	}

	f, err := os.Create(metaPath)
	if err != nil {
		return
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.Encode(data)
}

func (s *Store) loadExistingMeta() {
	metaPath := filepath.Join(s.baseDir, "_meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return
	}

	var loaded map[string]*FileMeta
	if json.Unmarshal(data, &loaded) != nil {
		return
	}

	s.mu.Lock()
	s.meta = loaded
	s.mu.Unlock()
}

func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
