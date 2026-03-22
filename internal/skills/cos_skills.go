package skills

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"hoa-agent-backend/internal/cos"
)

// COSFileTask represents a file operation task
type COSFileTask struct {
	Index         int    `json:"index"`
	Key           string `json:"key"`
	ContentBase64 string `json:"content_base64,omitempty"`
	ContentType   string `json:"content_type,omitempty"`
	Operation     string `json:"operation"` // "save", "delete"
}

// COSFileResult represents the result of a file operation
type COSFileResult struct {
	Index     int    `json:"index"`
	Key       string `json:"key"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
	FileID    string `json:"file_id,omitempty"`
	Size      int64  `json:"size,omitempty"`
	URL       string `json:"url,omitempty"`
	AccessURL string `json:"access_url,omitempty"`
}

// NewCOSSaveFileSkill creates a skill to save files to COS with batch support.
// Supports concurrent batch uploads for better performance.
func NewCOSSaveFileSkill(storage *cos.Storage) Skill {
	return Skill{
		Name:    "cos.save_file",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Check for batch upload
			if files, ok := input["files"].([]any); ok && len(files) > 0 {
				return cosSaveFilesBatch(ctx, files, storage)
			}

			// Single file upload (backward compatibility)
			return cosSaveSingleFile(ctx, input, storage)
		},
	}
}

// cosSaveSingleFile handles single file upload
func cosSaveSingleFile(ctx context.Context, input map[string]any, storage *cos.Storage) (map[string]any, error) {
	key, ok := input["key"].(string)
	if !ok || key == "" {
		return nil, &InvokeError{Code: "INVALID_INPUT", Message: "key is required", Retryable: false}
	}

	contentBase64, ok := input["content_base64"].(string)
	if !ok || contentBase64 == "" {
		return nil, &InvokeError{Code: "INVALID_INPUT", Message: "content_base64 is required", Retryable: false}
	}

	contentType, ok := input["content_type"].(string)
	if !ok || contentType == "" {
		contentType = "application/octet-stream"
	}

	content, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		return nil, &InvokeError{Code: "INVALID_INPUT", Message: fmt.Sprintf("decode base64: %v", err), Retryable: false}
	}

	result, err := storage.SaveFile(ctx, key, content, contentType)
	if err != nil {
		return nil, &InvokeError{Code: "STORAGE_ERROR", Message: err.Error(), Retryable: true}
	}

	return map[string]any{
		"file_id":    result.FileID,
		"size":       result.Size,
		"url":        result.URL,
		"access_url": result.AccessURL,
	}, nil
}

// cosSaveFilesBatch handles concurrent batch file uploads
func cosSaveFilesBatch(ctx context.Context, files []any, storage *cos.Storage) (map[string]any, error) {
	workers := 4
	if len(files) < workers {
		workers = len(files)
	}

	taskChan := make(chan COSFileTask, len(files))
	resultChan := make(chan COSFileResult, len(files))

	var successCount, failCount int64

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range taskChan {
				result := processCOSSaveTask(ctx, task, storage)
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
				task := COSFileTask{
					Index:         i,
					Key:           getString(fileMap, "key"),
					ContentBase64: getString(fileMap, "content_base64"),
					ContentType:   getString(fileMap, "content_type"),
					Operation:     "save",
				}
				if task.ContentType == "" {
					task.ContentType = "application/octet-stream"
				}
				taskChan <- task
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
	results := make([]COSFileResult, 0, len(files))
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

// processCOSSaveTask processes a single COS save task
func processCOSSaveTask(ctx context.Context, task COSFileTask, storage *cos.Storage) COSFileResult {
	if task.Key == "" {
		return COSFileResult{Index: task.Index, Success: false, Error: "key is required"}
	}

	if task.ContentBase64 == "" {
		return COSFileResult{Index: task.Index, Key: task.Key, Success: false, Error: "content_base64 is required"}
	}

	content, err := base64.StdEncoding.DecodeString(task.ContentBase64)
	if err != nil {
		return COSFileResult{Index: task.Index, Key: task.Key, Success: false, Error: fmt.Sprintf("decode base64: %v", err)}
	}

	result, err := storage.SaveFile(ctx, task.Key, content, task.ContentType)
	if err != nil {
		return COSFileResult{Index: task.Index, Key: task.Key, Success: false, Error: err.Error()}
	}

	return COSFileResult{
		Index:     task.Index,
		Key:       task.Key,
		Success:   true,
		FileID:    result.FileID,
		Size:      result.Size,
		URL:       result.URL,
		AccessURL: result.AccessURL,
	}
}

// NewCOSDeleteFileSkill creates a skill to delete files from COS with batch support.
// Supports concurrent batch deletions for better performance.
func NewCOSDeleteFileSkill(storage *cos.Storage) Skill {
	return Skill{
		Name:    "cos.delete_file",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Check for batch delete
			if keys, ok := input["keys"].([]any); ok && len(keys) > 0 {
				return cosDeleteFilesBatch(ctx, keys, storage)
			}

			// Single file delete (backward compatibility)
			return cosDeleteSingleFile(ctx, input, storage)
		},
	}
}

// cosDeleteSingleFile handles single file deletion
func cosDeleteSingleFile(ctx context.Context, input map[string]any, storage *cos.Storage) (map[string]any, error) {
	key, ok := input["key"].(string)
	if !ok || key == "" {
		return nil, &InvokeError{Code: "INVALID_INPUT", Message: "key is required", Retryable: false}
	}

	err := storage.DeleteFile(ctx, key)
	if err != nil {
		return nil, &InvokeError{Code: "STORAGE_ERROR", Message: err.Error(), Retryable: true}
	}

	return map[string]any{
		"deleted": true,
		"key":     key,
	}, nil
}

// cosDeleteFilesBatch handles concurrent batch file deletions
func cosDeleteFilesBatch(ctx context.Context, keys []any, storage *cos.Storage) (map[string]any, error) {
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
				err := storage.DeleteFile(ctx, t.key)
				if err != nil {
					resultChan <- map[string]any{
						"key":     t.key,
						"success": false,
						"error":   err.Error(),
					}
					atomic.AddInt64(&failCount, 1)
				} else {
					resultChan <- map[string]any{
						"key":     t.key,
						"success": true,
						"deleted": true,
					}
					atomic.AddInt64(&successCount, 1)
				}
			}
		}()
	}

	// Send tasks
	go func() {
		for i, k := range keys {
			if key, ok := k.(string); ok && key != "" {
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
		"results":       results,
	}, nil
}

// NewCOSListFilesSkill creates a skill to list files in COS.
func NewCOSListFilesSkill(storage *cos.Storage) Skill {
	return Skill{
		Name:    "cos.list_files",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			prefix, _ := input["prefix"].(string)

			maxKeys := 100
			if v, ok := input["max_keys"].(float64); ok {
				maxKeys = int(v)
			}

			files, err := storage.ListFiles(ctx, prefix, maxKeys)
			if err != nil {
				return nil, &InvokeError{Code: "STORAGE_ERROR", Message: err.Error(), Retryable: true}
			}

			return map[string]any{
				"files":  files,
				"prefix": prefix,
				"count":  len(files),
			}, nil
		},
	}
}

// NewCOSGetPresignedURLSkill creates a skill to get presigned URL with batch support.
func NewCOSGetPresignedURLSkill(storage *cos.Storage) Skill {
	return Skill{
		Name:    "cos.get_presigned_url",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Check for batch operation
			if keys, ok := input["keys"].([]any); ok && len(keys) > 0 {
				return cosGetPresignedURLsBatch(ctx, keys, input, storage)
			}

			// Single key (backward compatibility)
			key, ok := input["key"].(string)
			if !ok || key == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "key is required", Retryable: false}
			}

			expiresMinutes := 30
			if v, ok := input["expires_minutes"].(float64); ok {
				expiresMinutes = int(v)
			}

			url, err := storage.GetPresignedURL(ctx, key, time.Duration(expiresMinutes)*time.Minute)
			if err != nil {
				return nil, &InvokeError{Code: "STORAGE_ERROR", Message: err.Error(), Retryable: true}
			}

			return map[string]any{
				"url":     url,
				"key":     key,
				"expires": time.Now().Add(time.Duration(expiresMinutes) * time.Minute).Format(time.RFC3339),
			}, nil
		},
	}
}

// cosGetPresignedURLsBatch handles concurrent batch URL generation
func cosGetPresignedURLsBatch(ctx context.Context, keys []any, input map[string]any, storage *cos.Storage) (map[string]any, error) {
	expiresMinutes := 30
	if v, ok := input["expires_minutes"].(float64); ok {
		expiresMinutes = int(v)
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
				url, err := storage.GetPresignedURL(ctx, t.key, time.Duration(expiresMinutes)*time.Minute)
				if err != nil {
					resultChan <- map[string]any{
						"key":     t.key,
						"success": false,
						"error":   err.Error(),
					}
					atomic.AddInt64(&failCount, 1)
				} else {
					resultChan <- map[string]any{
						"key":     t.key,
						"success": true,
						"url":     url,
						"expires": time.Now().Add(time.Duration(expiresMinutes) * time.Minute).Format(time.RFC3339),
					}
					atomic.AddInt64(&successCount, 1)
				}
			}
		}()
	}

	// Send tasks
	go func() {
		for i, k := range keys {
			if key, ok := k.(string); ok && key != "" {
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
		"success":         atomic.LoadInt64(&successCount) == int64(len(keys)),
		"total":           len(keys),
		"success_count":   atomic.LoadInt64(&successCount),
		"fail_count":      atomic.LoadInt64(&failCount),
		"expires_minutes": expiresMinutes,
		"results":         results,
	}, nil
}
