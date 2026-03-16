package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// SummarizeRequest represents a summarization request
type SummarizeRequest struct {
	Query       string         `json:"query"`
	Results     []SearchResult `json:"results"`
	Style       string         `json:"style"`        // "concise", "detailed", "bullet_points", "qa"
	MaxLength   int            `json:"max_length"`   // Max length in tokens/chars
	Language    string         `json:"language"`     // "zh", "en", "auto"
	ContextHint string         `json:"context_hint"` // Additional context for the LLM
}

// SummarizeResponse represents the summarization response
type SummarizeResponse struct {
	Summary     string        `json:"summary"`
	KeyPoints   []string      `json:"key_points"`
	SourcesUsed []string      `json:"sources_used"`
	Confidence  float64       `json:"confidence"`
	TokenUsed   int           `json:"token_used"`
	Duration    time.Duration `json:"duration_ms"`
}

// NewAggregatorSummarizeSkill creates a skill for AI-powered search result summarization
func NewAggregatorSummarizeSkill() Skill {
	return Skill{
		Name:    "aggregator.summarize",
		IsAsync: true, // AI summarization can be time-consuming
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Parse request
			req, err := parseSummarizeRequest(input)
			if err != nil {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: err.Error(), Retryable: false}
			}

			// Execute summarization
			resp, err := executeSummarization(ctx, req)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("summarization failed: %v", err), Retryable: true}
			}

			return map[string]any{
				"summary":      resp.Summary,
				"key_points":   resp.KeyPoints,
				"sources_used": resp.SourcesUsed,
				"confidence":   resp.Confidence,
				"token_used":   resp.TokenUsed,
				"duration_ms":  resp.Duration.Milliseconds(),
			}, nil
		},
	}
}

// parseSummarizeRequest parses and validates the input
func parseSummarizeRequest(input map[string]any) (*SummarizeRequest, error) {
	req := &SummarizeRequest{
		Style:     "concise",
		MaxLength: 500,
		Language:  "auto",
	}

	// Parse query
	if query, ok := input["query"].(string); ok {
		req.Query = query
	}

	// Parse results
	if resultsRaw, ok := input["results"].([]any); ok {
		for _, r := range resultsRaw {
			if resultMap, ok := r.(map[string]any); ok {
				result := SearchResult{
					Source:  getString(resultMap, "source"),
					Title:   getString(resultMap, "title"),
					Content: getString(resultMap, "content"),
				}
				if score, ok := resultMap["score"].(float64); ok {
					result.Score = score
				}
				req.Results = append(req.Results, result)
			}
		}
	}

	if len(req.Results) == 0 {
		return nil, fmt.Errorf("results are required for summarization")
	}

	// Parse style
	if style, ok := input["style"].(string); ok {
		validStyles := map[string]bool{"concise": true, "detailed": true, "bullet_points": true, "qa": true}
		if validStyles[style] {
			req.Style = style
		}
	}

	// Parse max_length
	if maxLen, ok := input["max_length"].(float64); ok {
		req.MaxLength = int(maxLen)
	}

	// Parse language
	if lang, ok := input["language"].(string); ok {
		req.Language = lang
	}

	// Parse context hint
	if hint, ok := input["context_hint"].(string); ok {
		req.ContextHint = hint
	}

	return req, nil
}

// executeSummarization performs AI-powered summarization
func executeSummarization(ctx context.Context, req *SummarizeRequest) (*SummarizeResponse, error) {
	start := time.Now()

	// Build prompt for the LLM
	prompt := buildSummarizationPrompt(req)

	// Call BigModel API for summarization
	summary, keyPoints, err := callBigModelForSummary(ctx, prompt, req.MaxLength)
	if err != nil {
		return nil, err
	}

	// Calculate confidence based on result quality
	confidence := calculateConfidence(req.Results, summary)

	// Collect unique sources
	sourceMap := make(map[string]bool)
	for _, r := range req.Results {
		sourceMap[r.Source] = true
	}
	sources := make([]string, 0, len(sourceMap))
	for s := range sourceMap {
		sources = append(sources, s)
	}

	return &SummarizeResponse{
		Summary:     summary,
		KeyPoints:   keyPoints,
		SourcesUsed: sources,
		Confidence:  confidence,
		TokenUsed:   estimateTokenCount(prompt + summary),
		Duration:    time.Since(start),
	}, nil
}

// buildSummarizationPrompt builds the prompt for the LLM
func buildSummarizationPrompt(req *SummarizeRequest) string {
	var b strings.Builder

	// System instruction
	b.WriteString("You are an expert at summarizing search results. Your task is to create a clear, accurate summary based on the provided search results.\n\n")

	// Context hint if provided
	if req.ContextHint != "" {
		b.WriteString(fmt.Sprintf("Context: %s\n\n", req.ContextHint))
	}

	// Original query
	if req.Query != "" {
		b.WriteString(fmt.Sprintf("User Query: %s\n\n", req.Query))
	}

	// Style instruction
	switch req.Style {
	case "concise":
		b.WriteString("Style: Provide a concise summary in 2-3 sentences. Focus on the most important information.\n\n")
	case "detailed":
		b.WriteString("Style: Provide a detailed summary with comprehensive information. Include key details and explanations.\n\n")
	case "bullet_points":
		b.WriteString("Style: Present the information as bullet points. Each point should capture one key fact or insight.\n\n")
	case "qa":
		b.WriteString("Style: Answer the user's query directly based on the search results. Be factual and cite sources implicitly.\n\n")
	}

	// Language instruction
	if req.Language == "zh" || (req.Language == "auto" && isChineseText(req.Query)) {
		b.WriteString("Language: Respond in Chinese (中文).\n\n")
	} else {
		b.WriteString("Language: Respond in English.\n\n")
	}

	// Search results
	b.WriteString("Search Results:\n")
	for i, result := range req.Results {
		if i >= 10 { // Limit to top 10 results to fit context
			break
		}
		b.WriteString(fmt.Sprintf("\n[%d] Source: %s\n", i+1, result.Source))
		b.WriteString(fmt.Sprintf("Title: %s\n", result.Title))
		// Truncate content if too long
		content := result.Content
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		b.WriteString(fmt.Sprintf("Content: %s\n", content))
	}

	b.WriteString("\n\nBased on the above search results, provide a summary. ")
	b.WriteString("If using bullet_points style, start each point on a new line with '• '. ")
	b.WriteString("Make sure to synthesize information from multiple sources if relevant.")

	return b.String()
}

// callBigModelForSummary calls the BigModel API for text generation
func callBigModelForSummary(ctx context.Context, prompt string, maxLength int) (string, []string, error) {
	apiKey := os.Getenv("BIGMODEL_API_KEY")
	if apiKey == "" {
		// Fallback to simple extractive summary if no API key
		return fallbackSummarize(prompt), nil, nil
	}

	model := os.Getenv("BIGMODEL_SUMMARIZE_MODEL")
	if model == "" {
		model = "glm-4" // Default to GLM-4 for summarization
	}

	endpoint := "https://open.bigmodel.cn/api/paas/v4/chat/completions"

	// Build request body
	reqBody, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3, // Lower temperature for more focused summarization
		"max_tokens":  maxLength * 2,
	})
	if err != nil {
		return "", nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx2)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("API error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", nil, fmt.Errorf("no response from model")
	}

	content := result.Choices[0].Message.Content

	// Parse bullet points if present
	keyPoints := extractBulletPoints(content)

	return content, keyPoints, nil
}

// fallbackSummarize provides a simple extractive summary when AI is unavailable
func fallbackSummarize(prompt string) string {
	// Extract the search results section
	lines := strings.Split(prompt, "\n")
	var summaries []string
	inContent := false

	for _, line := range lines {
		if strings.HasPrefix(line, "Content: ") {
			inContent = true
			content := strings.TrimPrefix(line, "Content: ")
			if len(content) > 100 {
				summaries = append(summaries, content[:100]+"...")
			} else {
				summaries = append(summaries, content)
			}
		} else if inContent && line == "" {
			inContent = false
		}
	}

	if len(summaries) == 0 {
		return "No summary available (AI service not configured)."
	}

	return "Summary (extractive):\n" + strings.Join(summaries[:min(3, len(summaries))], "\n")
}

// extractBulletPoints extracts bullet points from the summary
func extractBulletPoints(text string) []string {
	var points []string
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for bullet points (•, -, *, or numbered)
		if strings.HasPrefix(line, "• ") || strings.HasPrefix(line, "- ") ||
			strings.HasPrefix(line, "* ") || (len(line) > 2 && line[0] >= '0' && line[0] <= '9' && line[1] == '.') {
			point := strings.TrimPrefix(line, "• ")
			point = strings.TrimPrefix(point, "- ")
			point = strings.TrimPrefix(point, "* ")
			point = strings.TrimSpace(point)
			if point != "" {
				points = append(points, point)
			}
		}
	}

	return points
}

// calculateConfidence calculates a confidence score based on result quality
func calculateConfidence(results []SearchResult, summary string) float64 {
	if len(results) == 0 || summary == "" {
		return 0.0
	}

	// Base confidence on number of sources
	confidence := 0.5

	// Boost for multiple sources
	if len(results) >= 3 {
		confidence += 0.2
	}

	// Boost for high-scoring results
	avgScore := 0.0
	for _, r := range results {
		avgScore += r.Score
	}
	avgScore /= float64(len(results))
	confidence += avgScore * 0.3

	// Cap at 0.95
	if confidence > 0.95 {
		confidence = 0.95
	}

	return confidence
}

// estimateTokenCount estimates the number of tokens in text
func estimateTokenCount(text string) int {
	// Rough estimation: 1 token ≈ 4 characters for Chinese, ≈ 4 characters for English
	return len(text) / 4
}

// isChineseText checks if text contains Chinese characters
func isChineseText(text string) bool {
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return true
		}
	}
	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
