package skills

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"hoa-agent-backend/internal/cos"
)

// FileUploadTask represents a single file upload task
type FileUploadTask struct {
	Key           string `json:"key"`
	ContentBase64 string `json:"content_base64"`
	ContentType   string `json:"content_type"`
	URL           string `json:"url,omitempty"`
	Text          string `json:"text,omitempty"`
}

// FileUploadResult represents the result of a single file upload
type FileUploadResult struct {
	Key       string `json:"key"`
	Success   bool   `json:"success"`
	Skipped   bool   `json:"skipped,omitempty"`
	Error     string `json:"error,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	Size      int64  `json:"size,omitempty"`
	URL       string `json:"url,omitempty"`
	AccessURL string `json:"access_url,omitempty"`
	RagChunks int64  `json:"rag_chunks,omitempty"`
}

// NewFilesUploadSkill creates a skill for uploading files to storage with batch support.
// Supports multiple sources: base64 content, URL, or direct binary.
// Now supports concurrent batch uploads for better performance.
func NewFilesUploadSkill(storage *cos.Storage) Skill {
	return Skill{
		Name:    "files.upload",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Check for batch upload
			if files, ok := input["files"].([]any); ok && len(files) > 0 {
				return uploadFilesBatch(ctx, files, storage)
			}

			// Single file upload (backward compatibility)
			return uploadSingleFile(ctx, input, storage)
		},
	}
}

// uploadSingleFile handles single file upload
func uploadSingleFile(ctx context.Context, input map[string]any, storage *cos.Storage) (map[string]any, error) {
	key, ok := input["key"].(string)
	if !ok || key == "" {
		return nil, &InvokeError{Code: "INVALID_INPUT", Message: "key is required", Retryable: false}
	}

	contentType, _ := input["content_type"].(string)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var content []byte
	var source string

	if b64, ok := input["content_base64"].(string); ok && b64 != "" {
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, &InvokeError{Code: "INVALID_INPUT", Message: "invalid base64 content: " + err.Error(), Retryable: false}
		}
		content = data
		source = "base64"
	} else if url, ok := input["url"].(string); ok && url != "" {
		data, err := downloadFromURL(ctx, url)
		if err != nil {
			return nil, &InvokeError{Code: "DOWNLOAD_FAILED", Message: err.Error(), Retryable: true}
		}
		content = data
		source = "url"
	} else if text, ok := input["text"].(string); ok && text != "" {
		content = []byte(text)
		source = "text"
	} else {
		return nil, &InvokeError{Code: "INVALID_INPUT", Message: "content_base64, url, or text is required", Retryable: false}
	}

	// SHA256 deduplication check
	contentHash := sha256.Sum256(content)
	sha256Hex := hex.EncodeToString(contentHash[:])
	dedupStore, err := NewDedupStoreFromEnv()
	if err == nil {
		defer dedupStore.Close()
		shouldIngest, existing, _ := dedupStore.ShouldIngest(ctx, sha256Hex)
		if !shouldIngest {
			return map[string]any{
				"success":       true,
				"skipped":       true,
				"duplicate_sha": sha256Hex[:12],
				"existing_cos":  existing.COSKey,
				"source":        source,
			}, nil
		}
	}

	result, err := storage.SaveFile(ctx, key, content, contentType)
	if err != nil {
		return nil, &InvokeError{Code: "UPLOAD_FAILED", Message: err.Error(), Retryable: true}
	}

	resp := map[string]any{
		"success":    true,
		"file_id":    result.FileID,
		"size":       result.Size,
		"url":        result.URL,
		"access_url": result.AccessURL,
		"source":     source,
		"sha256":     sha256Hex[:12],
	}

	// Record to dedup store
	if dedupStore != nil {
		_ = dedupStore.Record(ctx, sha256Hex, key, result.Size)
	}

	// Optional: direct RAG ingest
	if autoRag, ok := input["auto_ingest_rag"]; ok && autoRag == true {
		qdrant, err := NewQdrantClientFromEnv()
		if err == nil {
			embedder, err := NewEmbeddingProviderFromEnv()
			if err == nil {
				ingested, iErr := IngestMarkdownDirect(ctx, content, key, "files/"+key, storage, GetMCPRegistry(), qdrant, embedder)
				if iErr == nil {
					resp["rag_chunks"] = ingested
				}
			}
		}
	}

	return resp, nil
}

// uploadFilesBatch handles concurrent batch file uploads
func uploadFilesBatch(ctx context.Context, files []any, storage *cos.Storage) (map[string]any, error) {
	workers := 4 // Default concurrent workers
	if len(files) < workers {
		workers = len(files)
	}

	type task struct {
		index int
		file  map[string]any
	}

	taskChan := make(chan task, len(files))
	resultChan := make(chan FileUploadResult, len(files))

	var successCount, failCount int64

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for t := range taskChan {
				result := processUploadTask(ctx, t.file, storage)
				resultChan <- result
				if result.Success {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&failCount, 1)
				}
			}
		}(i)
	}

	// Send tasks
	go func() {
		for i, f := range files {
			if fileMap, ok := f.(map[string]any); ok {
				taskChan <- task{index: i, file: fileMap}
			}
		}
		close(taskChan)
	}()

	// Close result channel when done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	results := make([]FileUploadResult, 0, len(files))
	for r := range resultChan {
		results = append(results, r)
	}

	return map[string]any{
		"success":       atomic.LoadInt64(&successCount) == int64(len(files)),
		"total":         len(files),
		"success_count": atomic.LoadInt64(&successCount),
		"fail_count":    atomic.LoadInt64(&failCount),
		"results":       results,
	}, nil
}

// processUploadTask processes a single upload task
func processUploadTask(ctx context.Context, file map[string]any, storage *cos.Storage) FileUploadResult {
	key, _ := file["key"].(string)
	if key == "" {
		return FileUploadResult{Key: key, Success: false, Error: "key is required"}
	}

	contentType, _ := file["content_type"].(string)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var content []byte

	if b64, ok := file["content_base64"].(string); ok && b64 != "" {
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return FileUploadResult{Key: key, Success: false, Error: "invalid base64: " + err.Error()}
		}
		content = data
	} else if url, ok := file["url"].(string); ok && url != "" {
		data, err := downloadFromURL(ctx, url)
		if err != nil {
			return FileUploadResult{Key: key, Success: false, Error: "download failed: " + err.Error()}
		}
		content = data
	} else if text, ok := file["text"].(string); ok && text != "" {
		content = []byte(text)
	} else {
		return FileUploadResult{Key: key, Success: false, Error: "content_base64, url, or text is required"}
	}

	contentHash := sha256.Sum256(content)
	sha256Hex := hex.EncodeToString(contentHash[:])

	// Dedup check
	dedupStore, _ := NewDedupStoreFromEnv()
	if dedupStore != nil {
		defer dedupStore.Close()
		shouldIngest, existing, _ := dedupStore.ShouldIngest(ctx, sha256Hex)
		if !shouldIngest {
			return FileUploadResult{
				Key:     key,
				Success: true,
				Skipped: true,
				Error:   fmt.Sprintf("duplicate SHA256: %s (COS: %s)", sha256Hex[:12], existing.COSKey),
			}
		}
	}

	result, err := storage.SaveFile(ctx, key, content, contentType)
	if err != nil {
		return FileUploadResult{Key: key, Success: false, Error: err.Error()}
	}

	resp := FileUploadResult{
		Key:     key,
		Success: true,
		FileID:  result.FileID,
		Size:    result.Size,
		URL:     result.URL,
	}

	// Record to dedup store
	if dedupStore != nil {
		_ = dedupStore.Record(ctx, sha256Hex, key, result.Size)
	}

	// Optional: direct RAG ingest
	if autoRag, ok := file["auto_ingest_rag"]; ok && autoRag == true {
		qdrant, err := NewQdrantClientFromEnv()
		if err == nil {
			embedder, err := NewEmbeddingProviderFromEnv()
			if err == nil {
				ingested, iErr := IngestMarkdownDirect(ctx, content, key, "files/"+key, storage, GetMCPRegistry(), qdrant, embedder)
				if iErr == nil {
					resp.RagChunks = int64(ingested)
				}
			}
		}
	}

	return resp
}

// NewFilesDownloadSkill creates a skill for downloading files from storage.
// Now supports batch download with concurrency.
func NewFilesDownloadSkill(storage *cos.Storage) Skill {
	return Skill{
		Name:    "files.download",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Check for batch download
			if keys, ok := input["keys"].([]any); ok && len(keys) > 0 {
				return downloadFilesBatch(ctx, keys, input, storage)
			}

			// Single file download (backward compatibility)
			return downloadSingleFile(ctx, input, storage)
		},
	}
}

// downloadSingleFile handles single file download
func downloadSingleFile(ctx context.Context, input map[string]any, storage *cos.Storage) (map[string]any, error) {
	key, _ := input["key"].(string)
	if key == "" {
		return nil, &InvokeError{Code: "INVALID_INPUT", Message: "key is required", Retryable: false}
	}

	format, _ := input["format"].(string)
	if format == "" {
		format = "url"
	}

	switch format {
	case "url":
		expires := int64(3600)
		if e, ok := input["expires_seconds"].(float64); ok {
			expires = int64(e)
		}

		url, err := storage.GetDownloadURL(ctx, key, time.Duration(expires)*time.Second)
		if err != nil {
			return nil, &InvokeError{Code: "INTERNAL", Message: err.Error(), Retryable: true}
		}

		return map[string]any{
			"success":         true,
			"url":             url,
			"expires_seconds": expires,
			"key":             key,
		}, nil

	case "base64":
		content, err := storage.DownloadBytes(ctx, key)
		if err != nil {
			return nil, &InvokeError{Code: "DOWNLOAD_FAILED", Message: err.Error(), Retryable: true}
		}

		return map[string]any{
			"success":        true,
			"content_base64": base64.StdEncoding.EncodeToString(content),
			"size":           len(content),
			"key":            key,
		}, nil

	default:
		return nil, &InvokeError{Code: "INVALID_INPUT", Message: "invalid format, use 'url' or 'base64'", Retryable: false}
	}
}

// downloadFilesBatch handles concurrent batch file downloads
func downloadFilesBatch(ctx context.Context, keys []any, input map[string]any, storage *cos.Storage) (map[string]any, error) {
	format, _ := input["format"].(string)
	if format == "" {
		format = "url"
	}

	workers := 4
	if len(keys) < workers {
		workers = len(keys)
	}

	type task struct {
		index int
		key   string
	}

	taskChan := make(chan task, len(keys))
	resultChan := make(chan map[string]any, len(keys))

	var successCount, failCount int64

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range taskChan {
				result := processDownloadTask(ctx, t.key, format, input, storage)
				resultChan <- result
				if result["success"].(bool) {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&failCount, 1)
				}
			}
		}()
	}

	// Send tasks
	go func() {
		for i, k := range keys {
			if key, ok := k.(string); ok {
				taskChan <- task{index: i, key: key}
			}
		}
		close(taskChan)
	}()

	// Close result channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	results := make([]map[string]any, 0, len(keys))
	for r := range resultChan {
		results = append(results, r)
	}

	return map[string]any{
		"success":       atomic.LoadInt64(&successCount) == int64(len(keys)),
		"total":         len(keys),
		"success_count": atomic.LoadInt64(&successCount),
		"fail_count":    atomic.LoadInt64(&failCount),
		"format":        format,
		"results":       results,
	}, nil
}

// processDownloadTask processes a single download task
func processDownloadTask(ctx context.Context, key, format string, input map[string]any, storage *cos.Storage) map[string]any {
	switch format {
	case "url":
		expires := int64(3600)
		if e, ok := input["expires_seconds"].(float64); ok {
			expires = int64(e)
		}

		url, err := storage.GetDownloadURL(ctx, key, time.Duration(expires)*time.Second)
		if err != nil {
			return map[string]any{
				"key":     key,
				"success": false,
				"error":   err.Error(),
			}
		}

		return map[string]any{
			"key":             key,
			"success":         true,
			"url":             url,
			"expires_seconds": expires,
		}

	case "base64":
		content, err := storage.DownloadBytes(ctx, key)
		if err != nil {
			return map[string]any{
				"key":     key,
				"success": false,
				"error":   err.Error(),
			}
		}

		return map[string]any{
			"key":            key,
			"success":        true,
			"content_base64": base64.StdEncoding.EncodeToString(content),
			"size":           len(content),
		}

	default:
		return map[string]any{
			"key":     key,
			"success": false,
			"error":   "invalid format",
		}
	}
}

// downloadFromURL downloads content from a URL
func downloadFromURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
