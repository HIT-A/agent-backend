package syncknowledge

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GitHubSync 同步 GitHub 知识库到 Qdrant
type GitHubSync struct {
	repo      string // 如: HIT-A/DATA_knowledge
	branch    string // 如: main
	token     string // GitHub token (可选,用于 private repo)
	localPath string // 本地缓存路径
	qdrant    QdrantClient
	embedder  EmbeddingProvider
	lastSync  *SyncState
}

// SyncState 保存同步状态
type SyncState struct {
	LastCommit string            `json:"last_commit"`
	FileHashes map[string]string `json:"file_hashes"` // file path -> sha256
	LastSyncAt time.Time         `json:"last_sync_at"`
}

// GitHubFile 表示 GitHub 仓库中的文件
type GitHubFile struct {
	Path    string `json:"path"`
	SHA     string `json:"sha"`
	Content string `json:"content"` // base64 encoded
	Size    int    `json:"size"`
}

// NewGitHubSync 创建同步器
func NewGitHubSync(repo, branch, localPath string, qdrant QdrantClient, embedder EmbeddingProvider) *GitHubSync {
	return &GitHubSync{
		repo:      repo,
		branch:    branch,
		localPath: localPath,
		qdrant:    qdrant,
		embedder:  embedder,
		lastSync:  &SyncState{FileHashes: make(map[string]string)},
	}
}

// LoadState 从本地加载同步状态
func (s *GitHubSync) LoadState() error {
	stateFile := filepath.Join(s.localPath, ".sync_state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 首次同步
		}
		return err
	}
	return json.Unmarshal(data, s.lastSync)
}

// SaveState 保存同步状态
func (s *GitHubSync) SaveState() error {
	stateFile := filepath.Join(s.localPath, ".sync_state.json")
	data, err := json.MarshalIndent(s.lastSync, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0644)
}

// GetLatestCommit 获取最新 commit hash
func (s *GitHubSync) GetLatestCommit(ctx context.Context) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/commits/%s", s.repo, s.branch)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GitHub API error: %d, %s", resp.StatusCode, string(body))
	}

	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.SHA, nil
}

// GetTree 获取仓库文件树
func (s *GitHubSync) GetTree(ctx context.Context, commit string) ([]GitHubFile, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/git/trees/%s?recursive=1", s.repo, commit)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Tree []struct {
			Path string `json:"path"`
			SHA  string `json:"sha"`
			Type string `json:"type"`
			Size int    `json:"size"`
		} `json:"tree"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	files := make([]GitHubFile, 0)
	for _, item := range result.Tree {
		if item.Type == "blob" && isSupportedFile(item.Path) {
			files = append(files, GitHubFile{
				Path: item.Path,
				SHA:  item.SHA,
				Size: item.Size,
			})
		}
	}

	return files, nil
}

// DownloadFile 下载文件内容
func (s *GitHubSync) DownloadFile(ctx context.Context, path string) ([]byte, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s", s.repo, path, s.branch)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("download failed: %d, %s", resp.StatusCode, string(body))
	}

	var result struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// GitHub 返回 base64 编码的内容
	return base64.StdEncoding.DecodeString(result.Content)
}

// Sync 执行增量同步
func (s *GitHubSync) Sync(ctx context.Context) (*SyncResult, error) {
	result := &SyncResult{}

	// 1. 获取最新 commit
	latestCommit, err := s.GetLatestCommit(ctx)
	if err != nil {
		return nil, fmt.Errorf("get latest commit: %w", err)
	}
	result.LatestCommit = latestCommit

	// 2. 检查是否有变化
	if latestCommit == s.lastSync.LastCommit {
		result.NoChange = true
		return result, nil
	}

	// 3. 获取当前文件树
	files, err := s.GetTree(ctx, latestCommit)
	if err != nil {
		return nil, fmt.Errorf("get tree: %w", err)
	}

	// 4. 对比差异
	newFiles, modifiedFiles, deletedFiles := s.diffFiles(files)

	// 5. 处理新增和修改
	for _, file := range append(newFiles, modifiedFiles...) {
		if err := s.processFile(ctx, file); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", file.Path, err))
		} else {
			result.Upserted = append(result.Upserted, file.Path)
		}
	}

	// 6. 处理删除
	for _, path := range deletedFiles {
		if err := s.deleteFile(ctx, path); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("delete %s: %v", path, err))
		} else {
			result.Deleted = append(result.Deleted, path)
		}
	}

	// 7. 更新状态
	s.lastSync.LastCommit = latestCommit
	s.lastSync.LastSyncAt = time.Now()
	if err := s.SaveState(); err != nil {
		return result, fmt.Errorf("save state: %w", err)
	}

	return result, nil
}

// processFile 处理单个文件: 下载 → 解析 → 分块 → 向量化 → 入库
func (s *GitHubSync) processFile(ctx context.Context, file GitHubFile) error {
	// 下载内容
	content, err := s.DownloadFile(ctx, file.Path)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}

	// 计算 hash
	hash := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hash[:])

	// 解析为 markdown
	text := string(content)

	// 分块
	chunks := chunkDocument(text, file.Path)

	// 生成向量并入库
	for i, chunk := range chunks {
		vector, err := s.embedder.Embed(ctx, chunk.Text)
		if err != nil {
			return fmt.Errorf("embed chunk %d: %w", i, err)
		}

		point := &Point{
			ID:     fmt.Sprintf("%s:%d", file.Path, i),
			Vector: vector,
			Payload: map[string]interface{}{
				"file":      file.Path,
				"chunk":     i,
				"text":      chunk.Text,
				"hash":      hashStr,
				"timestamp": time.Now().Unix(),
			},
		}

		if err := s.qdrant.Upsert(ctx, point); err != nil {
			return fmt.Errorf("upsert chunk %d: %w", i, err)
		}
	}

	// 更新 hash 记录
	s.lastSync.FileHashes[file.Path] = hashStr

	return nil
}

// deleteFile 从 Qdrant 删除文件的所有 chunks
func (s *GitHubSync) deleteFile(ctx context.Context, path string) error {
	// 删除该文件的所有 chunk (通过前缀匹配)
	filter := map[string]interface{}{
		"key": "file",
		"match": map[string]interface{}{
			"value": path,
		},
	}

	if err := s.qdrant.DeleteByFilter(ctx, filter); err != nil {
		return err
	}

	delete(s.lastSync.FileHashes, path)
	return nil
}

// diffFiles 对比文件差异
func (s *GitHubSync) diffFiles(currentFiles []GitHubFile) (newFiles, modifiedFiles []GitHubFile, deletedFiles []string) {
	currentMap := make(map[string]GitHubFile)
	for _, f := range currentFiles {
		currentMap[f.Path] = f
	}

	// 检查新增和修改
	for _, file := range currentFiles {
		oldHash, exists := s.lastSync.FileHashes[file.Path]
		if !exists {
			newFiles = append(newFiles, file)
		} else if oldHash != file.SHA {
			// SHA 变了，需要重新下载计算
			modifiedFiles = append(modifiedFiles, file)
		}
	}

	// 检查删除
	for path := range s.lastSync.FileHashes {
		if _, exists := currentMap[path]; !exists {
			deletedFiles = append(deletedFiles, path)
		}
	}

	return
}

// SyncResult 同步结果
type SyncResult struct {
	LatestCommit string   `json:"latest_commit"`
	NoChange     bool     `json:"no_change"`
	Upserted     []string `json:"upserted"`
	Deleted      []string `json:"deleted"`
	Errors       []string `json:"errors,omitempty"`
}

// isSupportedFile 检查是否支持的文件类型
func isSupportedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	supported := []string{".md", ".txt", ".json", ".yaml", ".yml", ".csv"}
	for _, s := range supported {
		if ext == s {
			return true
		}
	}
	return false
}

// chunkDocument 将文档分块
type Chunk struct {
	Text  string
	Start int
	End   int
}

func chunkDocument(text, path string) []Chunk {
	// 简单按段落分块，每块最多 1000 字符
	var chunks []Chunk
	paragraphs := strings.Split(text, "\n\n")

	currentChunk := ""
	startPos := 0

	for i, para := range paragraphs {
		if len(currentChunk)+len(para) > 1000 {
			if currentChunk != "" {
				chunks = append(chunks, Chunk{
					Text:  currentChunk,
					Start: startPos,
					End:   startPos + len(currentChunk),
				})
				startPos += len(currentChunk)
			}
			currentChunk = para
		} else {
			if currentChunk != "" {
				currentChunk += "\n\n"
			}
			currentChunk += para
		}
		_ = i
	}

	if currentChunk != "" {
		chunks = append(chunks, Chunk{
			Text:  currentChunk,
			Start: startPos,
			End:   startPos + len(currentChunk),
		})
	}

	return chunks
}

// Point Qdrant 向量点
type Point struct {
	ID      string
	Vector  []float32
	Payload map[string]interface{}
}

// QdrantClient 接口 (需要在实际代码中实现)
type QdrantClient interface {
	Upsert(ctx context.Context, point *Point) error
	DeleteByFilter(ctx context.Context, filter map[string]interface{}) error
}

// EmbeddingProvider 接口 (需要在实际代码中实现)
type EmbeddingProvider interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
