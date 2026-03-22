package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
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

			// 处理数据（dedup 在内部通过 SQLite/SHA256 检查）
			result, err := processDataIngest(ctx, config, in, "")
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("ingest failed: %v", err), Retryable: true}
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

	if index.Sources == nil {
		index.Sources = make(map[string]SourceRecord)
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

func processDataIngest(ctx context.Context, config DataIngestConfig, in DataIngestInput, _ string) (*IngestResult, error) {
	result := &IngestResult{
		Status:   "completed",
		COSKeys:  []string{},
		Warnings: []string{},
	}

	var markdownContent []byte

	switch in.SourceType {
	case SourceTypeManual:
		if in.Content != "" {
			markdownContent = []byte(in.Content)
		} else if in.ContentBase64 != "" {
			decoded, err := decodeBase64(in.ContentBase64)
			if err != nil {
				return nil, fmt.Errorf("decode base64: %w", err)
			}
			markdownContent = decoded
		} else if in.FilePath != "" {
			data, err := os.ReadFile(in.FilePath)
			if err != nil {
				return nil, fmt.Errorf("read file: %w", err)
			}
			markdownContent = data
		}

	case SourceTypeGitHub:
		markdownContent = []byte(in.Content)

	case SourceTypeAnnas:
		annasResult, err := downloadFromAnnasArchive(ctx, in.SourceName, in.SourceURL)
		if err != nil {
			return nil, fmt.Errorf("download from annas archive: %w", err)
		}
		markdownContent = []byte(annasResult)

	case SourceTypeArxiv:
		paperID := strings.TrimSpace(in.SourceName)
		if paperID == "" {
			return nil, fmt.Errorf("arXiv paper ID is required (source_name)")
		}
		paperContent, err := downloadArxivPaper(ctx, paperID)
		if err != nil {
			return nil, fmt.Errorf("download arxiv paper: %w", err)
		}
		markdownContent = []byte(paperContent)

	case SourceTypeCrawl:
		markdownContent = []byte(in.Content)

	default:
		return nil, fmt.Errorf("unknown source type: %s", in.SourceType)
	}

	if len(markdownContent) == 0 {
		return nil, fmt.Errorf("no content to ingest")
	}

	// Compute SHA256 for deduplication
	contentSHA := sha256.Sum256(markdownContent)
	sha256Hex := hex.EncodeToString(contentSHA[:])

	// Dedup check via SQLite
	dedupStore, err := NewDedupStoreFromEnv()
	if err != nil {
		return nil, fmt.Errorf("open dedup store: %w", err)
	}
	defer dedupStore.Close()

	shouldIngest, existingRecord, err := dedupStore.ShouldIngest(ctx, sha256Hex)
	if err != nil {
		return nil, fmt.Errorf("dedup check: %w", err)
	}
	if !shouldIngest && !in.Overwrite {
		result.Status = "skipped"
		result.COSKeys = []string{existingRecord.COSKey}
		result.Warnings = append(result.Warnings, fmt.Sprintf("duplicate SHA256: %s (existing COS: %s)", sha256Hex[:12], existingRecord.COSKey))
		return result, nil
	}

	// Save to COS
	var cosKey string
	cosStorage := GetCOSStorage()
	if config.StoreInCOS && cosStorage != nil {
		cosKey = fmt.Sprintf("%s/%s/%s.md", config.COSPrefix, in.SourceType, sha256Hex[:12]+"_"+in.SourceName)
		if _, err := cosStorage.SaveFile(ctx, cosKey, markdownContent, "text/markdown"); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("COS save failed: %v", err))
			cosKey = ""
		} else {
			result.COSKeys = append(result.COSKeys, cosKey)
		}
	}

	// Direct RAG ingest to Qdrant
	if config.AutoIngestRAG {
		qdrant, err := NewQdrantClientFromEnv()
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Qdrant init failed: %v", err))
		} else {
			embedder, err := NewEmbeddingProviderFromEnv()
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Embedder init failed: %v", err))
			} else {
				sourceTag := fmt.Sprintf("%s/%s", in.SourceType, in.SourceName)
				ingestedChunks, err := IngestMarkdownDirect(ctx, markdownContent, in.SourceName, sourceTag, cosStorage, GetMCPRegistry(), qdrant, embedder)
				if err != nil {
					result.Warnings = append(result.Warnings, fmt.Sprintf("RAG ingest failed: %v", err))
				} else {
					result.ChunksCount = ingestedChunks
					result.Vectorized = true
				}
			}
		}
	}

	// Record to dedup store
	if cosKey != "" {
		if err := dedupStore.Record(ctx, sha256Hex, cosKey, int64(len(markdownContent))); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("dedup record failed: %v", err))
		}
	}

	result.FileCount = 1
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

func downloadArxivPaper(ctx context.Context, paperID string) (string, error) {
	arxivID := strings.TrimPrefix(strings.TrimSpace(paperID), "https://arxiv.org/abs/")
	arxivID = strings.TrimPrefix(arxivID, "http://arxiv.org/abs/")

	apiURL := fmt.Sprintf("https://export.arxiv.org/api/query?id_list=%s", arxivID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/atom+xml")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch arxiv: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return "", fmt.Errorf("arxiv API failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return parseArxivAtomFeed(string(body), arxivID)
}

type ArxivFeed struct {
	Entries []ArxivEntry `xml:"entry"`
}

type ArxivEntry struct {
	Title      string        `xml:"title"`
	Summary    string        `xml:"summary"`
	Published  string        `xml:"published"`
	Updated    string        `xml:"updated"`
	Authors    []ArxivAuthor `xml:"author"`
	Categories []ArxivCat    `xml:"category"`
	PrimaryCat string        `xml:"primary_category"`
	Links      []ArxivLink   `xml:"link"`
}

type ArxivAuthor struct {
	Name string `xml:"name"`
}

type ArxivCat struct {
	Term string `xml:"term,attr"`
}

type ArxivLink struct {
	Title string `xml:"title,attr"`
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr"`
}

func parseArxivAtomFeed(xmlContent, paperID string) (string, error) {
	var feed ArxivFeed
	if err := xml.Unmarshal([]byte(xmlContent), &feed); err != nil {
		return "", fmt.Errorf("parse atom feed: %w", err)
	}

	if len(feed.Entries) == 0 {
		return "", fmt.Errorf("no entry found for paper %s", paperID)
	}

	entry := feed.Entries[0]

	title := strings.TrimSpace(strings.ReplaceAll(entry.Title, "\n", " "))
	summary := strings.TrimSpace(strings.ReplaceAll(entry.Summary, "\n", " "))

	var authors []string
	for _, a := range entry.Authors {
		authors = append(authors, a.Name)
	}

	var categories []string
	for _, c := range entry.Categories {
		if c.Term != "" {
			categories = append(categories, c.Term)
		}
	}

	primaryCat := ""
	if len(categories) > 0 {
		primaryCat = categories[0]
	}

	pdfURL := ""
	for _, link := range entry.Links {
		if link.Title == "pdf" {
			pdfURL = link.Href
			break
		}
	}

	var buffer strings.Builder
	buffer.WriteString(fmt.Sprintf("# %s\n\n", title))
	buffer.WriteString(fmt.Sprintf("**arXiv ID:** %s\n", paperID))
	buffer.WriteString(fmt.Sprintf("**Published:** %s\n", entry.Published))
	buffer.WriteString(fmt.Sprintf("**Authors:** %s\n", strings.Join(authors, ", ")))
	if primaryCat != "" {
		buffer.WriteString(fmt.Sprintf("**Primary Category:** %s\n", primaryCat))
	}
	if len(categories) > 0 && len(categories) < 10 {
		buffer.WriteString(fmt.Sprintf("**Categories:** %s\n", strings.Join(categories, ", ")))
	}
	if pdfURL != "" {
		buffer.WriteString(fmt.Sprintf("**PDF:** [%s](%s)\n\n", pdfURL, pdfURL))
	}
	buffer.WriteString("---\n\n")
	buffer.WriteString("## Abstract\n\n")
	buffer.WriteString(summary)
	buffer.WriteString("\n")

	return buffer.String(), nil
}

func downloadFromAnnasArchive(ctx context.Context, sourceName, sourceURL string) (string, error) {
	if sourceURL == "" && sourceName == "" {
		return "", fmt.Errorf("either source_name (Anna's Archive ID) or source_url is required")
	}

	registry := GetMCPRegistry()
	if registry == nil {
		return "", fmt.Errorf("MCP registry not initialized")
	}

	server, exists := registry.Get("annas-archive")
	if !exists || !server.Initialized {
		return "", fmt.Errorf("annas-archive MCP server not available")
	}

	for _, tool := range server.Tools {
		if strings.Contains(strings.ToLower(tool.Name), "download") {
			args := map[string]any{}
			if sourceURL != "" {
				args["url"] = sourceURL
			} else {
				args["id"] = sourceName
			}

			result, err := callMCPTool(ctx, server, tool.Name, args)
			if err != nil {
				return "", fmt.Errorf("annas download failed: %w", err)
			}

			if content, ok := result["content"].([]any); ok && len(content) > 0 {
				if first, ok := content[0].(map[string]any); ok {
					if text, ok := first["text"].(string); ok {
						return text, nil
					}
				}
			}

			if text, ok := result["text"].(string); ok {
				return text, nil
			}

			return "", fmt.Errorf("annas download returned no content")
		}
	}

	return "", fmt.Errorf("annas-archive has no download tool available")
}
