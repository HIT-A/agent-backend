package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hoa-agent-backend/internal/cos"
)

type RAGIngestInput struct {
	Repo       string `json:"repo"`
	Ref        string `json:"ref"`
	PathPrefix string `json:"path_prefix"`
	Source     string `json:"source"`
	Collection string `json:"collection"`
	DryRun     bool   `json:"dry_run"`
	MaxFiles   int    `json:"max_files"`
	MaxChunks  int    `json:"max_chunks"`
	Workers    int    `json:"workers"`      // Number of concurrent workers
	StoreInCOS bool   `json:"store_in_cos"` // Whether to store original files in COS
	COSPrefix  string `json:"cos_prefix"`   // COS prefix for storing files
}

// FileTask represents a file to be processed
type FileTask struct {
	File  RepoFile
	Index int
}

// FileResult represents the result of processing a file
type FileResult struct {
	File        RepoFile
	Success     bool
	Error       error
	ChunkCount  int
	PointsCount int
	Skipped     bool
}

func NewRAGIngestSkill(cosStorage *cos.Storage) Skill {
	return Skill{
		Name:    "rag.ingest",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			in, err := decodeRAGIngestInput(input)
			if err != nil {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: err.Error(), Retryable: false}
			}
			if in.Ref == "" {
				in.Ref = "main"
			}
			if in.Collection == "" {
				// Only require Qdrant if not dry run
				if !in.DryRun {
					c, err := NewQdrantClientFromEnv()
					if err != nil {
						return nil, err
					}
					in.Collection = c.Collection
				} else {
					in.Collection = "dry-run-collection"
				}
			}
			if in.MaxFiles <= 0 {
				in.MaxFiles = 100
			}
			if in.MaxChunks <= 0 {
				in.MaxChunks = 5000
			}
			if in.Workers <= 0 {
				in.Workers = 4 // Default to 4 concurrent workers
			}
			if in.COSPrefix == "" {
				in.COSPrefix = "rag-source"
			}

			fetcher, err := NewGitHubFetcherFromEnv()
			if err != nil {
				return nil, err
			}

			// Only require Qdrant and embedder if not dry run
			var qdrant *QdrantClient
			var embedder EmbeddingProvider

			if !in.DryRun {
				qdrant, err = NewQdrantClientFromEnv()
				if err != nil {
					return nil, err
				}

				embedder, err = NewEmbeddingProviderFromEnv()
				if err != nil {
					return nil, err
				}
			}

			files, err := fetcher.ListFiles(ctx, in.Repo, in.Ref, in.PathPrefix)
			if err != nil {
				return nil, err
			}
			if len(files) > in.MaxFiles {
				files = files[:in.MaxFiles]
			}

			// Use channels for concurrent processing
			fileChan := make(chan FileTask, len(files))
			resultChan := make(chan FileResult, len(files))

			// Atomic counters for progress tracking
			var processedFiles int64
			var processedChunks int64
			var upsertedPoints int64
			var skippedFiles int64
			var errorsCount int64

			// Start workers
			var wg sync.WaitGroup
			for i := 0; i < in.Workers; i++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					for task := range fileChan {
						result := processFile(ctx, task, in, fetcher, qdrant, embedder, cosStorage)
						resultChan <- result

						// Update counters atomically
						if result.Success {
							atomic.AddInt64(&processedFiles, 1)
							atomic.AddInt64(&processedChunks, int64(result.ChunkCount))
							atomic.AddInt64(&upsertedPoints, int64(result.PointsCount))
						} else if result.Skipped {
							atomic.AddInt64(&skippedFiles, 1)
						} else {
							atomic.AddInt64(&errorsCount, 1)
						}
					}
				}(i)
			}

			// Send tasks to workers
			go func() {
				for i, f := range files {
					fileChan <- FileTask{File: f, Index: i}
				}
				close(fileChan)
			}()

			// Close result channel when all workers are done
			go func() {
				wg.Wait()
				close(resultChan)
			}()

			// Collect results
			for range resultChan {
				// Results are processed by workers, just drain the channel
			}

			return map[string]any{
				"processed_files":  atomic.LoadInt64(&processedFiles),
				"processed_chunks": atomic.LoadInt64(&processedChunks),
				"upserted_points":  atomic.LoadInt64(&upsertedPoints),
				"skipped_files":    atomic.LoadInt64(&skippedFiles),
				"errors_count":     atomic.LoadInt64(&errorsCount),
				"total_files":      len(files),
				"workers":          in.Workers,
			}, nil
		},
	}
}

func processFile(ctx context.Context, task FileTask, in RAGIngestInput,
	fetcher *GitHubFetcher, qdrant *QdrantClient, embedder EmbeddingProvider,
	cosStorage *cos.Storage) FileResult {

	result := FileResult{File: task.File}

	// Fetch file content
	fc, err := fetcher.GetFile(ctx, in.Repo, in.Ref, task.File.Path)
	if err != nil {
		result.Error = fmt.Errorf("fetch file: %w", err)
		return result
	}

	// Store in COS if enabled (before processing)
	if in.StoreInCOS && cosStorage != nil {
		cosKey := fmt.Sprintf("%s/%s/%s/%s", in.COSPrefix, in.Repo, in.Ref, task.File.Path)
		_, err := cosStorage.SaveFile(ctx, cosKey, fc.Content, "application/octet-stream")
		if err != nil {
			// Log error but continue processing
			fmt.Printf("Warning: failed to store file %s in COS: %v\n", task.File.Path, err)
		}
	}

	// Parse document
	doc, err := ParseDocument(fc.Path, fc.Content)
	if err != nil {
		result.Error = fmt.Errorf("parse document: %w", err)
		return result
	}
	if strings.TrimSpace(doc.Text) == "" {
		result.Skipped = true
		return result
	}

	// Chunk text
	chunks := ChunkText(doc, 1400)
	if len(chunks) == 0 {
		result.Skipped = true
		return result
	}

	result.ChunkCount = len(chunks)

	if in.DryRun {
		result.Success = true
		return result
	}

	// Create points from chunks
	points := make([]QdrantPoint, 0, len(chunks))
	for _, ch := range chunks {
		if atomic.LoadInt64(&processedChunks) >= int64(in.MaxChunks) {
			break
		}

		vec, err := embedder.Embed(ctx, ch.Text)
		if err != nil {
			result.Error = fmt.Errorf("embed chunk: %w", err)
			return result
		}

		docID := fc.Path
		chunkID := ch.ChunkID
		id := stablePointID(docID, chunkID)

		snippet := strings.TrimSpace(ch.Text)
		if len(snippet) > 280 {
			snippet = snippet[:280]
		}

		payload := map[string]any{
			"doc_id":   docID,
			"chunk_id": chunkID,
			"title":    doc.Title,
			"snippet":  snippet,
			"source":   in.Source,
			"repo":     in.Repo,
			"ref":      in.Ref,
		}

		points = append(points, QdrantPoint{ID: id, Vector: vec, Payload: payload})
	}

	if len(points) == 0 {
		result.Skipped = true
		return result
	}

	// Upsert to Qdrant with retry
	if err := upsertWithRetry(ctx, qdrant, in.Collection, points, 3); err != nil {
		result.Error = fmt.Errorf("upsert points: %w", err)
		return result
	}

	result.PointsCount = len(points)
	result.Success = true
	return result
}

// upsertWithRetry attempts to upsert points with exponential backoff
func upsertWithRetry(ctx context.Context, qdrant *QdrantClient, collection string, points []QdrantPoint, maxRetries int) error {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			// Exponential backoff: 100ms, 200ms, 400ms
			time.Sleep(time.Duration(100*(1<<(i-1))) * time.Millisecond)
		}

		err := qdrant.Upsert(ctx, collection, points)
		if err == nil {
			return nil
		}
		lastErr = err

		// Don't retry on context cancellation
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return fmt.Errorf("upsert failed after %d retries: %w", maxRetries, lastErr)
}

func decodeRAGIngestInput(m map[string]any) (RAGIngestInput, error) {
	var in RAGIngestInput
	if m == nil {
		return in, errors.New("missing input")
	}

	getStr := func(k string) string {
		v, _ := m[k]
		s, _ := v.(string)
		return strings.TrimSpace(s)
	}
	getInt := func(k string) int {
		v, ok := m[k]
		if !ok {
			return 0
		}
		switch t := v.(type) {
		case float64:
			return int(t)
		case int:
			return t
		default:
			return 0
		}
	}
	getBool := func(k string) bool {
		v, ok := m[k]
		if !ok {
			return false
		}
		b, _ := v.(bool)
		return b
	}

	in.Repo = getStr("repo")
	in.Ref = getStr("ref")
	in.PathPrefix = getStr("path_prefix")
	in.Source = getStr("source")
	in.Collection = getStr("collection")
	in.DryRun = getBool("dry_run")
	in.MaxFiles = getInt("max_files")
	in.MaxChunks = getInt("max_chunks")
	in.Workers = getInt("workers")
	in.StoreInCOS = getBool("store_in_cos")
	in.COSPrefix = getStr("cos_prefix")

	if in.Repo == "" {
		return in, fmt.Errorf("repo is required")
	}
	return in, nil
}

func stablePointID(docID, chunkID string) string {
	h := sha256.Sum256([]byte(docID + ":" + chunkID))
	return hex.EncodeToString(h[:])
}

// processedChunks is used for checking max chunks limit across goroutines
var processedChunks int64

func init() {
	processedChunks = 0
}
