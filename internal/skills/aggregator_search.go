package skills

import (
	"context"
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
	// TODO: Call Brave Search MCP
	return []SearchResult{}, nil
}

func unifiedSearchAnnas(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// TODO: Call Annas Archive MCP
	return []SearchResult{}, nil
}

func unifiedSearchArxiv(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// TODO: Call arXiv MCP
	return []SearchResult{}, nil
}

func unifiedSearchGitHub(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// TODO: Implement GitHub search
	return []SearchResult{}, nil
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
	// 调用 courses.search skill
	skill := NewCoursesSearchSkill()
	result, err := skill.Invoke(ctx, map[string]any{
		"keyword": query,
		"limit":   topK,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("courses.search: %w", err)
	}

	data, ok := result["output"].(map[string]any)
	if !ok {
		return []SearchResult{}, nil
	}

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

	results := make([]SearchResult, 0)
	if courseData, ok := data["data"].(map[string]any); ok {
		if resultData, ok := courseData["result"].(map[string]any); ok {
			readmeMD, _ := resultData["readme_md"].(string)
			tomlContent, _ := resultData["readme_toml"].(string)

			results = append(results, SearchResult{
				Source:     "course_read",
				SourceType: "course",
				Title:      fmt.Sprintf("课程 %s README", courseCode),
				Content:    readmeMD,
				URL:        fmt.Sprintf("https://github.com/%s/%s", courseData["org"], courseData["repo"]),
				Score:      1.0,
				Metadata: map[string]any{
					"toml":   tomlContent,
					"campus": courseData["campus"],
					"repo":   courseData["repo"],
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
	// Build context
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("用户查询: %s\n\n搜索结果:\n\n", query))

	for i, r := range results {
		if i >= 10 {
			break
		}
		contentBuilder.WriteString(fmt.Sprintf("%d. %s (来源: %s)\n", i+1, r.Title, r.Source))
		contentBuilder.WriteString(fmt.Sprintf("   内容: %s\n\n", truncateStr(r.Content, 200)))
	}

	// TODO: Call LLM for summarization
	// For now, return placeholder
	return "总结功能待实现", []string{"需要接入 LLM"}
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
