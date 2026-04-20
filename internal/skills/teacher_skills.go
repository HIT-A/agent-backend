package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/mozillazg/go-pinyin"

	"hoa-agent-backend/internal/mcp"
)

type TeacherSearchInput struct {
	Name   string `json:"name"`
	Pinyin string `json:"pinyin"`
}

// NewTeacherSearchSkill creates a skill for searching HIT teachers
func NewTeacherSearchSkill(mcpRegistry *mcp.Registry) Skill {
	return Skill{
		Name:    "hit.teacher",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			in := parseTeacherSearchInput(input)

			if in.Name == "" && in.Pinyin == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "name or pinyin is required", Retryable: false}
			}

			server, exists := mcpRegistry.Get("crawl4ai")
			if !exists {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: "crawl4ai MCP server not registered", Retryable: false}
			}
			if !server.Initialized {
				return nil, &InvokeError{Code: "INTERNAL", Message: "crawl4ai MCP server not initialized", Retryable: true}
			}

			pinyin := in.Pinyin
			if pinyin == "" && in.Name != "" {
				pinyin = convertToPinyin(in.Name)
			}

			teacherURL := fmt.Sprintf("https://homepage.hit.edu.cn/%s?lang=zh", pinyin)

			args := map[string]any{
				"url":            teacherURL,
				"content_filter": true,
				"output_format":  "markdown",
			}

			result, err := callMCPTool(ctx, server, "crawl_page", args)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("failed to crawl teacher page: %v", err), Retryable: true}
			}

			markdown := extractCrawlContent(result)
			title := extractCrawlTitle(result)
			if title == "未开通" || title == "noFound" || strings.Contains(markdown, "noFound") {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: fmt.Sprintf("teacher %s (pinyin: %s) has no homepage", in.Name, pinyin), Retryable: false}
			}

			response := map[string]any{
				"success":  true,
				"name":     in.Name,
				"pinyin":   pinyin,
				"homepage": teacherURL,
				"markdown": markdown,
			}

			return response, nil
		},
	}
}

// NewTeacherBatchSearchSkill creates a skill for batch searching teachers
func NewTeacherBatchSearchSkill(mcpRegistry *mcp.Registry) Skill {
	return Skill{
		Name:    "hit.teachers",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Parse input
			names := toStringSlice(input["names"])
			pinyins := toStringSlice(input["pinyins"])
			maxWorkers, _ := input["max_workers"].(float64)

			if len(names) == 0 && len(pinyins) == 0 {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "names or pinyins is required", Retryable: false}
			}

			workers := 4
			if maxWorkers > 0 {
				workers = int(maxWorkers)
			}

			// Get MCP server
			server, exists := mcpRegistry.Get("crawl4ai")
			if !exists {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: "crawl4ai MCP server not registered", Retryable: false}
			}

			if !server.Initialized {
				return nil, &InvokeError{Code: "INTERNAL", Message: "crawl4ai MCP server not initialized", Retryable: true}
			}

			// Build task list
			type task struct {
				index  int
				pinyin string
				name   string
			}

			var tasks []task
			for i, name := range names {
				pinyin := ""
				if i < len(pinyins) {
					pinyin = pinyins[i]
				}
				if pinyin == "" {
					pinyin = convertToPinyin(name)
				}
				tasks = append(tasks, task{index: i, pinyin: pinyin, name: name})
			}

			// Process concurrently
			results := make([]map[string]any, len(tasks))
			semaphore := make(chan struct{}, workers)
			var wg sync.WaitGroup

			for i, t := range tasks {
				wg.Add(1)
				semaphore <- struct{}{}
				go func(idx int, pinyin, name string) {
					defer func() { <-semaphore; wg.Done() }()

					teacherURL := fmt.Sprintf("https://homepage.hit.edu.cn/%s?lang=zh", pinyin)
					args := map[string]any{
						"url":            teacherURL,
						"content_filter": true,
						"output_format":  "markdown",
					}

					result, err := callMCPTool(ctx, server, "crawl_page", args)
					if err != nil {
						results[idx] = map[string]any{
							"success":  false,
							"error":    err.Error(),
							"pinyin":   pinyin,
							"name":     name,
							"homepage": teacherURL,
						}
						return
					}

					markdown := extractCrawlContent(result)
					results[idx] = map[string]any{
						"success":  true,
						"name":     name,
						"pinyin":   pinyin,
						"homepage": teacherURL,
						"markdown": markdown,
					}
				}(i, t.pinyin, t.name)
			}

			wg.Wait()

			// Count successes
			successCount := 0
			for _, r := range results {
				if r != nil && r["success"] == true {
					successCount++
				}
			}

			return map[string]any{
				"total":   len(tasks),
				"success": successCount,
				"failed":  len(tasks) - successCount,
				"results": results,
			}, nil
		},
	}
}

func parseTeacherSearchInput(input map[string]any) TeacherSearchInput {
	in := TeacherSearchInput{}
	if name, ok := input["name"].(string); ok {
		in.Name = name
	}
	if pinyin, ok := input["pinyin"].(string); ok {
		in.Pinyin = pinyin
	}
	return in
}

func extractCrawlContent(result map[string]any) string {
	contentArr, ok := result["content"].([]any)
	if !ok || len(contentArr) == 0 {
		return ""
	}
	first, ok := contentArr[0].(map[string]any)
	if !ok {
		return ""
	}
	text, ok := first["text"].(string)
	if !ok {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return text
	}
	if md, ok := parsed["content"].(string); ok && md != "" {
		return md
	}
	if md, ok := parsed["fit_markdown"].(string); ok && md != "" {
		return md
	}
	return text
}

func extractCrawlTitle(result map[string]any) string {
	contentArr, ok := result["content"].([]any)
	if !ok || len(contentArr) == 0 {
		return ""
	}
	first, ok := contentArr[0].(map[string]any)
	if !ok {
		return ""
	}
	text, ok := first["text"].(string)
	if !ok {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return ""
	}
	title, _ := parsed["title"].(string)
	return title
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func convertToPinyin(name string) string {
	hasChinese := false
	for _, r := range name {
		if unicode.Is(unicode.Han, r) {
			hasChinese = true
			break
		}
	}
	if !hasChinese {
		return strings.ToLower(name)
	}

	opts := pinyin.NewArgs()
	opts.Style = pinyin.Normal
	opts.Separator = ""

	return pinyin.Slug(name, opts)
}
