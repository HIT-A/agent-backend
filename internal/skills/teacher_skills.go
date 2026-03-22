package skills

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/mozillazg/go-pinyin"

	"hoa-agent-backend/internal/mcp"
)

// TeacherSearchInput represents input for searching a teacher
type TeacherSearchInput struct {
	Name       string `json:"name"`         // 教师姓名（中文或拼音）
	Pinyin     string `json:"pinyin"`       // 拼音（可选）
	Department string `json:"department"`   // 院系（可选）
	SaveToFile bool   `json:"save_to_file"` // 是否保存到文件
	OutputPath string `json:"output_path"`  // 输出路径
	AutoIngest bool   `json:"auto_ingest"`  // 是否自动接入 RAG
}

// TeacherInfo represents teacher information
type TeacherInfo struct {
	Name       string `json:"name"`
	Pinyin     string `json:"pinyin"`
	Title      string `json:"title"`
	Department string `json:"department"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Office     string `json:"office"`
	Research   string `json:"research"`
	Homepage   string `json:"homepage"`
	Profile    string `json:"profile"`
}

// NewTeacherSearchSkill creates a skill for searching HIT teachers
func NewTeacherSearchSkill(mcpRegistry *mcp.Registry) Skill {
	return Skill{
		Name:    "hit.teacher",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Parse input
			in := parseTeacherSearchInput(input)

			// Validate input
			if in.Name == "" && in.Pinyin == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "name or pinyin is required", Retryable: false}
			}

			// Get MCP server
			server, exists := mcpRegistry.Get("crawl4ai")
			if !exists {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: "crawl4ai MCP server not registered", Retryable: false}
			}

			if !server.Initialized {
				return nil, &InvokeError{Code: "INTERNAL", Message: "crawl4ai MCP server not initialized", Retryable: true}
			}

			// Build teacher URL
			pinyin := in.Pinyin
			if pinyin == "" && in.Name != "" {
				pinyin = convertToPinyin(in.Name)
			}

			teacherURL := fmt.Sprintf("https://homepage.hit.edu.cn/%s?lang=zh", pinyin)

			// Build arguments
			args := map[string]any{
				"url":            teacherURL,
				"content_filter": true,
				"output_format":  "markdown",
			}

			// Call MCP tool
			result, err := callMCPTool(ctx, server, "crawl_page", args)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("failed to crawl teacher page: %v", err), Retryable: true}
			}

			// Parse teacher info from result
			teacherInfo := parseTeacherInfo(result, pinyin, teacherURL)

			// Build response
			response := map[string]any{
				"success":     true,
				"teacher":     teacherInfo,
				"homepage":    teacherURL,
				"pinyin":      pinyin,
				"raw_content": result,
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
			names, _ := input["names"].([]string)
			pinyins, _ := input["pinyins"].([]string)
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

			for i, t := range tasks {
				semaphore <- struct{}{}
				go func(idx int, pinyin, name string) {
					defer func() { <-semaphore }()

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

					teacherInfo := parseTeacherInfo(result, pinyin, teacherURL)
					results[idx] = map[string]any{
						"success":     true,
						"teacher":     teacherInfo,
						"homepage":    teacherURL,
						"pinyin":      pinyin,
						"raw_content": result,
					}
				}(i, t.pinyin, t.name)
			}

			// Wait for all tasks
			time.Sleep(2 * time.Second)

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
	in := TeacherSearchInput{
		SaveToFile: false,
		AutoIngest: false,
	}

	if name, ok := input["name"].(string); ok {
		in.Name = name
	}
	if pinyin, ok := input["pinyin"].(string); ok {
		in.Pinyin = pinyin
	}
	if department, ok := input["department"].(string); ok {
		in.Department = department
	}
	if saveToFile, ok := input["save_to_file"].(bool); ok {
		in.SaveToFile = saveToFile
	}
	if outputPath, ok := input["output_path"].(string); ok {
		in.OutputPath = outputPath
	}
	if autoIngest, ok := input["auto_ingest"].(bool); ok {
		in.AutoIngest = autoIngest
	}

	return in
}

func parseTeacherInfo(result map[string]any, pinyin, homepage string) TeacherInfo {
	info := TeacherInfo{
		Pinyin:   pinyin,
		Homepage: homepage,
	}

	// Extract content from result
	if content, ok := result["content"].(string); ok {
		info.Profile = content

		// Try to extract common fields
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "@") {
				info.Email = extractEmail(line)
			}
			if strings.Contains(line, "电话") || strings.Contains(line, "Tel") || strings.Contains(line, "Phone") {
				info.Phone = extractPhone(line)
			}
			if strings.Contains(line, "办公室") || strings.Contains(line, "Office") {
				info.Office = extractOffice(line)
			}
			if strings.Contains(line, "职称") || strings.Contains(line, "Title") || strings.Contains(line, "教授") || strings.Contains(line, "副教授") || strings.Contains(line, "讲师") {
				info.Title = strings.TrimSpace(strings.Split(line, ":")[0])
			}
			if strings.Contains(line, "研究方向") || strings.Contains(line, "Research") || strings.Contains(line, "研究领域") {
				info.Research = line
			}
		}
	}

	return info
}

func extractEmail(text string) string {
	for _, word := range strings.Fields(text) {
		if strings.Contains(word, "@") && strings.Contains(word, ".") {
			return word
		}
	}
	return ""
}

func extractPhone(text string) string {
	fields := strings.Fields(text)
	for _, f := range fields {
		if len(f) >= 7 && strings.ContainsAny(f, "0123456789-") {
			return f
		}
	}
	return ""
}

func extractOffice(text string) string {
	parts := strings.Split(text, ":")
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// convertToPinyin converts Chinese characters to pinyin
// Uses mozillazg/go-pinyin library for accurate conversion
func convertToPinyin(name string) string {
	// If no Chinese characters, return lowercase
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

	// Convert Chinese to pinyin using Slug format (e.g., "zhang-san")
	opts := pinyin.NewArgs()
	opts.Style = pinyin.Normal
	opts.Separator = "-"

	return pinyin.Slug(name, opts)
}
