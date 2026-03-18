package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"hoa-agent-backend/internal/pr"
)

// PRJobInput represents the input for async PR skills
type PRJobInput struct {
	Campus     string            `json:"campus"`
	CourseCode string            `json:"course_code"`
	Ops        []json.RawMessage `json:"ops"`
}

// PRPreviewJobInput represents the input for pr.preview async job
type PRPreviewJobInput struct {
	Campus     string            `json:"campus"`
	CourseCode string            `json:"course_code"`
	Ops        []json.RawMessage `json:"ops"`
}

// NewPRPreviewSkill registers pr.preview skill.
//
// This skill previews course material changes by calling pr-server /v1/course:preview.
// It applies TOML operations (e.g., add_lecturer_review) and renders the README.md.
// Now supports async execution for better concurrency.
func NewPRPreviewSkill() Skill {
	return Skill{
		Name:    "pr.preview",
		IsAsync: true, // Changed to async for better concurrency
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Extract required fields
			campus, ok := input["campus"].(string)
			if !ok || strings.TrimSpace(campus) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "campus is required", Retryable: false}
			}

			courseCode, ok := input["course_code"].(string)
			if !ok || strings.TrimSpace(courseCode) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "course_code is required", Retryable: false}
			}

			// Extract ops (optional)
			ops, err := extractOps(input)
			if err != nil {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "invalid ops: " + err.Error(), Retryable: false}
			}

			// Build request to pr-server
			client := pr.NewClient(getPRServerBaseURL())
			client.Token = getPRServerToken()
			req := &pr.CoursePreviewRequest{}
			req.Target.Campus = campus
			req.Target.CourseCode = courseCode
			req.Ops = ops

			// Call pr-server
			resp, err := client.CoursePreview(ctx, req)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server error: " + err.Error(), Retryable: true}
			}

			if !resp.Ok {
				if resp.Error != nil {
					return nil, &InvokeError{
						Code:      translatePRServerError(resp.Error.Code),
						Message:   resp.Error.Message,
						Retryable: isRetryable(resp.Error.Code),
					}
				}
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server returned error without details", Retryable: true}
			}

			if resp.Data == nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server returned ok=true but no data", Retryable: false}
			}

			// Return preview result
			return map[string]any{
				"base": map[string]any{
					"org":       resp.Data.Base.Org,
					"repo":      resp.Data.Base.Repo,
					"ref":       resp.Data.Base.Ref,
					"toml_path": resp.Data.Base.TomlPath,
				},
				"result": map[string]any{
					"readme_toml": resp.Data.Result.ReadmeTOML,
					"readme_md":   resp.Data.Result.ReadmeMD,
				},
				"summary": map[string]any{
					"changed_files": []string{"readme.toml", "README.md"},
					"warnings":      []string{},
				},
			}, nil
		},
	}
}

// NewPRSubmitSkill registers pr.submit skill.
//
// This skill submits course material changes to GitHub by creating a PR.
// It applies TOML operations and creates a PR via pr-server /v1/course:submit.
// Now supports async execution for better concurrency.
func NewPRSubmitSkill() Skill {
	return Skill{
		Name:    "pr.submit",
		IsAsync: true, // Changed to async for better concurrency
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Extract required fields
			campus, ok := input["campus"].(string)
			if !ok || strings.TrimSpace(campus) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "campus is required", Retryable: false}
			}

			courseCode, ok := input["course_code"].(string)
			if !ok || strings.TrimSpace(courseCode) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "course_code is required", Retryable: false}
			}

			// Extract ops (optional)
			ops, err := extractOps(input)
			if err != nil {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "invalid ops: " + err.Error(), Retryable: false}
			}

			// Build request to pr-server
			client := pr.NewClient(getPRServerBaseURL())
			client.Token = getPRServerToken()
			req := &pr.CourseSubmitRequest{}
			req.Target.Campus = campus
			req.Target.CourseCode = courseCode
			req.Ops = ops
			if idem, ok := input["idempotency_key"].(string); ok && strings.TrimSpace(idem) != "" {
				req.IdempotencyKey = strings.TrimSpace(idem)
			} else {
				req.IdempotencyKey = fmt.Sprintf("%s:%s:%d", strings.ToLower(campus), strings.ToUpper(courseCode), time.Now().UnixNano())
			}

			// Call pr-server
			resp, err := client.CourseSubmit(ctx, req)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server error: " + err.Error(), Retryable: true}
			}

			if !resp.Ok {
				if resp.Error != nil {
					return nil, &InvokeError{
						Code:      translatePRServerError(resp.Error.Code),
						Message:   resp.Error.Message,
						Retryable: isRetryable(resp.Error.Code),
					}
				}
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server returned error without details", Retryable: true}
			}

			if resp.Data == nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server returned ok=true but no data", Retryable: false}
			}

			// Return PR info
			return map[string]any{
				"pr_number": resp.Data.PR.Number,
				"pr_url":    resp.Data.PR.URL,
				"branch":    resp.Data.PR.HeadBranch,
			}, nil
		},
	}
}

// NewPRLookupSkill registers pr.lookup skill.
//
// This skill queries the status of a PR by calling pr-server /v1/pr:lookup.
// Keeps sync as it's a simple lookup operation.
func NewPRLookupSkill() Skill {
	return Skill{
		Name:    "pr.lookup",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Extract required fields
			org, ok := input["org"].(string)
			if !ok || strings.TrimSpace(org) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "org is required", Retryable: false}
			}

			repo, ok := input["repo"].(string)
			if !ok || strings.TrimSpace(repo) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "repo is required", Retryable: false}
			}

			prNum := 0
			if v, ok := input["number"].(float64); ok {
				prNum = int(v)
			} else if v, ok := input["number"].(int); ok {
				prNum = v
			} else if v, ok := input["pr"].(float64); ok {
				prNum = int(v)
			} else if v, ok := input["pr"].(int); ok {
				prNum = v
			}
			if prNum <= 0 {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "number (or pr) is required and must be positive", Retryable: false}
			}

			// Build request to pr-server
			client := pr.NewClient(getPRServerBaseURL())
			client.Token = getPRServerToken()
			req := &pr.PRLookupRequest{
				Org:    org,
				Repo:   repo,
				Number: prNum,
			}

			// Call pr-server
			resp, err := client.PRLookup(ctx, req)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server error: " + err.Error(), Retryable: true}
			}

			if !resp.Ok {
				if resp.Error != nil {
					return nil, &InvokeError{
						Code:      translatePRServerError(resp.Error.Code),
						Message:   resp.Error.Message,
						Retryable: isRetryable(resp.Error.Code),
					}
				}
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server returned error without details", Retryable: true}
			}

			if resp.Data == nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: "pr-server returned ok=true but no data", Retryable: false}
			}

			// Return PR info
			result := map[string]any{
				"number": prNum,
				"state":  resp.Data.State,
				"url":    resp.Data.URL,
				"merged": resp.Data.Merged,
			}
			if resp.Data.Checks != nil {
				result["checks"] = map[string]any{
					"status": resp.Data.Checks.Status,
				}
				if resp.Data.Checks.Conclusion != nil {
					result["checks"].(map[string]any)["conclusion"] = *resp.Data.Checks.Conclusion
				}
			}
			return result, nil
		},
	}
}

// getPRServerBaseURL returns the pr-server base URL from environment.
// Falls back to localhost:8080 if not set.
func getPRServerBaseURL() string {
	if url := os.Getenv("PR_SERVER_URL"); url != "" {
		return strings.TrimRight(url, "/")
	}
	return "http://localhost:8080"
}

// getPRServerToken returns the pr-server token from environment.
func getPRServerToken() string {
	return os.Getenv("PR_SERVER_TOKEN")
}

// translatePRServerError maps pr-server error codes to agent-backend error codes.
func translatePRServerError(code string) string {
	// Map pr-server error codes to agent-backend error codes
	// If the code is already in agent-backend format, return as-is
	mapping := map[string]string{
		"TOML_SCHEMA_ERROR": "INVALID_INPUT",
		"RENDER_FAILED":     "INTERNAL",
		"REPO_NOT_FOUND":    "NOT_FOUND",
		"TOML_NOT_FOUND":    "NOT_FOUND",
		"CONFIG_ERROR":      "INTERNAL",
		"BRANCH_NOT_FOUND":  "NOT_FOUND",
		"INVALID_JSON":      "INVALID_INPUT",
		"MISSING_TARGET":    "INVALID_INPUT",
		"INVALID_OPS":       "INVALID_INPUT",
	}

	if mapped, ok := mapping[code]; ok {
		return mapped
	}
	// Default: return the original code
	return code
}

// isRetryable determines if a pr-server error is retryable.
func isRetryable(code string) bool {
	retryableCodes := map[string]bool{
		"TOML_SCHEMA_ERROR": false,
		"RENDER_FAILED":     true,
		"REPO_NOT_FOUND":    false,
		"TOML_NOT_FOUND":    false,
		"CONFIG_ERROR":      false,
		"BRANCH_NOT_FOUND":  false,
		"INVALID_JSON":      false,
		"MISSING_TARGET":    false,
		"INVALID_OPS":       false,
	}

	if retryable, ok := retryableCodes[code]; ok {
		return retryable
	}
	// Default: assume retryable for unknown errors
	return true
}

func extractOps(input map[string]any) ([]json.RawMessage, error) {
	rawOps, exists := input["ops"]
	if !exists || rawOps == nil {
		return nil, nil
	}

	if direct, ok := rawOps.([]json.RawMessage); ok {
		return direct, nil
	}

	opsAny, ok := rawOps.([]any)
	if !ok {
		return nil, fmt.Errorf("ops must be an array")
	}

	ops := make([]json.RawMessage, 0, len(opsAny))
	for i, op := range opsAny {
		switch v := op.(type) {
		case []byte:
			ops = append(ops, json.RawMessage(v))
		case json.RawMessage:
			ops = append(ops, v)
		case map[string]any:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("ops[%d] marshal failed: %w", i, err)
			}
			ops = append(ops, json.RawMessage(b))
		default:
			b, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("ops[%d] unsupported type: %T", i, op)
			}
			ops = append(ops, json.RawMessage(b))
		}
	}

	return ops, nil
}
