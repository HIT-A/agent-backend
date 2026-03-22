package cos

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/tencentyun/cos-go-sdk-v5"
)

type Client struct {
	client *cos.Client
	bucket string
	region string
}

type UploadResult struct {
	FileID    string `json:"file_id"`
	Size      int64  `json:"size"`
	URL       string `json:"url"`
	AccessURL string `json:"access_url"`
}

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

	u, err := url.Parse(fmt.Sprintf("https://%s.cos.%s.myqcloud.com", bucket, region))
	if err != nil {
		return nil, fmt.Errorf("invalid COS URL: %w", err)
	}

	baseURL := &cos.BaseURL{
		BucketURL: u,
	}

	client := cos.NewClient(baseURL, &http.Client{
		Transport: &cos.AuthorizationTransport{
			SecretID:  secretID,
			SecretKey: secretKey,
		},
	})

	return &Client{
		client: client,
		bucket: bucket,
		region: region,
	}, nil
}

func NewClientFromEnv() (*Client, error) {
	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")
	region := os.Getenv("COS_REGION")
	bucket := os.Getenv("COS_BUCKET")

	if secretID == "" || secretKey == "" {
		return nil, fmt.Errorf("COS credentials required: set COS_SECRET_ID and COS_SECRET_KEY")
	}

	return NewClient(secretID, secretKey, region, bucket)
}

func (c *Client) Upload(ctx context.Context, key string, reader *bytes.Reader, size int64) (*UploadResult, error) {
	if c.client == nil {
		return nil, fmt.Errorf("COS client not initialized")
	}

	opts := &cos.ObjectPutOptions{
		ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{
			ContentLength: size,
		},
	}

	_, err := c.client.Object.Put(ctx, key, reader, opts)
	if err != nil {
		return nil, fmt.Errorf("COS upload failed: %w", err)
	}

	return &UploadResult{
		FileID:    key,
		Size:      size,
		URL:       c.GetPublicURL(key),
		AccessURL: c.GetPublicURL(key),
	}, nil
}

func (c *Client) DownloadBytes(ctx context.Context, key string) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("COS client not initialized")
	}

	resp, err := c.client.Object.Get(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("COS download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("file not found: %s", key)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("COS download failed: status=%d", resp.StatusCode)
	}

	// Limit to 100MB max to prevent OOM
	const maxDownloadSize = 100 * 1024 * 1024
	limitedReader := io.LimitReader(resp.Body, maxDownloadSize)

	return io.ReadAll(limitedReader)
}

func (c *Client) Download(ctx context.Context, key string, writer io.Writer) error {
	content, err := c.DownloadBytes(ctx, key)
	if err != nil {
		return err
	}
	_, err = writer.Write(content)
	return err
}

func (c *Client) Delete(ctx context.Context, key string) error {
	if c.client == nil {
		return fmt.Errorf("COS client not initialized")
	}

	_, err := c.client.Object.Delete(ctx, key, nil)
	if err != nil {
		return fmt.Errorf("COS delete failed: %w", err)
	}

	return nil
}

func (c *Client) GetPresignedURL(ctx context.Context, key string, expires time.Duration) (string, error) {
	if c.client == nil {
		return "", fmt.Errorf("COS client not initialized")
	}

	secretID := os.Getenv("COS_SECRET_ID")
	secretKey := os.Getenv("COS_SECRET_KEY")

	presignedURL, err := c.client.Object.GetPresignedURL(ctx, http.MethodGet, key, secretID, secretKey, expires, nil)
	if err != nil {
		return "", fmt.Errorf("generate presigned URL failed: %w", err)
	}

	return presignedURL.String(), nil
}

func (c *Client) ListFiles(ctx context.Context, prefix string, maxKeys int) ([]map[string]interface{}, error) {
	if c.client == nil {
		return nil, fmt.Errorf("COS client not initialized")
	}

	if maxKeys <= 0 {
		maxKeys = 100
	}
	if maxKeys > 1000 {
		maxKeys = 1000
	}

	opt := &cos.BucketGetOptions{
		Prefix:  prefix,
		MaxKeys: maxKeys,
	}

	var results []map[string]interface{}

	for {
		v, _, err := c.client.Bucket.Get(ctx, opt)
		if err != nil {
			return nil, fmt.Errorf("COS list failed: %w", err)
		}

		for _, content := range v.Contents {
			results = append(results, map[string]interface{}{
				"key":           content.Key,
				"size":          content.Size,
				"last_modified": content.LastModified,
				"etag":          content.ETag,
			})
		}

		if !v.IsTruncated {
			break
		}

		opt.Marker = v.NextMarker
		if opt.Marker == "" {
			break
		}
	}

	return results, nil
}

func (c *Client) GetPublicURL(key string) string {
	return fmt.Sprintf("https://%s.cos.%s.myqcloud.com/%s", c.bucket, c.region, key)
}
