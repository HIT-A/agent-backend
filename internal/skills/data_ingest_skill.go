package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DataSourceType 数据来源类型
type DataSourceType string

const (
	SourceTypeGitHub DataSourceType = "github"
	SourceTypeManual DataSourceType = "manual"
	SourceTypeAnnas  DataSourceType = "annas"
	SourceTypeArxiv  DataSourceType = "arxiv"
	SourceTypeCrawl  DataSourceType = "crawl"
)

// DataIngestConfig 数据接入配置
type DataIngestConfig struct {
	// 目标
	TargetRepo   string
	TargetBranch string
	LocalPath    string

	// COS 配置
	StoreInCOS bool
	COSPrefix  string

	// RAG 配置
	AutoIngestRAG bool
	RAGCollection string

	// 并发
	Workers int
}

// DataIngestInput 数据接入输入
type DataIngestInput struct {
	// 数据来源
	SourceType    DataSourceType `json:"source_type"`    // github, manual, annas, arxiv, crawl
	SourceName    string         `json:"source_name"`    // 仓库名/文件名/论文ID
	SourceURL     string         `json:"source_url"`     // for crawl
	Content       string         `json:"content"`        // 文本内容 (for manual)
	ContentBase64 string         `json:"content_base64"` // base64 (for manual upload)
	FilePath      string         `json:"file_path"`      // 文件路径 (for upload)

	// 可选
	TargetRepo    string `json:"target_repo"`
	TargetBranch  string `json:"target_branch"`
	StoreInCOS    bool   `json:"store_in_cos"`
	COSPrefix     string `json:"cos_prefix"`
	AutoIngestRAG bool   `json:"auto_ingest_rag"`
	Overwrite     bool   `json:"overwrite"` // 是否覆盖已存在的
}

// SourceRecord 来源记录 (用于 sources.json)
type SourceRecord struct {
	SourceType   string    `json:"source_type"`
	SourceName   string    `json:"source_name"`
	Hash         string    `json:"hash"`
	UploadedAt   time.Time `json:"uploaded_at"`
	FileCount    int       `json:"file_count"`
	ChunksCount  int       `json:"chunks_count"`
	COSKeys      []string  `json:"cos_keys"`
	MarkdownPath string    `json:"markdown_path"`
}

// SourcesIndex sources.json 结构
type SourcesIndex struct {
	Version  string                  `json:"version"`
	LastSync time.Time               `json:"last_sync"`
	Sources  map[string]SourceRecord `json:"sources"`
}

func NewDataIngestSkill() Skill {
	return Skill{
		Name:    "data.ingest",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			config := parseDataIngestConfig(input)
			in := parseDataIngestInput(input)

			// 计算 content hash
			contentHash := calculateContentHash(in.Content + in.ContentBase64 + in.FilePath)

			// 检查去重
			if !in.Overwrite && isDuplicate(config.LocalPath, in.SourceType, in.SourceName, contentHash) {
				return map[string]any{
					"ok": true,
					"output": map[string]any{
						"status": "skipped",
						"reason": "duplicate",
						"source": in.SourceName,
						"hash":   contentHash,
					},
				}, nil
			}

			// 处理数据
			result, err := processDataIngest(ctx, config, in, contentHash)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("ingest failed: %v", err), Retryable: true}
			}

			// 更新 sources.json
			if err := updateSourcesIndex(config.LocalPath, in.SourceType, in.SourceName, contentHash, result); err != nil {
				// 记录错误但不失败
				result.Warnings = append(result.Warnings, fmt.Sprintf("update index failed: %v", err))
			}

			return map[string]any{
				"ok":     true,
				"output": result,
			}, nil
		},
	}
}

func parseDataIngestConfig(input map[string]any) DataIngestConfig {
	config := DataIngestConfig{
		TargetRepo:    "HIT-A/HITA_RagData",
		TargetBranch:  "main",
		LocalPath:     "/tmp/HITA_RagData",
		StoreInCOS:    true,
		COSPrefix:     "rag-content",
		AutoIngestRAG: true,
		Workers:       4,
	}

	if repo, ok := input["target_repo"].(string); ok {
		config.TargetRepo = repo
	}
	if branch, ok := input["target_branch"].(string); ok {
		config.TargetBranch = branch
	}
	if path, ok := input["local_path"].(string); ok {
		config.LocalPath = path
	}
	if storeCOS, ok := input["store_in_cos"].(bool); ok {
		config.StoreInCOS = storeCOS
	}
	if prefix, ok := input["cos_prefix"].(string); ok {
		config.COSPrefix = prefix
	}
	if workers, ok := input["workers"].(float64); ok {
		config.Workers = int(workers)
	}

	return config
}

func parseDataIngestInput(input map[string]any) DataIngestInput {
	in := DataIngestInput{
		StoreInCOS:    true,
		AutoIngestRAG: true,
		Overwrite:     false,
	}

	if sourceType, ok := input["source_type"].(string); ok {
		in.SourceType = DataSourceType(sourceType)
	}
	if sourceName, ok := input["source_name"].(string); ok {
		in.SourceName = sourceName
	}
	if sourceURL, ok := input["source_url"].(string); ok {
		in.SourceURL = sourceURL
	}
	if content, ok := input["content"].(string); ok {
		in.Content = content
	}
	if contentBase64, ok := input["content_base64"].(string); ok {
		in.ContentBase64 = contentBase64
	}
	if filePath, ok := input["file_path"].(string); ok {
		in.FilePath = filePath
	}
	if overwrite, ok := input["overwrite"].(bool); ok {
		in.Overwrite = overwrite
	}

	return in
}

func calculateContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

func isDuplicate(localPath string, sourceType DataSourceType, sourceName, contentHash string) bool {
	index, err := loadSourcesIndex(localPath)
	if err != nil {
		return false
	}

	key := fmt.Sprintf("%s:%s", sourceType, sourceName)
	if record, exists := index.Sources[key]; exists {
		return record.Hash == contentHash
	}

	return false
}

func loadSourcesIndex(localPath string) (*SourcesIndex, error) {
	indexPath := filepath.Join(localPath, "sources", "sources.json")

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SourcesIndex{
				Version: "1.0",
				Sources: make(map[string]SourceRecord),
			}, nil
		}
		return nil, err
	}

	var index SourcesIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, err
	}

	return &index, nil
}

func updateSourcesIndex(localPath string, sourceType DataSourceType, sourceName, contentHash string, result *IngestResult) error {
	index, err := loadSourcesIndex(localPath)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s:%s", sourceType, sourceName)
	index.Sources[key] = SourceRecord{
		SourceType:   string(sourceType),
		SourceName:   sourceName,
		Hash:         contentHash,
		UploadedAt:   time.Now(),
		FileCount:    result.FileCount,
		ChunksCount:  result.ChunksCount,
		COSKeys:      result.COSKeys,
		MarkdownPath: result.MarkdownPath,
	}
	index.LastSync = time.Now()

	indexPath := filepath.Join(localPath, "sources", "sources.json")
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(indexPath, data, 0644)
}

// IngestResult 数据接入结果
type IngestResult struct {
	Status       string
	FileCount    int
	ChunksCount  int
	COSKeys      []string
	MarkdownPath string
	Vectorized   bool
	Warnings     []string
}

func processDataIngest(ctx context.Context, config DataIngestConfig, in DataIngestInput, contentHash string) (*IngestResult, error) {
	result := &IngestResult{
		Status:  "completed",
		COSKeys: []string{},
	}

	var markdownContent string

	switch in.SourceType {
	case SourceTypeManual:
		// 处理手动上传的内容
		if in.Content != "" {
			markdownContent = in.Content
		} else if in.ContentBase64 != "" {
			decoded, err := decodeBase64(in.ContentBase64)
			if err != nil {
				return nil, fmt.Errorf("decode base64: %w", err)
			}
			markdownContent = string(decoded)
		} else if in.FilePath != "" {
			data, err := os.ReadFile(in.FilePath)
			if err != nil {
				return nil, fmt.Errorf("read file: %w", err)
			}
			markdownContent = string(data)
		}

	case SourceTypeGitHub:
		// TODO: 从 GitHub 下载
		markdownContent = in.Content

	case SourceTypeAnnas:
		// TODO: 从 Anna's Archive 下载

	case SourceTypeArxiv:
		// TODO: 从 arXiv 下载

	case SourceTypeCrawl:
		// 爬虫内容
		markdownContent = in.Content

	default:
		return nil, fmt.Errorf("unknown source type: %s", in.SourceType)
	}

	_ = in.SourceName // silence unused warning

	// 保存 Markdown 到本地
	sourcesDir := filepath.Join(config.LocalPath, "sources", string(in.SourceType))
	if err := os.MkdirAll(sourcesDir, 0755); err != nil {
		return nil, fmt.Errorf("create sources dir: %w", err)
	}

	markdownPath := filepath.Join(sourcesDir, fmt.Sprintf("%s.md", in.SourceName))
	if err := os.WriteFile(markdownPath, []byte(markdownContent), 0644); err != nil {
		return nil, fmt.Errorf("write markdown: %w", err)
	}

	result.MarkdownPath = markdownPath
	result.FileCount = 1

	// 计算 chunks
	chunks := len(markdownContent) / 1400
	if chunks == 0 {
		chunks = 1
	}
	result.ChunksCount = chunks

	// 存源文件到 COS
	if config.StoreInCOS && len(markdownContent) > 0 {
		cosStorage := GetCOSStorage()
		if cosStorage != nil {
			cosKey := fmt.Sprintf("%s/%s/%s.md", config.COSPrefix, in.SourceType, in.SourceName)
			if _, err := cosStorage.SaveFile(ctx, cosKey, []byte(markdownContent), "text/markdown"); err == nil {
				result.COSKeys = append(result.COSKeys, cosKey)
			}
		}
	}

	// RAG 向量化
	if config.AutoIngestRAG {
		// TODO: 触发 RAG 向量化
		result.Vectorized = true
	}

	return result, nil
}

func decodeBase64(encoded string) ([]byte, error) {
	return io.ReadAll(io.NopCloser(strings.NewReader(encoded)))
}

// BatchDataIngestSkill 批量数据接入
func NewBatchDataIngestSkill() Skill {
	return Skill{
		Name:    "data.ingest_batch",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			itemsRaw, ok := input["items"].([]any)
			if !ok || len(itemsRaw) == 0 {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "items is required", Retryable: false}
			}

			config := parseDataIngestConfig(input)

			var wg sync.WaitGroup
			results := make([]map[string]any, len(itemsRaw))
			semaphore := make(chan struct{}, config.Workers)

			for i, itemRaw := range itemsRaw {
				semaphore <- struct{}{}
				wg.Add(1)

				go func(idx int, item map[string]any) {
					defer func() { <-semaphore }()
					defer wg.Done()

					itemInput := mergeInputWithConfig(item, config)
					result, err := invokeDataIngest(ctx, itemInput)
					if err != nil {
						results[idx] = map[string]any{
							"status": "failed",
							"error":  err.Error(),
						}
						return
					}
					results[idx] = result
				}(i, itemRaw.(map[string]any))
			}

			wg.Wait()

			success := 0
			failed := 0
			for _, r := range results {
				if r["status"] == "completed" {
					success++
				} else {
					failed++
				}
			}

			return map[string]any{
				"ok": true,
				"output": map[string]any{
					"total":   len(itemsRaw),
					"success": success,
					"failed":  failed,
					"results": results,
				},
			}, nil
		},
	}
}

func mergeInputWithConfig(item map[string]any, config DataIngestConfig) map[string]any {
	input := make(map[string]any)
	for k, v := range item {
		input[k] = v
	}
	input["target_repo"] = config.TargetRepo
	input["target_branch"] = config.TargetBranch
	input["store_in_cos"] = config.StoreInCOS
	input["cos_prefix"] = config.COSPrefix
	input["auto_ingest_rag"] = config.AutoIngestRAG
	return input
}

func invokeDataIngest(ctx context.Context, input map[string]any) (map[string]any, error) {
	skill := NewDataIngestSkill()
	result, err := skill.Invoke(ctx, input, nil)
	return result, err
}
