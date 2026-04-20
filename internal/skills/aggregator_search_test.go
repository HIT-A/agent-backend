package skills

import (
	"testing"
)

func TestParseBraveToolResult_NodeContentEntries(t *testing.T) {
	toolResult := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": `{"url":"https://openai.com","title":"OpenAI","description":"AI research lab"}`},
			map[string]any{"type": "text", "text": `{"url":"https://openai.com/blog","title":"OpenAI Blog","description":"News and updates"}`},
		},
		"isError": false,
	}

	results, err := parseBraveToolResult(toolResult)
	if err != nil {
		t.Fatalf("parseBraveToolResult returned error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "OpenAI" || results[1].Title != "OpenAI Blog" {
		t.Fatalf("unexpected parsed titles: %#v", results)
	}
}

func TestParseBraveToolResult_PythonPayload(t *testing.T) {
	toolResult := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": `{"results":[{"url":"https://www.hitsz.edu.cn","title":"HITSZ","description":"Campus site"}]}`},
		},
		"isError": false,
	}

	results, err := parseBraveToolResult(toolResult)
	if err != nil {
		t.Fatalf("parseBraveToolResult returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].URL != "https://www.hitsz.edu.cn" {
		t.Fatalf("unexpected parsed url: %#v", results[0])
	}
}

func TestParseBraveToolResult_NoWebResultsError(t *testing.T) {
	toolResult := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": "No web results found"},
		},
		"isError": true,
	}

	results, err := parseBraveToolResult(toolResult)
	if err != nil {
		t.Fatalf("expected no error for explicit empty result, got: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestParseBraveAnswerToolResult_JSONPayload(t *testing.T) {
	toolResult := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": `{"answer":"OpenAI is an AI company.","model":"brave","usage":{"X-Request-Total-Cost":"0.01"}}`},
		},
		"isError": false,
	}

	result, err := parseBraveAnswerToolResult(toolResult)
	if err != nil {
		t.Fatalf("parseBraveAnswerToolResult returned error: %v", err)
	}
	if result.Answer != "OpenAI is an AI company." {
		t.Fatalf("unexpected answer: %#v", result)
	}
	if result.Model != "brave" {
		t.Fatalf("unexpected model: %#v", result)
	}
}

func TestParseBraveAnswerToolResult_ErrorPayload(t *testing.T) {
	toolResult := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": `{"error":"Missing BRAVE_ANSWER_API_KEY"}`},
		},
		"isError": false,
	}

	_, err := parseBraveAnswerToolResult(toolResult)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "brave-answer payload error: Missing BRAVE_ANSWER_API_KEY" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseBraveAnswerToolResult_MCPError(t *testing.T) {
	toolResult := map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": "Brave Answers API request failed"},
		},
		"isError": true,
	}

	_, err := parseBraveAnswerToolResult(toolResult)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "brave-answer MCP error: Brave Answers API request failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}
