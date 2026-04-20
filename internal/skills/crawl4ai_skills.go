package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"
	"time"

	"hoa-agent-backend/internal/mcp"
)

// CrawlPageInput represents input for crawling a single page
type CrawlPageInput struct {
	URL           string   `json:"url"`
	WaitFor       string   `json:"wait_for,omitempty"`
	ExcludePaths  []string `json:"exclude_paths,omitempty"`
	ContentFilter bool     `json:"content_filter"`
	OutputFormat  string   `json:"output_format"`
	StoreInCOS    bool     `json:"store_in_cos"`
	COSPrefix     string   `json:"cos_prefix"`
	AutoIngestRAG bool     `json:"auto_ingest_rag"`
}

// CrawlSiteInput represents input for full site crawling
type CrawlSiteInput struct {
	StartURL             string   `json:"start_url"`
	MaxPages             int      `json:"max_pages"`
	MaxDepth             int      `json:"max_depth"`
	IncludePatterns      []string `json:"include_patterns,omitempty"`
	ExcludePatterns      []string `json:"exclude_patterns,omitempty"`
	ContentFilter        bool     `json:"content_filter"`
	UseSitemap           bool     `json:"use_sitemap"`
	ConcurrentRequests   int      `json:"concurrent_requests"`
	RespectRobotsTxt     bool     `json:"respect_robots_txt"`
	DelayBetweenRequests float64  `json:"delay_between_requests"`
	StoreInCOS           bool     `json:"store_in_cos"`
	COSPrefix            string   `json:"cos_prefix"`
	AutoIngestToRAG      bool     `json:"auto_ingest_to_rag"`
	RAGSource            string   `json:"rag_source"`
}

// NewCrawl4AIPageSkill creates a skill for crawling a single page
func NewCrawl4AIPageSkill(mcpRegistry *mcp.Registry) Skill {
	return Skill{
		Name:    "crawl4ai.page",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Parse input
			in := parseCrawlPageInput(input)

			// Validate URL
			if strings.TrimSpace(in.URL) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "url is required", Retryable: false}
			}

			// Get MCP server
			server, exists := mcpRegistry.Get("crawl4ai")
			if !exists {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: "crawl4ai MCP server not available in backend configuration", Retryable: false}
			}

			if !server.Initialized {
				return nil, &InvokeError{Code: "INTERNAL", Message: "crawl4ai MCP server not initialized", Retryable: true}
			}

			// Build arguments
			args := map[string]any{
				"url":            in.URL,
				"content_filter": in.ContentFilter,
				"output_format":  in.OutputFormat,
			}
			if in.WaitFor != "" {
				args["wait_for"] = in.WaitFor
			}
			if len(in.ExcludePaths) > 0 {
				args["exclude_paths"] = in.ExcludePaths
			}

			// Call MCP tool
			result, err := callMCPTool(ctx, server, "crawl_page", args)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("crawl failed: %v", err), Retryable: true}
			}

			// Extract markdown content for storage/RAG
			fileName, body := buildCrawlRawSnapshot(in.URL, result)
			if len(body) == 0 {
				return result, nil
			}

			// Direct RAG ingest
			if in.AutoIngestRAG && len(body) > 0 {
				qdrant, err := NewQdrantClientFromEnv()
				if err == nil {
					embedder, err := NewEmbeddingProviderFromEnv()
					if err == nil {
						ingested, iErr := IngestMarkdownDirect(ctx, body, fileName, "crawl4ai/"+fileName, qdrant, embedder)
						if iErr == nil {
							result["rag"] = map[string]any{"chunks": ingested}
						}
					}
				}
			}

			return result, nil
		},
	}
}

// NewCrawl4AISiteSkill creates a skill for full site crawling (async)
func NewCrawl4AISiteSkill(mcpRegistry *mcp.Registry) Skill {
	return Skill{
		Name:    "crawl4ai.site",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Parse input
			in := parseCrawlSiteInput(input)

			// Validate URL
			if strings.TrimSpace(in.StartURL) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "start_url is required", Retryable: false}
			}

			// Get MCP server
			server, exists := mcpRegistry.Get("crawl4ai")
			if !exists {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: "crawl4ai MCP server not available in backend configuration", Retryable: false}
			}

			if !server.Initialized {
				return nil, &InvokeError{Code: "INTERNAL", Message: "crawl4ai MCP server not initialized", Retryable: true}
			}

			// Build arguments
			args := map[string]any{
				"start_url":              in.StartURL,
				"max_pages":              in.MaxPages,
				"max_depth":              in.MaxDepth,
				"content_filter":         in.ContentFilter,
				"use_sitemap":            in.UseSitemap,
				"concurrent_requests":    in.ConcurrentRequests,
				"respect_robots_txt":     in.RespectRobotsTxt,
				"delay_between_requests": in.DelayBetweenRequests,
			}
			if len(in.IncludePatterns) > 0 {
				args["include_patterns"] = in.IncludePatterns
			}
			if len(in.ExcludePatterns) > 0 {
				args["exclude_patterns"] = in.ExcludePatterns
			}

			// Start crawl
			result, err := callMCPTool(ctx, server, "crawl_site", args)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("site crawl failed to start: %v", err), Retryable: true}
			}

			// Store crawl metadata for potential auto-processing
			if crawlID, ok := result["crawl_id"].(string); ok {
				result["auto_ingest_config"] = map[string]any{
					"enabled":      in.AutoIngestToRAG,
					"source":       in.RAGSource,
					"store_in_cos": in.StoreInCOS,
					"cos_prefix":   in.COSPrefix,
				}
				_ = crawlID // Could store in job metadata for post-processing
			}

			return result, nil
		},
	}
}

// NewCrawl4AIStatusSkill creates a skill for checking crawl status
func NewCrawl4AIStatusSkill(mcpRegistry *mcp.Registry) Skill {
	return Skill{
		Name:    "crawl4ai.status",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Parse input
			crawlID, ok := input["crawl_id"].(string)
			if !ok || crawlID == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "crawl_id is required", Retryable: false}
			}

			// Get MCP server
			server, exists := mcpRegistry.Get("crawl4ai")
			if !exists {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: "crawl4ai MCP server not registered", Retryable: false}
			}

			if !server.Initialized {
				return nil, &InvokeError{Code: "INTERNAL", Message: "crawl4ai MCP server not initialized", Retryable: true}
			}

			// Call MCP tool
			result, err := callMCPTool(ctx, server, "get_crawl_status", map[string]any{
				"crawl_id": crawlID,
			})
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("failed to get status: %v", err), Retryable: true}
			}

			return result, nil
		},
	}
}

// parseCrawlPageInput parses and validates crawl page input
func parseCrawlPageInput(input map[string]any) CrawlPageInput {
	in := CrawlPageInput{
		ContentFilter: true,
		OutputFormat:  "markdown",
		StoreInCOS:    true,
		COSPrefix:     "crawls",
		AutoIngestRAG: true,
	}

	if url, ok := input["url"].(string); ok {
		in.URL = url
	}
	if waitFor, ok := input["wait_for"].(string); ok {
		in.WaitFor = waitFor
	}
	if contentFilter, ok := input["content_filter"].(bool); ok {
		in.ContentFilter = contentFilter
	}
	if outputFormat, ok := input["output_format"].(string); ok {
		in.OutputFormat = outputFormat
	}
	if v, ok := input["queue_to_intake"].(bool); ok {
		in.AutoIngestRAG = v
	}
	if v, ok := input["store_in_cos"].(bool); ok {
		in.StoreInCOS = v
	}
	if v, ok := input["cos_prefix"].(string); ok {
		in.COSPrefix = v
	}
	if v, ok := input["auto_ingest_rag"].(bool); ok {
		in.AutoIngestRAG = v
	}
	if excludePaths, ok := input["exclude_paths"].([]any); ok {
		for _, path := range excludePaths {
			if p, ok := path.(string); ok {
				in.ExcludePaths = append(in.ExcludePaths, p)
			}
		}
	}

	return in
}

func buildCrawlRawSnapshot(rawURL string, result map[string]any) (string, []byte) {
	name := "crawl-result.md"
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		host := strings.ReplaceAll(strings.ToLower(u.Hostname()), ".", "-")
		if host == "" {
			host = "crawl"
		}
		base := path.Base(strings.TrimSpace(u.Path))
		if base == "" || base == "." || base == "/" {
			base = "index"
		}
		name = safeName(host+"-"+base) + ".md"
	}

	if md, ok := result["markdown"].(string); ok && strings.TrimSpace(md) != "" {
		return name, []byte(md)
	}
	if content, ok := result["content"].(string); ok && strings.TrimSpace(content) != "" {
		return name, []byte(content)
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	if strings.HasSuffix(name, ".md") {
		name = strings.TrimSuffix(name, ".md") + ".json"
	}
	return name, b
}

// parseCrawlSiteInput parses and validates crawl site input
func parseCrawlSiteInput(input map[string]any) CrawlSiteInput {
	in := CrawlSiteInput{
		MaxPages:             100,
		MaxDepth:             3,
		ContentFilter:        true,
		UseSitemap:           true,
		ConcurrentRequests:   5,
		RespectRobotsTxt:     true,
		DelayBetweenRequests: 0.5,
		StoreInCOS:           true,
		COSPrefix:            "crawls",
	}

	if url, ok := input["start_url"].(string); ok {
		in.StartURL = url
	}
	if maxPages, ok := input["max_pages"].(float64); ok {
		in.MaxPages = int(maxPages)
	}
	if maxDepth, ok := input["max_depth"].(float64); ok {
		in.MaxDepth = int(maxDepth)
	}
	if contentFilter, ok := input["content_filter"].(bool); ok {
		in.ContentFilter = contentFilter
	}
	if useSitemap, ok := input["use_sitemap"].(bool); ok {
		in.UseSitemap = useSitemap
	}
	if concurrentReqs, ok := input["concurrent_requests"].(float64); ok {
		in.ConcurrentRequests = int(concurrentReqs)
	}
	if respectRobots, ok := input["respect_robots_txt"].(bool); ok {
		in.RespectRobotsTxt = respectRobots
	}
	if delay, ok := input["delay_between_requests"].(float64); ok {
		in.DelayBetweenRequests = delay
	}
	if storeInCOS, ok := input["store_in_cos"].(bool); ok {
		in.StoreInCOS = storeInCOS
	}
	if cosPrefix, ok := input["cos_prefix"].(string); ok {
		in.COSPrefix = cosPrefix
	}
	if autoIngest, ok := input["auto_ingest_to_rag"].(bool); ok {
		in.AutoIngestToRAG = autoIngest
	}
	if ragSource, ok := input["rag_source"].(string); ok {
		in.RAGSource = ragSource
	}

	// Parse patterns
	if includePatterns, ok := input["include_patterns"].([]any); ok {
		for _, pattern := range includePatterns {
			if p, ok := pattern.(string); ok {
				in.IncludePatterns = append(in.IncludePatterns, p)
			}
		}
	}
	if excludePatterns, ok := input["exclude_patterns"].([]any); ok {
		for _, pattern := range excludePatterns {
			if p, ok := pattern.(string); ok {
				in.ExcludePatterns = append(in.ExcludePatterns, p)
			}
		}
	}

	return in
}

// callMCPTool calls a tool on an MCP server
func callMCPTool(ctx context.Context, server *mcp.RegisteredServer, toolName string, args map[string]any) (map[string]any, error) {
	if server.Client != nil && server.Initialized {
		result, err := server.Client.CallTool(ctx, toolName, args)
		if err != nil {
			return nil, fmt.Errorf("tool call failed: %w", err)
		}

		resultMap := map[string]any{}
		data, _ := json.Marshal(result)
		if err := json.Unmarshal(data, &resultMap); err != nil {
			return nil, fmt.Errorf("unmarshal result: %w", err)
		}
		return resultMap, nil
	}

	var transport mcp.Transport
	if server.Config.Transport == "http" {
		transport = mcp.NewHTTPTransport(server.Config.URL)
	} else if server.Config.Transport == "stdio" {
		if server.Config.LineDelimited {
			transport = mcp.NewLineDelimitedTransport(server.Config.Command, server.Config.Env)
		} else {
			transport = mcp.NewStdioTransport(server.Config.Command, server.Config.Env)
		}
	} else {
		return nil, fmt.Errorf("unsupported transport: %s", server.Config.Transport)
	}

	client := mcp.NewClient(transport)

	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Initialize(ctx2); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}
	defer client.Close()

	// Call tool with longer timeout
	ctx3, cancel2 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel2()

	result, err := client.CallTool(ctx3, toolName, args)
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}

	// Parse result from Content array
	var parsedResult map[string]any
	if len(result.Content) > 0 && result.Content[0].Type == "text" {
		if err := json.Unmarshal([]byte(result.Content[0].Text), &parsedResult); err != nil {
			return map[string]any{"raw_result": result.Content[0].Text}, nil
		}
	} else {
		parsedResult = map[string]any{"result": result}
	}

	return parsedResult, nil
}
