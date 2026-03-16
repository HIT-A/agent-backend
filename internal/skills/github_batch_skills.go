package skills

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hoa-agent-backend/internal/cos"
	"hoa-agent-backend/internal/mcp"
)

// GitHubBatchDownloadInput represents batch download input
type GitHubBatchDownloadInput struct {
	Repos         []string `json:"repos"`
	FileTypes     []string `json:"file_types"`
	MaxFileSize   int64    `json:"max_file_size"`    // 单个文件最大大小 (字节)
	MaxRepoSizeMB int      `json:"max_repo_size_mb"` // 仓库最大大小 (MB)，0表示不限制
	MaxFiles      int      `json:"max_files"`
	ConvertToMD   bool     `json:"convert_to_md"`
	PushToGitHub  bool     `json:"push_to_github"`
	TargetRepo    string   `json:"target_repo"`
	TargetBranch  string   `json:"target_branch"`
	StoreInCOS    bool     `json:"store_in_cos"`
	COSPrefix     string   `json:"cos_prefix"`
	AutoIngestRAG bool     `json:"auto_ingest_rag"`
	RAGSource     string   `json:"rag_source"`
	Workers       int      `json:"workers"`
}

// DownloadedFile represents a downloaded file
type DownloadedFile struct {
	Repo        string `json:"repo"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Content     string `json:"content_base64"`
	ContentType string `json:"content_type"`
	Converted   string `json:"converted_markdown,omitempty"`
	Error       string `json:"error,omitempty"`
}

// NewGitHubBatchDownloadSkill creates a skill for batch downloading from GitHub repos
func NewGitHubBatchDownloadSkill(mcpRegistry *mcp.Registry, cosStorage *cos.Storage) Skill {
	return Skill{
		Name:    "github.batch_download",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			in := parseBatchDownloadInput(input)

			if len(in.Repos) == 0 {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "repos list is required", Retryable: false}
			}

			// Execute batch download with concurrency
			result := executeBatchDownload(ctx, in, mcpRegistry, cosStorage)

			return result, nil
		},
	}
}

// NewDocumentConverterSkill creates a skill for converting documents to Markdown
func NewDocumentConverterSkill(mcpRegistry *mcp.Registry) Skill {
	return Skill{
		Name:    "document.convert",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			contentBase64, _ := input["content_base64"].(string)
			filename, _ := input["filename"].(string)

			if contentBase64 == "" || filename == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "content_base64 and filename are required", Retryable: false}
			}

			// Get unstructured MCP server
			server, exists := mcpRegistry.Get("unstructured")
			if !exists {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: "unstructured MCP server not registered", Retryable: false}
			}

			if !server.Initialized {
				return nil, &InvokeError{Code: "INTERNAL", Message: "unstructured MCP server not initialized", Retryable: true}
			}

			// Call MCP tool
			result, err := callMCPTool(ctx, server, "convert_to_markdown", map[string]any{
				"content_base64":    contentBase64,
				"filename":          filename,
				"chunking_strategy": input["chunking_strategy"],
				"max_characters":    input["max_characters"],
			})
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("conversion failed: %v", err), Retryable: true}
			}

			return result, nil
		},
	}
}

// NewRAGIngestFromGitHubSkill creates a complete pipeline from GitHub to RAG
func NewRAGIngestFromGitHubSkill(mcpRegistry *mcp.Registry, cosStorage *cos.Storage) Skill {
	return Skill{
		Name:    "rag.ingest_from_github",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			in := parseBatchDownloadInput(input)

			if len(in.Repos) == 0 {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "repos list is required", Retryable: false}
			}

			// Pipeline: Download → Convert → Store COS → Push GitHub → Ingest RAG
			result := executeFullPipeline(ctx, in, mcpRegistry, cosStorage)

			return result, nil
		},
	}
}

func parseBatchDownloadInput(input map[string]any) GitHubBatchDownloadInput {
	in := GitHubBatchDownloadInput{
		MaxFileSize:   10 * 1024 * 1024, // 10MB 单文件上限
		MaxRepoSizeMB: 500,              // 500MB 仓库上限，超过跳过
		MaxFiles:      1000,
		ConvertToMD:   true,
		PushToGitHub:  true,
		TargetRepo:    "HIT-A/HITA_RagData",
		TargetBranch:  "main",
		StoreInCOS:    true,
		COSPrefix:     "rag-content",
		AutoIngestRAG: true,
		Workers:       4,
		FileTypes:     []string{".pdf", ".docx", ".doc", ".pptx", ".ppt", ".txt", ".md", ".rtf"},
	}

	if repos, ok := input["repos"].([]any); ok {
		for _, r := range repos {
			if repo, ok := r.(string); ok {
				in.Repos = append(in.Repos, repo)
			}
		}
	}

	if fileTypes, ok := input["file_types"].([]any); ok && len(fileTypes) > 0 {
		in.FileTypes = nil
		for _, ft := range fileTypes {
			if s, ok := ft.(string); ok {
				in.FileTypes = append(in.FileTypes, s)
			}
		}
	}

	if maxFiles, ok := input["max_files"].(float64); ok {
		in.MaxFiles = int(maxFiles)
	}
	if maxFileSize, ok := input["max_file_size"].(float64); ok {
		in.MaxFileSize = int64(maxFileSize)
	}
	if maxRepoSize, ok := input["max_repo_size_mb"].(float64); ok {
		in.MaxRepoSizeMB = int(maxRepoSize)
	}
	if convertToMD, ok := input["convert_to_md"].(bool); ok {
		in.ConvertToMD = convertToMD
	}
	if pushToGitHub, ok := input["push_to_github"].(bool); ok {
		in.PushToGitHub = pushToGitHub
	}
	if targetRepo, ok := input["target_repo"].(string); ok {
		in.TargetRepo = targetRepo
	}
	if storeInCOS, ok := input["store_in_cos"].(bool); ok {
		in.StoreInCOS = storeInCOS
	}
	if cosPrefix, ok := input["cos_prefix"].(string); ok {
		in.COSPrefix = cosPrefix
	}
	if autoIngest, ok := input["auto_ingest_rag"].(bool); ok {
		in.AutoIngestRAG = autoIngest
	}
	if ragSource, ok := input["rag_source"].(string); ok {
		in.RAGSource = ragSource
	}
	if workers, ok := input["workers"].(float64); ok {
		in.Workers = int(workers)
	}

	return in
}

func executeBatchDownload(ctx context.Context, in GitHubBatchDownloadInput, mcpRegistry *mcp.Registry, cosStorage *cos.Storage) map[string]any {
	start := time.Now()

	type repoTask struct {
		repo string
		idx  int
	}

	taskChan := make(chan repoTask, len(in.Repos))
	resultChan := make(chan []DownloadedFile, len(in.Repos))

	var totalFiles int64
	var totalSize int64
	var successCount int64
	var failCount int64

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < in.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				files := downloadFromRepo(ctx, task.repo, in, mcpRegistry)
				resultChan <- files

				for _, f := range files {
					if f.Error == "" {
						atomic.AddInt64(&successCount, 1)
						atomic.AddInt64(&totalSize, f.Size)
					} else {
						atomic.AddInt64(&failCount, 1)
					}
					atomic.AddInt64(&totalFiles, 1)
				}
			}
		}()
	}

	// Send tasks
	go func() {
		for i, repo := range in.Repos {
			taskChan <- repoTask{repo: repo, idx: i}
		}
		close(taskChan)
	}()

	// Close result channel
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var allFiles []DownloadedFile
	for files := range resultChan {
		allFiles = append(allFiles, files...)
	}

	return map[string]any{
		"total_repos":   len(in.Repos),
		"total_files":   atomic.LoadInt64(&totalFiles),
		"success_count": atomic.LoadInt64(&successCount),
		"fail_count":    atomic.LoadInt64(&failCount),
		"total_size_mb": float64(atomic.LoadInt64(&totalSize)) / (1024 * 1024),
		"duration_ms":   time.Since(start).Milliseconds(),
		"files":         allFiles,
	}
}

func downloadFromRepo(ctx context.Context, repo string, in GitHubBatchDownloadInput, mcpRegistry *mcp.Registry) []DownloadedFile {
	// Parse repo (owner/repo format)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return []DownloadedFile{{Repo: repo, Error: "invalid repo format, expected owner/repo"}}
	}
	owner, repoName := parts[0], parts[1]

	// Use GitHub fetcher
	fetcher, err := NewGitHubFetcherFromEnv()
	if err != nil {
		return []DownloadedFile{{Repo: repo, Error: err.Error()}}
	}

	// List files
	files, err := fetcher.ListFiles(ctx, owner+"/"+repoName, "main", "")
	if err != nil {
		return []DownloadedFile{{Repo: repo, Error: fmt.Sprintf("list files: %v", err)}}
	}

	// Filter by file types
	var filteredFiles []RepoFile
	for _, f := range files {
		for _, ext := range in.FileTypes {
			if strings.HasSuffix(strings.ToLower(f.Path), ext) {
				filteredFiles = append(filteredFiles, f)
				break
			}
		}
		if len(filteredFiles) >= in.MaxFiles {
			break
		}
	}

	// Download files
	var results []DownloadedFile
	for _, f := range filteredFiles {
		content, err := fetcher.GetFile(ctx, owner+"/"+repoName, "main", f.Path)
		if err != nil {
			results = append(results, DownloadedFile{
				Repo:  repo,
				Path:  f.Path,
				Error: err.Error(),
			})
			continue
		}

		// Check file size
		if int64(len(content.Content)) > in.MaxFileSize {
			results = append(results, DownloadedFile{
				Repo:  repo,
				Path:  f.Path,
				Error: fmt.Sprintf("file too large: %d bytes", len(content.Content)),
			})
			continue
		}

		// Convert to base64
		contentBase64 := base64.StdEncoding.EncodeToString(content.Content)

		// Convert to Markdown if requested
		var convertedMD string
		if in.ConvertToMD {
			server, exists := mcpRegistry.Get("unstructured")
			if exists && server.Initialized {
				result, err := callMCPTool(ctx, server, "convert_to_markdown", map[string]any{
					"content_base64": contentBase64,
					"filename":       f.Path,
				})
				if err == nil {
					if md, ok := result["markdown"].(string); ok {
						convertedMD = md
					}
				}
			}
		}

		results = append(results, DownloadedFile{
			Repo:        repo,
			Path:        f.Path,
			Size:        int64(len(content.Content)),
			Content:     contentBase64,
			ContentType: "application/octet-stream",
			Converted:   convertedMD,
		})
	}

	return results
}

func executeFullPipeline(ctx context.Context, in GitHubBatchDownloadInput, mcpRegistry *mcp.Registry, cosStorage *cos.Storage) map[string]any {
	// Step 1: Download from GitHub
	downloadResult := executeBatchDownload(ctx, in, mcpRegistry, cosStorage)

	files, _ := downloadResult["files"].([]DownloadedFile)

	// Step 2: Store in COS
	var cosResults []map[string]any
	for _, f := range files {
		if f.Error != "" || f.Converted == "" {
			continue
		}

		if in.StoreInCOS && cosStorage != nil {
			cosKey := fmt.Sprintf("%s/%s/%s.md", in.COSPrefix, strings.ReplaceAll(f.Repo, "/", "-"), strings.ReplaceAll(f.Path, "/", "-"))
			_, err := cosStorage.SaveFile(ctx, cosKey, []byte(f.Converted), "text/markdown")
			if err == nil {
				cosResults = append(cosResults, map[string]any{
					"repo":    f.Repo,
					"path":    f.Path,
					"cos_key": cosKey,
					"size":    len(f.Converted),
				})
			}
		}
	}

	// Step 3: Push to GitHub (simplified - would need GitHub API client)
	var githubResults map[string]any
	if in.PushToGitHub {
		githubResults = map[string]any{
			"target_repo":  in.TargetRepo,
			"branch":       in.TargetBranch,
			"files_pushed": len(cosResults),
			"note":         "GitHub push requires GitHub API client integration",
		}
	}

	// Step 4: Ingest to RAG
	var ragResults map[string]any
	if in.AutoIngestRAG {
		ragResults = map[string]any{
			"source":       in.RAGSource,
			"files_queued": len(cosResults),
			"note":         "RAG ingest would be triggered here",
		}
	}

	return map[string]any{
		"download":        downloadResult,
		"cos_upload":      cosResults,
		"github_push":     githubResults,
		"rag_ingest":      ragResults,
		"pipeline_status": "completed",
	}
}
