package skills

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hoa-agent-backend/internal/cos"
	"hoa-agent-backend/internal/mcp"
)

// RAGSyncConfig 配置
type RAGSyncConfig struct {
	// 来源配置
	Sources []RAGSource `json:"sources"`

	// 目标仓库
	TargetRepo   string `json:"target_repo"`   // HIT-A/HITA_RagData
	TargetBranch string `json:"target_branch"` // main
	LocalPath    string `json:"local_path"`    // 本地克隆路径

	// COS 配置
	StoreInCOS bool   `json:"store_in_cos"`
	COSPrefix  string `json:"cos_prefix"`

	// RAG 配置
	AutoIngestRAG bool   `json:"auto_ingest_rag"`
	RAGCollection string `json:"rag_collection"`

	// 并发配置
	Workers int `json:"workers"`
}

// RAGSource 数据来源
type RAGSource struct {
	Type       string   `json:"type"` // github, crawl, manual
	Repo       string   `json:"repo"` // owner/repo for github
	URL        string   `json:"url"`  // for crawl
	PathPrefix string   `json:"path_prefix"`
	FileTypes  []string `json:"file_types"`
	MaxFiles   int      `json:"max_files"`
	MaxSizeKB  int      `json:"max_size_kb"` // 单文件最大大小限制，默认512KB (0.5MB)
}

// SourceMetadata 元数据
type SourceMetadata struct {
	Source struct {
		Type string `json:"type"`
		Repo string `json:"repo,omitempty"`
		Ref  string `json:"ref,omitempty"`
		URL  string `json:"url,omitempty"`
	} `json:"source"`
	Sync struct {
		Timestamp      time.Time `json:"timestamp"`
		Version        string    `json:"version"`
		FilesTotal     int       `json:"files_total"`
		FilesProcessed int       `json:"files_processed"`
		ChunksTotal    int       `json:"chunks_total"`
	} `json:"sync"`
	Files []FileInfo `json:"files"`
}

// FileInfo 文件信息
type FileInfo struct {
	Original  string `json:"original"`
	Converted string `json:"converted"`
	Size      int64  `json:"size"`
	Chunks    int    `json:"chunks"`
	COSKey    string `json:"cos_key,omitempty"`
}

// fileResult 文件处理结果（内部使用）
type fileResult struct {
	info   FileInfo
	chunks int
	err    error
}

// NewRAGSyncToRepoSkill 创建完整编排技能
func NewRAGSyncToRepoSkill(cosStorage *cos.Storage, mcpRegistry *mcp.Registry) Skill {
	return Skill{
		Name:    "rag.sync_to_repo",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			config := parseRAGSyncConfig(input)

			// Step 1: 准备本地仓库
			repoPath, err := prepareLocalRepo(ctx, config)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("prepare repo: %v", err), Retryable: false}
			}

			// Step 2: 处理所有来源
			var allMetadata []SourceMetadata
			var totalFiles, totalChunks int

			var qdrant *QdrantClient
			var embedder EmbeddingProvider
			if config.AutoIngestRAG {
				qdrant, err = NewQdrantClientFromEnv()
				if err == nil {
					embedder, err = NewEmbeddingProviderFromEnv()
					if err != nil {
						return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("embedder init: %v", err), Retryable: true}
					}
				} else {
					return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("qdrant init: %v", err), Retryable: true}
				}
			}

			for _, source := range config.Sources {
				metadata, files, chunks, err := processSource(ctx, source, repoPath, config, cosStorage, mcpRegistry, qdrant, embedder)
				if err != nil {
					return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("process source %s: %v", source.Repo, err), Retryable: true}
				}
				allMetadata = append(allMetadata, metadata)
				totalFiles += files
				totalChunks += chunks
			}

			// Step 3: 更新索引
			if err := updateIndex(repoPath, allMetadata); err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("update index: %v", err), Retryable: false}
			}

			// Step 4: Git commit & push
			commitHash, err := gitCommitAndPush(ctx, repoPath, config)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("git push: %v", err), Retryable: true}
			}

			return map[string]any{
				"status":        "completed",
				"repo_path":     repoPath,
				"sources_count": len(config.Sources),
				"total_files":   totalFiles,
				"total_chunks":  totalChunks,
				"commit_hash":   commitHash,
				"cos_uploaded":  config.StoreInCOS,
			}, nil
		},
	}
}

func parseRAGSyncConfig(input map[string]any) RAGSyncConfig {
	config := RAGSyncConfig{
		TargetRepo:    "HIT-A/HITA_RagData",
		TargetBranch:  "main",
		LocalPath:     "/tmp/HITA_RagData",
		StoreInCOS:    true,
		COSPrefix:     "rag-content",
		AutoIngestRAG: true,
		RAGCollection: "hit-courses",
		Workers:       4,
	}

	if repo, ok := input["target_repo"].(string); ok {
		config.TargetRepo = repo
	} else if repo, ok := input["repo"].(string); ok {
		config.TargetRepo = repo
	}
	if branch, ok := input["target_branch"].(string); ok {
		config.TargetBranch = branch
	} else if branch, ok := input["branch"].(string); ok {
		config.TargetBranch = branch
	}
	if path, ok := input["local_path"].(string); ok {
		config.LocalPath = path
	}
	if storeCOS, ok := input["store_in_cos"].(bool); ok {
		config.StoreInCOS = storeCOS
	}
	if cosPrefix, ok := input["cos_prefix"].(string); ok {
		config.COSPrefix = cosPrefix
	}
	if workers, ok := input["workers"].(float64); ok {
		config.Workers = int(workers)
	}

	// Parse sources
	if sources, ok := input["sources"].([]any); ok {
		for _, s := range sources {
			if sm, ok := s.(map[string]any); ok {
				source := RAGSource{
					Type:       getString(sm, "type"),
					Repo:       getString(sm, "repo"),
					URL:        getString(sm, "url"),
					PathPrefix: getString(sm, "path_prefix"),
					MaxFiles:   100,
				}
				if maxFiles, ok := sm["max_files"].(float64); ok {
					source.MaxFiles = int(maxFiles)
				}
				if maxSizeKB, ok := sm["max_size_kb"].(float64); ok {
					source.MaxSizeKB = int(maxSizeKB)
				}
				if fileTypes, ok := sm["file_types"].([]any); ok {
					for _, ft := range fileTypes {
						if fts, ok := ft.(string); ok {
							source.FileTypes = append(source.FileTypes, fts)
						}
					}
				}
				config.Sources = append(config.Sources, source)
			}
		}
	}

	// 如果没有指定来源，使用默认的优质仓库列表
	if len(config.Sources) == 0 {
		config.Sources = getDefaultSources()
	}

	return config
}

func getDefaultSources() []RAGSource {
	return []RAGSource{
		{Type: "github", Repo: "HIT-A/HITA_RagData", PathPrefix: "新生手册", FileTypes: []string{".txt", ".md"}, MaxFiles: 50},
	}
}

func prepareLocalRepo(ctx context.Context, config RAGSyncConfig) (string, error) {
	repoPath := config.LocalPath

	// 检查是否已经克隆
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); os.IsNotExist(err) {
		// 克隆仓库
		cloneURL := buildGitHubCloneURL(config.TargetRepo)
		cmd := exec.CommandContext(ctx, "git", "clone", "-b", config.TargetBranch, cloneURL, repoPath)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git clone failed: %w\n%s", err, output)
		}
	} else {
		if err := configureGitRemoteForAuth(ctx, repoPath, config.TargetRepo); err != nil {
			return "", fmt.Errorf("configure git remote auth failed: %w", err)
		}
		// 拉取最新代码
		cmd := exec.CommandContext(ctx, "git", "pull", "origin", config.TargetBranch)
		cmd.Dir = repoPath
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git pull failed: %w\n%s", err, output)
		}
	}

	// 创建必要的目录结构
	dirs := []string{
		"sources/github",
		"sources/crawled",
		"sources/manual",
		"index",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(repoPath, dir), 0755); err != nil {
			return "", err
		}
	}

	return repoPath, nil
}

func processSource(ctx context.Context, source RAGSource, repoPath string, config RAGSyncConfig, cosStorage *cos.Storage, mcpRegistry *mcp.Registry, qdrant *QdrantClient, embedder EmbeddingProvider) (SourceMetadata, int, int, error) {
	metadata := SourceMetadata{}
	metadata.Source.Type = source.Type
	metadata.Source.Repo = source.Repo
	metadata.Source.Ref = "main"
	metadata.Sync.Timestamp = time.Now()

	var totalFiles, totalChunks int

	switch source.Type {
	case "github":
		files, chunks, err := processGitHubSource(ctx, source, repoPath, config, cosStorage, mcpRegistry, &metadata, qdrant, embedder)
		if err != nil {
			return metadata, 0, 0, err
		}
		totalFiles = files
		totalChunks = chunks
	case "crawl":
		totalFiles, totalChunks, _ = processCrawlSource(ctx, source, repoPath, config, cosStorage, mcpRegistry)
	}

	metadata.Sync.FilesTotal = totalFiles
	metadata.Sync.FilesProcessed = totalFiles
	metadata.Sync.ChunksTotal = totalChunks

	return metadata, totalFiles, totalChunks, nil
}

func processGitHubSource(ctx context.Context, source RAGSource, repoPath string, config RAGSyncConfig, cosStorage *cos.Storage, mcpRegistry *mcp.Registry, metadata *SourceMetadata, qdrant *QdrantClient, embedder EmbeddingProvider) (int, int, error) {
	fetcher, err := NewGitHubFetcherFromEnv()
	if err != nil {
		return 0, 0, err
	}

	// 列出文件
	files, err := fetcher.ListFiles(ctx, source.Repo, "main", source.PathPrefix)
	if err != nil {
		return 0, 0, err
	}

	// 过滤文件类型 + 大小限制
	var filteredFiles []RepoFile
	maxSizeKB := source.MaxSizeKB
	if maxSizeKB == 0 {
		maxSizeKB = 512 // 默认 0.5MB
	}
	maxSizeBytes := maxSizeKB * 1024

	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Path))
		allowed := false
		for _, t := range source.FileTypes {
			if ext == t {
				allowed = true
				break
			}
		}
		if !allowed {
			continue
		}
		// 按文件大小过滤
		if f.Size > maxSizeBytes {
			continue
		}
		filteredFiles = append(filteredFiles, f)
	}

	if len(filteredFiles) > source.MaxFiles {
		filteredFiles = filteredFiles[:source.MaxFiles]
	}

	// 创建来源目录
	sourceDirName := strings.ReplaceAll(source.Repo, "/", "-")
	sourceDir := filepath.Join(repoPath, "sources/github", sourceDirName)
	rawDir := filepath.Join(sourceDir, "raw")
	convertedDir := filepath.Join(sourceDir, "converted")

	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return 0, 0, err
	}
	if err := os.MkdirAll(convertedDir, 0755); err != nil {
		return 0, 0, err
	}

	var totalChunks int
	var processedFiles int

	resultChan := make(chan fileResult, len(filteredFiles))
	var wg sync.WaitGroup

	for _, file := range filteredFiles {
		wg.Add(1)
		go func(f RepoFile) {
			defer wg.Done()
			result := processGitHubFile(ctx, fetcher, source, f, rawDir, convertedDir, cosStorage, config, mcpRegistry, qdrant, embedder)
			resultChan <- result
		}(file)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		if result.err == nil {
			metadata.Files = append(metadata.Files, result.info)
			totalChunks += result.chunks
			processedFiles++
		}
	}

	return processedFiles, totalChunks, nil
}

func processGitHubFile(ctx context.Context, fetcher *GitHubFetcher, source RAGSource, file RepoFile, rawDir, convertedDir string, cosStorage *cos.Storage, config RAGSyncConfig, mcpRegistry *mcp.Registry, qdrant *QdrantClient, embedder EmbeddingProvider) fileResult {
	result := fileResult{}

	// 获取文件内容
	content, err := fetcher.GetFile(ctx, source.Repo, "main", file.Path)
	if err != nil {
		result.err = err
		return result
	}

	relPath := file.Path

	// SHA256 dedup check
	contentHash := sha256.Sum256(content.Content)
	sha256Hex := hex.EncodeToString(contentHash[:])

	dedupStore, _ := NewDedupStoreFromEnv()
	if dedupStore != nil {
		shouldIngest, existing, _ := dedupStore.ShouldIngest(ctx, sha256Hex)
		if !shouldIngest {
			result.info = FileInfo{
				Original:  relPath,
				Converted: existing.COSKey,
				Size:      int64(len(content.Content)),
				Chunks:    0,
			}
			result.err = fmt.Errorf("duplicate SHA256: %s (COS: %s)", sha256Hex[:12], existing.COSKey)
			dedupStore.Close()
			return result
		}
	}

	// 保存原始文件到 GitHub audit trail
	rawPath := filepath.Join(rawDir, relPath)
	if err := os.MkdirAll(filepath.Dir(rawPath), 0755); err != nil {
		result.err = err
		return result
	}
	if err := os.WriteFile(rawPath, content.Content, 0644); err != nil {
		result.err = err
		return result
	}

	// 存储到 COS
	var cosKey string
	if config.StoreInCOS && cosStorage != nil {
		cosKey = fmt.Sprintf("%s/%s/%s/%s", config.COSPrefix, strings.ReplaceAll(source.Repo, "/", "-"), sha256Hex[:12], relPath)
		if _, err := cosStorage.SaveFile(ctx, cosKey, content.Content, "application/octet-stream"); err != nil {
			result.err = fmt.Errorf("COS save: %w", err)
			return result
		}
	}

	// 记录到 dedup store
	if dedupStore != nil && cosKey != "" {
		dedupStore.Record(ctx, sha256Hex, cosKey, int64(file.Size))
		dedupStore.Close()
	}

	// 转换为 Markdown
	ext := strings.ToLower(filepath.Ext(file.Path))
	var markdownContent []byte
	if ext == ".md" || ext == ".txt" {
		markdownContent = content.Content
	} else {
		markdownContent = convertToMarkdownWithUnstructured(ctx, mcpRegistry, content.Content, file.Path)
	}

	// 添加来源信息头部
	header := fmt.Sprintf("---\nsource: %s\noriginal_path: %s\nsha256: %s\ndownloaded: %s\n---\n\n",
		source.Repo, file.Path, sha256Hex, time.Now().Format(time.RFC3339))
	markdownContent = append([]byte(header), markdownContent...)

	// 保存转换后文件到 GitHub audit trail
	safeRelPath := strings.ReplaceAll(relPath, "/", "_")
	convertedBase := strings.TrimSuffix(safeRelPath, filepath.Ext(safeRelPath))
	convertedPath := filepath.Join(convertedDir, convertedBase+".md")
	if err := os.WriteFile(convertedPath, markdownContent, 0644); err != nil {
		result.err = err
		return result
	}

	// 直接 RAG ingest 到 Qdrant
	var chunks int
	if config.AutoIngestRAG && qdrant != nil && embedder != nil {
		sourceTag := fmt.Sprintf("sync/%s/%s", source.Repo, relPath)
		chunks, err = IngestMarkdownDirect(ctx, markdownContent, relPath, sourceTag, cosStorage, mcpRegistry, qdrant, embedder)
		if err != nil {
			result.err = fmt.Errorf("RAG ingest: %w", err)
			return result
		}
	}

	result.info = FileInfo{
		Original:  relPath,
		Converted: relPath + ".md",
		Size:      int64(len(markdownContent)),
		Chunks:    chunks,
	}
	result.chunks = chunks

	return result
}

func updateIndex(repoPath string, allMetadata []SourceMetadata) error {
	// 更新 documents.json
	documentsPath := filepath.Join(repoPath, "index", "documents.json")

	var documents []map[string]any
	for _, meta := range allMetadata {
		for _, file := range meta.Files {
			doc := map[string]any{
				"source":     meta.Source.Repo,
				"original":   file.Original,
				"converted":  file.Converted,
				"size":       file.Size,
				"chunks":     file.Chunks,
				"cos_key":    file.COSKey,
				"updated_at": meta.Sync.Timestamp.Format(time.RFC3339),
			}
			documents = append(documents, doc)
		}
	}

	data, err := json.MarshalIndent(documents, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(documentsPath, data, 0644)
}

func gitCommitAndPush(ctx context.Context, repoPath string, config RAGSyncConfig) (string, error) {
	if err := ensureGitIdentity(ctx, repoPath); err != nil {
		return "", fmt.Errorf("configure git identity failed: %w", err)
	}

	// git add
	cmd := exec.CommandContext(ctx, "git", "add", ".")
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add failed: %w\n%s", err, output)
	}

	// git commit
	commitMsg := fmt.Sprintf("chore: sync RAG data from sources (%s)", time.Now().Format("2006-01-02 15:04:05"))
	cmd = exec.CommandContext(ctx, "git", "commit", "-m", commitMsg)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		// 可能没有变更
		if strings.Contains(string(output), "nothing to commit") {
			return "", nil
		}
		return "", fmt.Errorf("git commit failed: %w\n%s", err, output)
	}

	// git push
	if err := configureGitRemoteForAuth(ctx, repoPath, config.TargetRepo); err != nil {
		return "", fmt.Errorf("configure git remote auth failed: %w", err)
	}
	cmd = exec.CommandContext(ctx, "git", "push", "origin", config.TargetBranch)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git push failed: %w\n%s", err, output)
	}

	// 获取 commit hash
	cmd = exec.CommandContext(ctx, "git", "rev-parse", "HEAD")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}

func ensureGitIdentity(ctx context.Context, repoPath string) error {
	name := strings.TrimSpace(os.Getenv("GIT_AUTHOR_NAME"))
	if name == "" {
		name = "agent-backend-bot"
	}
	email := strings.TrimSpace(os.Getenv("GIT_AUTHOR_EMAIL"))
	if email == "" {
		email = "agent-backend-bot@localhost"
	}

	setName := exec.CommandContext(ctx, "git", "config", "user.name", name)
	setName.Dir = repoPath
	if output, err := setName.CombinedOutput(); err != nil {
		return fmt.Errorf("git config user.name failed: %w\n%s", err, output)
	}

	setEmail := exec.CommandContext(ctx, "git", "config", "user.email", email)
	setEmail.Dir = repoPath
	if output, err := setEmail.CombinedOutput(); err != nil {
		return fmt.Errorf("git config user.email failed: %w\n%s", err, output)
	}

	return nil
}

func buildGitHubCloneURL(targetRepo string) string {
	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		return fmt.Sprintf("https://github.com/%s.git", targetRepo)
	}

	user := strings.TrimSpace(os.Getenv("GITHUB_USERNAME"))
	if user == "" {
		user = "x-access-token"
	}

	return fmt.Sprintf("https://%s:%s@github.com/%s.git", url.QueryEscape(user), url.QueryEscape(token), targetRepo)
}

func configureGitRemoteForAuth(ctx context.Context, repoPath, targetRepo string) error {
	cloneURL := buildGitHubCloneURL(targetRepo)
	cmd := exec.CommandContext(ctx, "git", "remote", "set-url", "origin", cloneURL)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git remote set-url failed: %w\n%s", err, output)
	}
	return nil
}

func convertToMarkdownWithUnstructured(ctx context.Context, mcpRegistry *mcp.Registry, content []byte, filename string) []byte {
	if mcpRegistry == nil {
		return content
	}

	server, exists := mcpRegistry.Get("unstructured")
	if !exists || !server.Initialized {
		return content
	}

	contentBase64 := base64.StdEncoding.EncodeToString(content)

	result, err := callMCPTool(ctx, server, "convert_to_markdown", map[string]any{
		"content_base64": contentBase64,
		"filename":       filename,
	})
	if err != nil {
		return content
	}

	if md, ok := result["markdown"].(string); ok && md != "" {
		return []byte(md)
	}

	return content
}

func processCrawlSource(ctx context.Context, source RAGSource, repoPath string, config RAGSyncConfig, cosStorage *cos.Storage, mcpRegistry *mcp.Registry) (int, int, error) {
	totalFiles := 0
	totalChunks := 0

	if source.URL == "" {
		return 0, 0, nil
	}

	if mcpRegistry == nil {
		return 0, 0, fmt.Errorf("MCP registry not available")
	}

	server, exists := mcpRegistry.Get("crawl4ai")
	if !exists {
		return 0, 0, fmt.Errorf("crawl4ai MCP server not registered")
	}
	if !server.Initialized {
		return 0, 0, fmt.Errorf("crawl4ai MCP server not initialized")
	}

	// 创建来源目录
	safeURL := strings.ReplaceAll(source.URL, "://", "-")
	safeURL = strings.ReplaceAll(safeURL, "/", "-")
	safeURL = strings.ReplaceAll(safeURL, ".", "-")
	safeURL = strings.ReplaceAll(safeURL, ":", "-")
	sourceDir := filepath.Join(repoPath, "sources", "crawled", safeURL)
	rawDir := sourceDir

	if err := os.MkdirAll(rawDir, 0755); err != nil {
		return 0, 0, fmt.Errorf("create directory failed: %w", err)
	}

	// 调用 crawl4ai 爬取页面
	args := map[string]any{
		"url":            source.URL,
		"content_filter": true,
		"output_format":  "markdown",
	}

	result, err := callMCPTool(ctx, server, "crawl_page", args)
	if err != nil {
		return 0, 0, fmt.Errorf("crawl4ai call failed: %w", err)
	}

	// 提取 markdown 内容
	var markdownContent string
	if content, ok := result["markdown"].(string); ok && content != "" {
		markdownContent = content
	} else if html, ok := result["html"].(string); ok && html != "" {
		markdownContent = html
	} else if raw, ok := result["raw_result"].(string); ok && raw != "" {
		markdownContent = raw
	} else {
		// 尝试从 JSON 中提取
		jsonBytes, _ := json.Marshal(result)
		markdownContent = string(jsonBytes)
	}

	if markdownContent == "" {
		return 0, 0, fmt.Errorf("no content extracted from crawl result")
	}

	// 保存原始内容
	filename := fmt.Sprintf("%s.md", safeURL[:min(len(safeURL), 50)])
	rawPath := filepath.Join(rawDir, filename)
	if err := os.WriteFile(rawPath, []byte(markdownContent), 0644); err != nil {
		return 0, 0, fmt.Errorf("write file failed: %w", err)
	}

	// 添加来源信息头部
	header := fmt.Sprintf("---\nsource: crawl\nurl: %s\ndownloaded: %s\n---\n\n",
		source.URL, time.Now().Format(time.RFC3339))
	markdownWithHeader := append([]byte(header), []byte(markdownContent)...)

	// 上传到 COS
	cosKey := ""
	if config.StoreInCOS && cosStorage != nil {
		cosKey = fmt.Sprintf("%s/crawled/%s/%s.md", config.COSPrefix, safeURL[:min(len(safeURL), 50)], safeURL[:min(len(safeURL), 50)])
		if _, err := cosStorage.SaveFile(ctx, cosKey, markdownWithHeader, "text/markdown"); err != nil {
			cosKey = ""
		}
	}

	// 计算块数
	chunks := len(markdownWithHeader) / 1400
	if chunks == 0 {
		chunks = 1
	}

	totalFiles = 1
	totalChunks = chunks

	return totalFiles, totalChunks, nil
}
