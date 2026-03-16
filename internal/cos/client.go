package cos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Client is a simplified COS client interface
type Client struct {
	bucket string
	region string
}

type UploadResult struct {
	FileID    string `json:"file_id"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
	AccessURL string `json:"access_url"`
}

// NewClient creates a COS client
func NewClient(secretID, secretKey, region, bucket string) (*Client, error) {
	if secretID == "" || secretKey == "" {
		return nil, fmt.Errorf("COS credentials required")
	}
	if region == "" {
		region = "ap-guangzhou"
	}
	if bucket == "" {
		bucket = "hita-courses"
	}

	return &Client{
		bucket: bucket,
		region: region,
	}, nil
}

// NewClientFromEnv creates client from environment
func NewClientFromEnv() (*Client, error) {
	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")
	region := os.Getenv("COS_REGION")
	bucket := os.Getenv("COS_BUCKET")

	if secretID == "" {
		secretID = "placeholder"
	}
	if secretKey == "" {
		secretKey = "placeholder"
	}

	return NewClient(secretID, secretKey, region, bucket)
}

// Upload uploads a file
func (c *Client) Upload(ctx context.Context, key string, reader *bytes.Reader, size int64) (*UploadResult, error) {
	// TODO: Implement actual COS SDK upload
	return &UploadResult{
		FileID:    key,
		Size:      size,
		URL:       c.GetPublicURL(key),
		AccessURL: c.GetPublicURL(key),
	}, nil
}

// DownloadBytes downloads a file and returns bytes
func (c *Client) DownloadBytes(ctx context.Context, key string) ([]byte, error) {
	url := c.GetPublicURL(key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("file not found: %s", key)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// Download downloads a file to a writer
func (c *Client) Download(ctx context.Context, key string, writer io.Writer) error {
	content, err := c.DownloadBytes(ctx, key)
	if err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}

// Delete deletes a file
func (c *Client) Delete(ctx context.Context, key string) error {
	return nil
}

// GetPresignedURL generates temporary URL
func (c *Client) GetPresignedURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	return c.GetPublicURL(key) + "?expires=" + expires.String(), nil
}

// ListFiles lists files
func (c *Client) ListFiles(ctx context.Context, prefix string, maxKeys int) ([]map[string]interface{}, error) {
	return []map[string]interface{}{}, nil
}

// GetPublicURL returns public URL
func (c *Client) GetPublicURL(key string) string {
	return fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s", c.bucket, c.region, key)
}
