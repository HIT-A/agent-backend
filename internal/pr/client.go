package pr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Client is a minimal HTTP client for pr-server.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CoursePreviewRequest is the request body for /v1/course:preview.
type CoursePreviewRequest struct {
	Target struct {
		Campus     string `json:"campus"`
		CourseCode string `json:"course_code"`
	} `json:"target"`
	Ops []json.RawMessage `json:"ops"`
}

// CoursePreviewResponse is the response from /v1/course:preview.
type CoursePreviewResponse struct {
	Ok    bool `json:"ok"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Data *struct {
		Base struct {
			Org      string `json:"org"`
			Repo     string `json:"repo"`
			Ref      string `json:"ref"`
			TomlPath string `json:"toml_path"`
		} `json:"base"`
		Result struct {
			ReadmeTOML string `json:"readme_toml"`
			ReadmeMD   string `json:"readme_md"`
		} `json:"result"`
	} `json:"data,omitempty"`
}

// CoursePreview calls POST /v1/course:preview.
func (c *Client) CoursePreview(ctx context.Context, req *CoursePreviewRequest) (*CoursePreviewResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/course:preview", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	var result CoursePreviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// PRLookupRequest is the request for /v1/pr:lookup.
type PRLookupRequest struct {
	Org    string `json:"org"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

// PRLookupResponse is the response from /v1/pr:lookup.
type PRLookupResponse struct {
	Ok    bool `json:"ok"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Data *struct {
		State  string `json:"state"`
		URL    string `json:"url"`
		Merged bool   `json:"merged"`
		Checks *struct {
			Status     string  `json:"status"`
			Conclusion *string `json:"conclusion,omitempty"`
		} `json:"checks,omitempty"`
	} `json:"data,omitempty"`
}

// PRLookup calls GET /v1/pr:lookup?org=...&repo=...&number=...
func (c *Client) PRLookup(ctx context.Context, req *PRLookupRequest) (*PRLookupResponse, error) {
	// Build query parameters
	endpoint := fmt.Sprintf("%s/v1/pr:lookup?org=%s&repo=%s&number=%d",
		c.BaseURL,
		url.QueryEscape(req.Org),
		url.QueryEscape(req.Repo),
		req.Number)

	httpReq, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	var result PRLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// CourseSubmitRequest is the request body for /v1/course:submit.
type CourseSubmitRequest struct {
	Target struct {
		Campus     string `json:"campus"`
		CourseCode string `json:"course_code"`
	} `json:"target"`
	Ops            []json.RawMessage `json:"ops"`
	IdempotencyKey string            `json:"idempotency_key"`
}

// CourseSubmitResponse is the response from /v1/course:submit.
type CourseSubmitResponse struct {
	Ok    bool `json:"ok"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Data *struct {
		Base struct {
			Org      string `json:"org"`
			CacheOrg string `json:"cache_org"`
			Repo     string `json:"repo"`
			Ref      string `json:"ref"`
			TomlPath string `json:"toml_path"`
		} `json:"base"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
		PR struct {
			URL        string `json:"url"`
			Number     int    `json:"number"`
			HeadBranch string `json:"head_branch"`
			BaseBranch string `json:"base_branch"`
		} `json:"pr"`
	} `json:"data,omitempty"`
}

// CourseSubmit calls POST /v1/course:submit.
func (c *Client) CourseSubmit(ctx context.Context, req *CourseSubmitRequest) (*CourseSubmitResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/course:submit", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	var result CourseSubmitResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
