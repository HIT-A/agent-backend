package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// SearchResult represents a single search result
type SearchResult struct {
	Source     string         `json:"source"`
	SourceType string         `json:"source_type"` // course, knowledge, web, paper, book
	Title      string         `json:"title"`
	Content    string         `json:"content"`
	URL        string         `json:"url,omitempty"`
	Score      float64        `json:"score"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	Timestamp  time.Time      `json:"timestamp,omitempty"`
}

// UnifiedSearchInput represents unified search input (new search skill)
type UnifiedSearchInput struct {
	Query     string
	Sources   []string
	TopK      int
	Summarize bool
	Lang      string
	Timeout   int
}

// UnifiedSearchOutput represents unified search output
type UnifiedSearchOutput struct {
	Query        string         `json:"query"`
	Total        int            `json:"total"`
	Results      []SearchResult `json:"results"`
	BySource     map[string]int `json:"by_source"`
	BySourceType map[string]int `json:"by_source_type"`
	Summary      string         `json:"summary,omitempty"`
	KeyPoints    []string       `json:"key_points,omitempty"`
	DurationMs   int64          `json:"duration_ms"`
	Errors       []string       `json:"errors,omitempty"`
}

// NewUnifiedSearchSkill creates unified search skill
func NewUnifiedSearchSkill() Skill {
	return Skill{
		Name:    "search",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			in := parseUnifiedSearchInput(input)

			if len(in.Sources) == 0 {
				in.Sources = []string{"rag", "brave"}
			}
			if in.TopK <= 0 {
				in.TopK = 10
			}
			if in.Timeout <= 0 {
				in.Timeout = 30
			}

			startTime := time.Now()

			results, errors := executeUnifiedSearch(ctx, in)

			sort.Slice(results, func(i, j int) bool {
				return results[i].Score > results[j].Score
			})

			if len(results) > in.TopK {
				results = results[:in.TopK]
			}

			bySource := make(map[string]int)
			bySourceType := make(map[string]int)
			for _, r := range results {
				bySource[r.Source]++
				bySourceType[r.SourceType]++
			}

			output := UnifiedSearchOutput{
				Query:        in.Query,
				Total:        len(results),
				Results:      results,
				BySource:     bySource,
				BySourceType: bySourceType,
				DurationMs:   time.Since(startTime).Milliseconds(),
				Errors:       errors,
			}

			if in.Summarize && len(results) > 0 {
				summary, keyPoints := summarizeWithLLM(ctx, in.Query, results)
				output.Summary = summary
				output.KeyPoints = keyPoints
			}

			return map[string]any{
				"ok":     true,
				"output": output,
			}, nil
		},
	}
}

func parseUnifiedSearchInput(input map[string]any) UnifiedSearchInput {
	in := UnifiedSearchInput{
		TopK:      10,
		Summarize: false,
		Lang:      "zh",
		Timeout:   30,
	}

	if query, ok := input["query"].(string); ok {
		in.Query = query
	}
	if sources, ok := input["sources"].([]any); ok {
		for _, s := range sources {
			if sStr, ok := s.(string); ok {
				in.Sources = append(in.Sources, sStr)
			}
		}
	}
	if topK, ok := input["top_k"].(float64); ok {
		in.TopK = int(topK)
	}
	if summarize, ok := input["summarize"].(bool); ok {
		in.Summarize = summarize
	}
	if lang, ok := input["lang"].(string); ok {
		in.Lang = lang
	}
	if timeout, ok := input["timeout"].(float64); ok {
		in.Timeout = int(timeout)
	}

	return in
}

func executeUnifiedSearch(ctx context.Context, in UnifiedSearchInput) ([]SearchResult, []string) {
	results := make([]SearchResult, 0)
	errors := make([]string, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	resultChan := make(chan []SearchResult, len(in.Sources))
	errorChan := make(chan string, len(in.Sources))

	for _, source := range in.Sources {
		wg.Add(1)
		go func(src string) {
			defer wg.Done()

			var res []SearchResult
			var err error

			switch src {
			case "rag":
				res, err = unifiedSearchRAG(ctx, in.Query, in.TopK)
			case "brave":
				res, err = unifiedSearchBrave(ctx, in.Query, in.TopK)
			case "annas":
				res, err = unifiedSearchAnnas(ctx, in.Query, in.TopK)
			case "arxiv":
				res, err = unifiedSearchArxiv(ctx, in.Query, in.TopK)
			case "github":
				res, err = unifiedSearchGitHub(ctx, in.Query, in.TopK)
			case "cos":
				res, err = unifiedSearchCOS(ctx, in.Query, in.TopK)
			case "course":
				res, err = unifiedSearchCourse(ctx, in.Query, in.TopK)
			case "course_read":
				res, err = unifiedSearchCourseRead(ctx, in.Query, in.TopK)
			case "hit_teacher":
				res, err = unifiedSearchHitTeacher(ctx, in.Query, in.TopK)
			default:
				err = fmt.Errorf("unknown source: %s", src)
			}

			if err != nil {
				errorChan <- fmt.Sprintf("%s: %v", src, err)
				resultChan <- nil
				return
			}

			resultChan <- res
			errorChan <- ""
		}(source)
	}

	go func() {
		wg.Wait()
		close(resultChan)
		close(errorChan)
	}()

	for res := range resultChan {
		if res != nil {
			mu.Lock()
			results = append(results, res...)
			mu.Unlock()
		}
	}

	for err := range errorChan {
		if err != "" {
			mu.Lock()
			errors = append(errors, err)
			mu.Unlock()
		}
	}

	return results, errors
}

func unifiedSearchRAG(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	provider, err := NewEmbeddingProviderFromEnv()
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}

	vec, err := provider.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	qdrant, err := NewQdrantClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("qdrant: %w", err)
	}

	hits, err := qdrant.Search(ctx, vec, topK)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	results := make([]SearchResult, len(hits))
	for i, hit := range hits {
		results[i] = SearchResult{
			Source:     "rag",
			SourceType: "knowledge",
			Title:      hit.Title,
			Content:    hit.Snippet,
			URL:        hit.URL,
			Score:      hit.Score,
			Metadata:   map[string]any{"doc_id": hit.DocID, "chunk_id": hit.ChunkID},
		}
	}

	return results, nil
}

func unifiedSearchBrave(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	registry := GetMCPRegistry()
	if registry == nil {
		return nil, fmt.Errorf("mcp registry not initialized")
	}

	server, exists := registry.Get("brave-search")
	if !exists {
		return nil, fmt.Errorf("brave-search MCP server not found")
	}
	if !server.Initialized {
		return nil, fmt.Errorf("brave-search MCP server not initialized")
	}

	toolResult, err := callMCPTool(ctx, server, "brave_web_search", map[string]any{
		"query": query,
		"count": topK,
	})
	if err != nil {
		return nil, fmt.Errorf("brave_web_search: %w", err)
	}

	type braveItem struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Description string `json:"description"`
	}

	results := make([]SearchResult, 0)

	// Try parsing from text content (MCP returns JSON in content field)
	if content, ok := toolResult["content"].([]any); ok && len(content) > 0 {
		if first, ok := content[0].(map[string]any); ok {
			if text, ok := first["text"].(string); ok && strings.TrimSpace(text) != "" {
				var payload struct {
					Results []braveItem `json:"results"`
				}
				if err := json.Unmarshal([]byte(text), &payload); err == nil {
					for _, item := range payload.Results {
						if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.URL) == "" {
							continue
						}
						results = append(results, SearchResult{
							Source:     "brave",
							SourceType: "web",
							Title:      item.Title,
							Content:    item.Description,
							URL:        item.URL,
							Score:      0.7,
						})
					}
					return results, nil
				}
			}
		}
	}

	// Fallback: try parsing from direct results field
	if rawResults, ok := toolResult["results"].([]any); ok {
		for _, d := range rawResults {
			if m, ok := d.(map[string]any); ok {
				item := braveItem{}
				if v, ok := m["title"].(string); ok {
					item.Title = v
				}
				if v, ok := m["url"].(string); ok {
					item.URL = v
				}
				if v, ok := m["description"].(string); ok {
					item.Description = v
				}
				if strings.TrimSpace(item.Title) == "" && strings.TrimSpace(item.URL) == "" {
					continue
				}
				results = append(results, SearchResult{
					Source:     "brave",
					SourceType: "web",
					Title:      item.Title,
					Content:    item.Description,
					URL:        item.URL,
					Score:      0.7,
				})
			}
		}
	}

	return results, nil
}

func unifiedSearchAnnas(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	registry := GetMCPRegistry()
	if registry == nil {
		return nil, fmt.Errorf("mcp registry not initialized")
	}

	server, exists := registry.Get("annas-archive")
	if !exists {
		return nil, fmt.Errorf("annas-archive MCP server not found")
	}
	if !server.Initialized {
		return nil, fmt.Errorf("annas-archive MCP server not initialized")
	}

	// Anna's Archive has two search tools: book_search and article_search
	// Try article_search first (for DOI or academic papers)
	results := make([]SearchResult, 0)
	for _, tool := range server.Tools {
		if tool.Name == "article_search" {
			toolResult, err := callMCPTool(ctx, server, "article_search", map[string]any{
				"query": query,
			})
			if err == nil {
				results = parseAnnasTextResults(toolResult, "article", topK)
			}
			break
		}
	}

	// If no results from article_search, try book_search
	if len(results) == 0 {
		for _, tool := range server.Tools {
			if tool.Name == "book_search" {
				toolResult, err := callMCPTool(ctx, server, "book_search", map[string]any{
					"query": query,
				})
				if err == nil {
					results = parseAnnasTextResults(toolResult, "book", topK)
				}
				break
			}
		}
	}

	return results, nil
}

func parseAnnasTextResults(toolResult map[string]any, sourceType string, topK int) []SearchResult {
	results := make([]SearchResult, 0)

	var text string

	// Handle MCP result that has content array with text
	if content, ok := toolResult["content"].([]any); ok && len(content) > 0 {
		if first, ok := content[0].(map[string]any); ok {
			if t, ok := first["text"].(string); ok && strings.TrimSpace(t) != "" {
				text = t
			}
		}
	}

	// Fallback: Handle raw_result (when JSON parsing failed)
	if text == "" {
		if raw, ok := toolResult["raw_result"].(string); ok && strings.TrimSpace(raw) != "" {
			text = raw
		}
	}

	if text != "" {
		entries := strings.Split(text, "\n\n")
		for i, entry := range entries {
			if i >= topK {
				break
			}
			if strings.TrimSpace(entry) == "" || strings.HasPrefix(entry, "No ") {
				continue
			}

			result := parseAnnasEntry(entry, sourceType)
			if result.Title != "" {
				results = append(results, result)
			}
		}
	}

	return results
}

func parseAnnasEntry(entry string, sourceType string) SearchResult {
	result := SearchResult{
		Source:     "annas",
		SourceType: sourceType,
		Score:      0.8,
	}

	lines := strings.Split(entry, "\n")
	for _, line := range lines {
		if idx := strings.Index(line, ":"); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])

			switch strings.ToLower(key) {
			case "title":
				result.Title = value
			case "authors":
				result.Content = "Authors: " + value + "\n"
			case "publisher":
				result.Content += "Publisher: " + value + "\n"
			case "language":
				result.Content += "Language: " + value + "\n"
			case "format":
				result.Content += "Format: " + value + "\n"
			case "size":
				result.Content += "Size: " + value + "\n"
			case "url":
				result.URL = value
			case "hash":
				result.Metadata = map[string]any{"hash": value}
			case "journal":
				result.Content += "Journal: " + value + "\n"
			case "doi":
				result.Metadata = map[string]any{"doi": value}
			}
		}
	}

	return result
}

func unifiedSearchArxiv(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	registry := GetMCPRegistry()
	if registry == nil {
		return nil, fmt.Errorf("mcp registry not initialized")
	}

	server, exists := registry.Get("arxiv")
	if !exists {
		return nil, fmt.Errorf("arxiv MCP server not found")
	}
	if !server.Initialized {
		return nil, fmt.Errorf("arxiv MCP server not initialized")
	}

	if topK <= 0 {
		topK = 10
	}
	if topK > 50 {
		topK = 50
	}

	toolResult, err := callMCPTool(ctx, server, "search_arxiv", map[string]any{
		"query":       query,
		"max_results": topK,
		"sort_by":     "relevance",
	})
	if err != nil {
		return nil, fmt.Errorf("search_arxiv: %w", err)
	}

	type arxivPaper struct {
		ID        string   `json:"id"`
		Title     string   `json:"title"`
		Summary   string   `json:"summary"`
		Authors   []string `json:"authors"`
		Published string   `json:"published"`
		URL       string   `json:"url"`
		PDFURL    string   `json:"pdf_url"`
	}

	type arxivPayload struct {
		Query    string       `json:"query"`
		Returned int          `json:"returned"`
		Papers   []arxivPaper `json:"papers"`
	}

	payload := arxivPayload{}
	if content, ok := toolResult["content"].([]any); ok && len(content) > 0 {
		if first, ok := content[0].(map[string]any); ok {
			if text, ok := first["text"].(string); ok && strings.TrimSpace(text) != "" {
				_ = json.Unmarshal([]byte(text), &payload)
			}
		}
	}

	if len(payload.Papers) == 0 {
		if papersRaw, ok := toolResult["papers"].([]any); ok {
			for _, p := range papersRaw {
				if m, ok := p.(map[string]any); ok {
					paper := arxivPaper{}
					if v, ok := m["id"].(string); ok {
						paper.ID = v
					}
					if v, ok := m["title"].(string); ok {
						paper.Title = v
					}
					if v, ok := m["summary"].(string); ok {
						paper.Summary = v
					}
					if v, ok := m["authors"].([]any); ok {
						for _, a := range v {
							if s, ok := a.(string); ok {
								paper.Authors = append(paper.Authors, s)
							}
						}
					}
					if v, ok := m["url"].(string); ok {
						paper.URL = v
					}
					if v, ok := m["pdf_url"].(string); ok {
						paper.PDFURL = v
					}
					payload.Papers = append(payload.Papers, paper)
				}
			}
		}
	}

	results := make([]SearchResult, 0, len(payload.Papers))
	for _, paper := range payload.Papers {
		authors := strings.Join(paper.Authors, ", ")
		content := fmt.Sprintf("Authors: %s\nPublished: %s\n\n%s", authors, paper.Published, paper.Summary)
		if len(content) > 500 {
			content = content[:500] + "..."
		}

		url := paper.URL
		if url == "" && paper.ID != "" {
			url = "https://arxiv.org/abs/" + paper.ID
		}

		results = append(results, SearchResult{
			Source:     "arxiv",
			SourceType: "paper",
			Title:      paper.Title,
			Content:    content,
			URL:        url,
			Score:      0.8,
			Metadata:   map[string]any{"paper_id": paper.ID, "pdf_url": paper.PDFURL},
		})
	}

	return results, nil
}

func unifiedSearchGitHub(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	fetcher, err := NewGitHubFetcherFromEnv()
	if err != nil {
		return nil, fmt.Errorf("GitHub fetcher init failed: %w", err)
	}

	if topK <= 0 {
		topK = 10
	}
	if topK > 100 {
		topK = 100
	}

	searchResults, err := fetcher.SearchCode(ctx, query, topK)
	if err != nil {
		return nil, fmt.Errorf("GitHub search failed: %w", err)
	}

	results := make([]SearchResult, 0, len(searchResults.Items))
	for _, item := range searchResults.Items {
		content := fmt.Sprintf("Repository: %s\nPath: %s", item.Repository.FullName, item.Path)
		if len(item.TextMatches) > 0 {
			if fragment, ok := item.TextMatches[0]["fragment"].(string); ok {
				content = fragment
			}
		}

		results = append(results, SearchResult{
			Source:     "github",
			SourceType: "code",
			Title:      fmt.Sprintf("%s/%s", item.Repository.FullName, item.Path),
			Content:    content,
			URL:        fmt.Sprintf("https://github.com/%s/blob/main/%s", item.Repository.FullName, item.Path),
			Score:      item.Score / float64(searchResults.TotalCount+1),
		})
	}

	return results, nil
}

func unifiedSearchCOS(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	storage := GetCOSStorage()
	if storage == nil {
		return nil, fmt.Errorf("COS not configured")
	}

	files, err := storage.ListFiles(ctx, query, topK)
	if err != nil {
		return nil, fmt.Errorf("COS list: %w", err)
	}

	results := make([]SearchResult, len(files))
	for i, f := range files {
		key, _ := f["key"].(string)
		results[i] = SearchResult{
			Source:     "cos",
			SourceType: "file",
			Title:      key,
			Content:    fmt.Sprintf("Size: %v bytes", f["size"]),
			Score:      0.5,
		}
	}

	return results, nil
}

func unifiedSearchCourse(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// 调用 courses.search skill（网络抖动导致EOF时重试一次）
	skill := NewCoursesSearchSkill()
	invokeInput := map[string]any{
		"keyword": query,
		"limit":   topK,
	}

	result, err := skill.Invoke(ctx, invokeInput, nil)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "eof") {
			time.Sleep(120 * time.Millisecond)
			result, err = skill.Invoke(ctx, invokeInput, nil)
		}
		if err != nil {
			// 聚合搜索里course源失败时降级为空，避免拖垮整体检索体验。
			return []SearchResult{}, nil
		}
	}

	data, ok := result["output"].(map[string]any)
	if !ok {
		return []SearchResult{}, nil
	}
	data = unwrapSkillOutput(data)

	results := make([]SearchResult, 0)
	if resultsData, ok := data["data"].(map[string]any); ok {
		if courses, ok := resultsData["results"].([]any); ok {
			for _, c := range courses {
				if course, ok := c.(map[string]any); ok {
					code, _ := course["code"].(string)
					name, _ := course["name"].(string)
					repo, _ := course["repo"].(string)
					org, _ := course["org"].(string)

					results = append(results, SearchResult{
						Source:     "course",
						SourceType: "course",
						Title:      fmt.Sprintf("%s - %s", code, name),
						Content:    fmt.Sprintf("课程代码: %s, 仓库: %s/%s", code, org, repo),
						URL:        fmt.Sprintf("https://github.com/%s/%s", org, repo),
						Score:      0.9,
						Metadata:   course,
					})
				}
			}
		}
	}

	return results, nil
}

func unifiedSearchCourseRead(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// 提取课程代码
	courseCode := extractCourseCode(query)
	if courseCode == "" {
		return []SearchResult{}, nil
	}

	// 调用 course.read skill
	skill := NewCourseReadSkill()
	result, err := skill.Invoke(ctx, map[string]any{
		"course_code": courseCode,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("course.read: %w", err)
	}

	data, ok := result["output"].(map[string]any)
	if !ok {
		return []SearchResult{}, nil
	}
	data = unwrapSkillOutput(data)

	results := make([]SearchResult, 0)
	if courseData, ok := data["data"].(map[string]any); ok {
		if resultData, ok := courseData["result"].(map[string]any); ok {
			readmeMD, _ := resultData["readme_md"].(string)
			tomlContent, _ := resultData["readme_toml"].(string)
			baseData, _ := courseData["base"].(map[string]any)
			repo, _ := baseData["repo"].(string)
			org, _ := baseData["org"].(string)

			results = append(results, SearchResult{
				Source:     "course_read",
				SourceType: "course",
				Title:      fmt.Sprintf("课程 %s README", courseCode),
				Content:    readmeMD,
				URL:        fmt.Sprintf("https://github.com/%s/%s", org, repo),
				Score:      1.0,
				Metadata: map[string]any{
					"toml": tomlContent,
					"repo": repo,
					"org":  org,
				},
			})
		}
	}

	return results, nil
}

func unifiedSearchHitTeacher(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// 从查询中提取老师姓名或拼音
	pinyin := extractPinyin(query)
	if pinyin == "" {
		return []SearchResult{}, nil
	}

	// 调用 hit.teacher skill
	skill := NewTeacherSearchSkill(nil)
	result, err := skill.Invoke(ctx, map[string]any{
		"pinyin": pinyin,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("hit.teacher: %w", err)
	}

	output, ok := result["output"].(map[string]any)
	if !ok {
		return []SearchResult{}, nil
	}

	teacher, ok := output["teacher"].(map[string]any)
	if !ok {
		return []SearchResult{}, nil
	}

	name, _ := teacher["name"].(string)
	title, _ := teacher["title"].(string)
	dept, _ := teacher["department"].(string)
	email, _ := teacher["email"].(string)
	homepage, _ := teacher["homepage"].(string)

	content := fmt.Sprintf("职称: %s\n院系: %s\n邮箱: %s", title, dept, email)

	results := []SearchResult{{
		Source:     "hit_teacher",
		SourceType: "teacher",
		Title:      name,
		Content:    content,
		URL:        homepage,
		Score:      0.95,
		Metadata:   teacher,
	}}

	return results, nil
}

func extractCourseCode(query string) string {
	// 简单提取课程代码
	if len(query) >= 6 {
		return query[:6]
	}
	return ""
}

func extractPinyin(query string) string {
	// 如果查询包含中文，返回空（需要单独处理）
	// 这里简化处理：假设查询直接是拼音
	return query
}

func summarizeWithLLM(ctx context.Context, query string, results []SearchResult) (string, []string) {
	req := &SummarizeRequest{
		Query:     query,
		Results:   results,
		Style:     "concise",
		MaxLength: 500,
		Language:  "auto",
	}

	prompt := buildSummarizationPrompt(req)
	summary, keyPoints, err := callBigModelForSummary(ctx, prompt, req.MaxLength)
	if err != nil {
		return fmt.Sprintf("搜索结果共 %d 条，主要涉及: %s", len(results), query), nil
	}
	if keyPoints == nil {
		keyPoints = []string{}
	}
	return summary, keyPoints
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func unwrapSkillOutput(data map[string]any) map[string]any {
	current := data
	for i := 0; i < 3; i++ {
		next, ok := current["output"].(map[string]any)
		if !ok {
			break
		}
		current = next
	}
	return current
}
