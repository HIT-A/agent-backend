package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CourseReadInput 读取课程输入
type CourseReadInput struct {
	Campus     string `json:"campus"` // shenzhen/weihai/harbin
	CourseCode string `json:"course_code"`
	Repo       string `json:"repo,omitempty"`
	Org        string `json:"org,omitempty"`
	IncludeTOML bool   `json:"include_toml,omitempty"`
}

// CourseSearchInput 搜索课程输入
type CourseSearchInput struct {
	Keyword string `json:"keyword"`
	Campus  string `json:"campus"` // shenzhen/weihai/harbin 空为全部
	Limit   int    `json:"limit"`
}

// CourseReadSkill 读取课程 README
func NewCourseReadSkill() Skill {
	return Skill{
		Name:    "course.read",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			in := parseCourseReadInput(input)

			if in.Campus == "" {
				in.Campus = "shenzhen"
			}
			if in.CourseCode == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "course_code is required", Retryable: false}
			}

			// 调用 pr-server
			prServerURL := getPRServerBaseURL()
			if prServerURL == "" {
				return nil, &InvokeError{Code: "CONFIG", Message: "pr_server_url not configured", Retryable: false}
			}

			reqBody := map[string]any{
				"target": map[string]any{
					"campus":      in.Campus,
					"course_code": in.CourseCode,
				},
			}
			if includeTOML, ok := input["include_toml"].(bool); ok {
				reqBody["include_toml"] = includeTOML
			}

			reqJSON, _ := json.Marshal(reqBody)
			req, err := http.NewRequestWithContext(ctx, "POST", prServerURL+"/v1/course:read", bytes.NewBuffer(reqJSON))
			if err != nil {
				return nil, &InvokeError{Code: "NETWORK", Message: fmt.Sprintf("create request: %v", err), Retryable: true}
			}

			req.Header.Set("Content-Type", "application/json")
			prServerToken := getPRServerToken()
			if prServerToken != "" {
				req.Header.Set("Authorization", "Bearer "+prServerToken)
			}

			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return nil, &InvokeError{Code: "NETWORK", Message: fmt.Sprintf("call pr-server: %v", err), Retryable: true}
			}
			defer resp.Body.Close()

			var result map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("decode response: %v", err), Retryable: false}
			}

			if resp.StatusCode != 200 {
				return nil, &InvokeError{Code: "PR_SERVER_ERROR", Message: fmt.Sprintf("pr-server returned %d", resp.StatusCode), Retryable: true}
			}

			return map[string]any{
				"ok":     true,
				"output": result,
			}, nil
		},
	}
}

// CoursesSearchSkill 搜索课程
func NewCoursesSearchSkill() Skill {
	return Skill{
		Name:    "courses.search",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			in := parseCourseSearchInput(input)

			if in.Keyword == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "keyword is required", Retryable: false}
			}
			if in.Limit <= 0 {
				in.Limit = 10
			}

			prServerURL := getPRServerBaseURL()
			if prServerURL == "" {
				return nil, &InvokeError{Code: "CONFIG", Message: "pr_server_url not configured", Retryable: false}
			}

			reqBody := map[string]any{
				"keyword": in.Keyword,
				"limit":   in.Limit,
			}
			if in.Campus != "" {
				reqBody["campus"] = in.Campus
			}

			reqJSON, _ := json.Marshal(reqBody)
			req, err := http.NewRequestWithContext(ctx, "POST", prServerURL+"/v1/courses:search", bytes.NewBuffer(reqJSON))
			if err != nil {
				return nil, &InvokeError{Code: "NETWORK", Message: fmt.Sprintf("create request: %v", err), Retryable: true}
			}

			req.Header.Set("Content-Type", "application/json")
			prServerToken := getPRServerToken()
			if prServerToken != "" {
				req.Header.Set("Authorization", "Bearer "+prServerToken)
			}

			client := &http.Client{Timeout: 30 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				return nil, &InvokeError{Code: "NETWORK", Message: fmt.Sprintf("call pr-server: %v", err), Retryable: true}
			}
			defer resp.Body.Close()

			var result map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("decode response: %v", err), Retryable: false}
			}

			if resp.StatusCode != 200 {
				return nil, &InvokeError{Code: "PR_SERVER_ERROR", Message: fmt.Sprintf("pr-server returned %d", resp.StatusCode), Retryable: true}
			}

			return map[string]any{
				"ok":     true,
				"output": result,
			}, nil
		},
	}
}

func parseCourseReadInput(input map[string]any) CourseReadInput {
	in := CourseReadInput{
		Campus: "shenzhen",
	}

	if campus, ok := input["campus"].(string); ok {
		in.Campus = campus
	}
	if courseCode, ok := input["course_code"].(string); ok {
		in.CourseCode = courseCode
	}
	if repo, ok := input["repo"].(string); ok {
		in.Repo = repo
	}
	if org, ok := input["org"].(string); ok {
		in.Org = org
	}
	if includeTOML, ok := input["include_toml"].(bool); ok {
		in.IncludeTOML = includeTOML
	}

	return in
}

func parseCourseSearchInput(input map[string]any) CourseSearchInput {
	in := CourseSearchInput{
		Limit: 10,
	}

	if keyword, ok := input["keyword"].(string); ok {
		in.Keyword = keyword
	}
	if campus, ok := input["campus"].(string); ok {
		in.Campus = campus
	}
	if limit, ok := input["limit"].(float64); ok {
		in.Limit = int(limit)
	}

	return in
}
