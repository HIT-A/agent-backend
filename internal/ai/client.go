package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewClient() *Client {
	baseURL := os.Getenv("MINIMAX_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.minimaxi.com/v1"
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  os.Getenv("MINIMAX_API_KEY"),
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

type MiniMaxMessage struct {
	Role    string `json:"role"`
	Content string `json:"content,omitempty"`
	Name    string `json:"name,omitempty"`
}

type ChatRequest struct {
	Model     string           `json:"model"`
	Messages  []MiniMaxMessage `json:"messages"`
	MaxTokens int              `json:"max_tokens,omitempty"`
}

type ChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Index        int    `json:"index"`
		Message      struct {
			Content string `json:"content"`
			Role    string `json:"role"`
			Name    string `json:"name"`
		} `json:"message"`
	} `json:"choices"`
	Model string `json:"model"`
	Usage struct {
		TotalTokens      int `json:"total_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func (c *Client) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("MINIMAX_API_KEY not set")
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}

		resp, err := c.doChatRequest(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("after 3 attempts: %w", lastErr)
}

func (c *Client) doChatRequest(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/text/chatcompletion_v2", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	// MiniMax sometimes returns multiple JSON objects concatenated (e.g. error + base_resp)
	// Find the first valid JSON object
	firstObjEnd := 0
	braceCount := 0
	inString := false
	escape := false
	for i, b := range body {
		if escape {
			escape = false
			continue
		}
		if b == '\\' && inString {
			escape = true
			continue
		}
		if b == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if b == '{' {
			braceCount++
		} else if b == '}' {
			braceCount--
			if braceCount == 0 {
				firstObjEnd = i + 1
				break
			}
		}
	}

	jsonBody := body
	if firstObjEnd > 0 && firstObjEnd < len(body) {
		jsonBody = body[:firstObjEnd]
	}

	var result ChatResponse
	if err := json.Unmarshal(jsonBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if result.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("MiniMax error: %s (code: %d)", result.BaseResp.StatusMsg, result.BaseResp.StatusCode)
	}

	if len(result.Choices) == 0 && firstObjEnd > 0 && firstObjEnd < len(body) {
		remaining := bytes.TrimSpace(body[firstObjEnd:])
		if len(remaining) > 0 {
			var result2 ChatResponse
			if json.Unmarshal(remaining, &result2) == nil && len(result2.Choices) > 0 {
				if result2.BaseResp.StatusCode != 0 {
					return nil, fmt.Errorf("MiniMax error: %s (code: %d)", result2.BaseResp.StatusMsg, result2.BaseResp.StatusCode)
				}
				return &result2, nil
			}
		}
	}

	return &result, nil
}

func (c *Client) SimpleChat(ctx context.Context, system, userMessage string) (string, error) {
	messages := []MiniMaxMessage{}
	if system != "" {
		messages = append(messages, MiniMaxMessage{Role: "system", Content: system, Name: "System"})
	}
	messages = append(messages, MiniMaxMessage{Role: "user", Content: userMessage, Name: "用户"})

	req := &ChatRequest{
		Model:     "MiniMax-M2.7",
		Messages:  messages,
		MaxTokens: 4096,
	}

	resp, err := c.Chat(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response from AI")
}

type ReActStep struct {
	Thought     string `json:"thought"`
	Action      string `json:"action"`
	ActionInput string `json:"action_input"`
	Observation string `json:"observation,omitempty"`
}

type ReActResponse struct {
	Steps    []ReActStep `json:"steps"`
	Answer   string      `json:"answer"`
	Complete bool        `json:"complete"`
}

type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Execute     func(ctx context.Context, input string) (string, error)
}

func (c *Client) ReActChat(ctx context.Context, userMessage string, tools []Tool) (*ReActResponse, error) {
	hasTools := len(tools) > 0
	if !hasTools {
		answer, err := c.SimpleChat(ctx, "", userMessage)
		if err != nil {
			return nil, err
		}
		return &ReActResponse{
			Steps:    []ReActStep{},
			Answer:   answer,
			Complete: true,
		}, nil
	}

	maxSteps := 5
	steps := []ReActStep{}

	systemPrompt := `你是一个智能助手，必须使用以下 ReAct 格式响应：

**格式规则（严格遵守）：**
1. 每次回复必须包含"思考"标签
2. 如果需要使用工具，必须包含"动作"和"动作输入"标签
3. 如果已完成任务，使用"答案"标签

**标签格式：**
思考：[你的推理过程，解释你在想什么]
动作：[工具名称，如 get_timetable/search_course/search_teacher]
动作输入：[JSON格式参数，如 {"date": "2024-01-15"}]

或

思考：[你的推理]
答案：[给用户的最终回复]

**可用工具：**` + formatTools(tools) + `

**重要规则：**
- "动作"必须是工具名称或"答案"二选一
- "动作输入"必须是有效的JSON格式
- 不要输出"观察"部分，系统会自动提供
- 每次只能执行一个动作

**示例：**
用户：明天有什么课？
思考：用户想查询明天的课程表，我需要使用 get_timetable 工具
动作：get_timetable
动作输入：{"date": "2024-01-16"}

然后等待系统提供观察结果，再继续下一步。`

	conversation := []MiniMaxMessage{
		{Role: "system", Content: systemPrompt, Name: "System"},
		{Role: "user", Content: userMessage, Name: "user"},
	}

	for step := 0; step < maxSteps; step++ {
		req := &ChatRequest{
			Model:     "MiniMax-M2.7",
			Messages:  conversation,
			MaxTokens: 4096,
		}

		var resp *ChatResponse
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			resp, err = c.Chat(ctx, req)
			if err != nil {
				continue
			}
			if len(resp.Choices) > 0 {
				break
			}
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}

		if err != nil {
			return nil, fmt.Errorf("step %d chat error: %w", step, err)
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no response at step %d after 3 attempts", step)
		}

		aiResponse := resp.Choices[0].Message.Content
		parsed := parseReActStep(aiResponse)

		steps = append(steps, ReActStep{
			Thought:     parsed.Thought,
			Action:      parsed.Action,
			ActionInput: parsed.ActionInput,
		})

		if parsed.Action == "答案" || parsed.Action == "answer" || parsed.Action == "" {
			return &ReActResponse{
				Steps:    steps,
				Answer:   parsed.Thought,
				Complete: true,
			}, nil
		}

		toolResult, err := executeTool(ctx, parsed.Action, parsed.ActionInput, tools)
		if err != nil {
			toolResult = fmt.Sprintf("错误: %v", err)
		}

		steps[len(steps)-1].Observation = toolResult

		conversation = append(conversation,
			MiniMaxMessage{Role: "assistant", Content: aiResponse, Name: "assistant"},
			MiniMaxMessage{Role: "user", Content: fmt.Sprintf("观察结果: %s", toolResult), Name: "user"},
		)
	}

	return &ReActResponse{
		Steps:    steps,
		Answer:   "达到最大步骤限制，请简化您的问题",
		Complete: false,
	}, nil
}

type parsedStep struct {
	Thought     string
	Action      string
	ActionInput string
}

func parseReActStep(text string) parsedStep {
	result := parsedStep{}

	thoughtRegex := regexp.MustCompile(`(?is)思考[：:]\s*(.+?)(?=\n动作[：:]|\n答案[：:]|$)`)
	actionRegex := regexp.MustCompile(`(?i)动作[：:]\s*(\S+)`)
	actionInputRegex := regexp.MustCompile(`(?i)动作输入[：:]\s*(.+?)(?:\n|$)`)
	answerRegex := regexp.MustCompile(`(?is)答案[：:]\s*(.+)`)

	if match := thoughtRegex.FindStringSubmatch(text); len(match) > 1 {
		result.Thought = strings.TrimSpace(match[1])
	}

	if match := actionRegex.FindStringSubmatch(text); len(match) > 1 {
		result.Action = strings.TrimSpace(match[1])
	}

	if match := actionInputRegex.FindStringSubmatch(text); len(match) > 1 {
		result.ActionInput = strings.TrimSpace(match[1])
	}

	if result.Action == "" {
		if match := answerRegex.FindStringSubmatch(text); len(match) > 1 {
			result.Action = "答案"
			result.Thought = strings.TrimSpace(match[1])
		}
	}

	if result.Action == "" {
		result.Action = "答案"
		result.Thought = strings.TrimSpace(text)
	}

	return result
}

func executeTool(ctx context.Context, action, input string, tools []Tool) (string, error) {
	for _, tool := range tools {
		if tool.Name == action {
			return tool.Execute(ctx, input)
		}
	}
	return "", fmt.Errorf("未知工具: %s", action)
}

func formatTools(tools []Tool) string {
	if len(tools) == 0 {
		return "无可用工具"
	}
	var result string
	for _, tool := range tools {
		result += fmt.Sprintf("\n- %s: %s", tool.Name, tool.Description)
	}
	return result
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
