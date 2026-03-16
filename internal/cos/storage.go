package cos

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"
)

// Storage wraps COS operations with quota management
type Storage struct {
	client     *Client
	maxSize    int64
	quotaLimit int64
	usedToday  int64
}

// NewStorage creates storage instance
func NewStorage(client *Client, maxSize int64) *Storage {
	if maxSize == 0 {
		maxSize = 10 * 1024 * 1024 // 10MB
	}
	return &Storage{
		client:     client,
		maxSize:    maxSize,
		quotaLimit: 10 * 1024 * 1024 * 1024, // 10GB
		usedToday:  0,
	}
}

// NewDefaultStorage creates storage with defaults
func NewDefaultStorage() *Storage {
	client, _ := NewClientFromEnv()
	return NewStorage(client, 10*1024*1024)
}

// SaveFile saves bytes to COS
func (s *Storage) SaveFile(ctx context.Context, key string, content []byte, contentType string) (*UploadResult, error) {
	size := int64(len(content))

	if size > s.maxSize {
		return nil, fmt.Errorf("file too large: %d bytes (max: %d)", size, s.maxSize)
	}
	if s.usedToday+size > s.quotaLimit {
		return nil, fmt.Errorf("quota exceeded")
	}

	result, err := s.client.Upload(ctx, key, bytes.NewReader(content), size)
	if err != nil {
		return nil, err
	}

	s.usedToday += size
	return result, nil
}

// SaveFileFromPath saves file from disk
func (s *Storage) SaveFileFromPath(ctx context.Context, key string, filePath string, contentType string) (*UploadResult, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return s.SaveFile(ctx, key, content, contentType)
}

// DownloadFile downloads to disk
func (s *Storage) DownloadFile(ctx context.Context, key string, localPath string) error {
	content, err := s.DownloadBytes(ctx, key)
	if err != nil {
		return err
	}
	return os.WriteFile(localPath, content, 0644)
}

// DownloadBytes downloads file and returns bytes
func (s *Storage) DownloadBytes(ctx context.Context, key string) ([]byte, error) {
	return s.client.DownloadBytes(ctx, key)
}

// GetDownloadURL returns a download URL (presigned or public)
func (s *Storage) GetDownloadURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	return s.client.GetPresignedURL(ctx, key, expires)
}

// DeleteFile deletes from COS
func (s *Storage) DeleteFile(ctx context.Context, key string) error {
	return s.client.Delete(ctx, key)
}

// ListFiles lists files
func (s *Storage) ListFiles(ctx context.Context, prefix string, maxKeys int) ([]map[string]interface{}, error) {
	return s.client.ListFiles(ctx, prefix, maxKeys)
}

// GetPresignedURL gets temporary URL
func (s *Storage) GetPresignedURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	return s.client.GetPresignedURL(ctx, key, expires)
}

// GetQuota returns usage
func (s *Storage) GetQuota() (used int64, limit int64) {
	return s.usedToday, s.quotaLimit
}

// ResetDailyQuota resets quota
func (s *Storage) ResetDailyQuota() {
	s.usedToday = 0
}
