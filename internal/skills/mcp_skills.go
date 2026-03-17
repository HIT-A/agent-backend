package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"hoa-agent-backend/internal/mcp"
)

var transitURLRegexp = regexp.MustCompile(`https?://[^\s"'<>]+`)
var localPathRegexp = regexp.MustCompile(`(?i)path:\s*([^\n\r]+)`)

// NewMCPListServersSkill returns a skill that lists all registered MCP servers.
func NewMCPListServersSkill(registry *mcp.Registry) Skill {
	return Skill{
		Name:    "mcp.list_servers",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace
			_ = input

			servers := registry.List()

			result := make([]map[string]any, len(servers))
			for i, server := range servers {
				result[i] = map[string]any{
					"name":        server.Config.Name,
					"transport":   server.Config.Transport,
					"url":         server.Config.URL,
					"command":     server.Config.Command,
					"enabled":     server.Config.Enabled,
					"initialized": server.Initialized,
					"tools":       server.Tools,
					"resources":   server.Resources,
				}
			}

			return map[string]any{
				"servers": result,
			}, nil
		},
	}
}

// NewMCPCallToolSkill returns a skill that calls an MCP tool.
// Now async to support concurrent tool execution.
func NewMCPCallToolSkill(registry *mcp.Registry) Skill {
	return Skill{
		Name:    "mcp.call_tool",
		IsAsync: true, // Changed to async for better concurrency with external tools
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Extract required fields
			serverName, ok := input["server"].(string)
			if !ok || strings.TrimSpace(serverName) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "server is required", Retryable: false}
			}

			toolName, ok := input["tool"].(string)
			if !ok || strings.TrimSpace(toolName) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "tool is required", Retryable: false}
			}

			// Get tool arguments
			var arguments map[string]any
			if args, ok := input["arguments"].(map[string]any); ok {
				arguments = args
			} else {
				arguments = make(map[string]any)
			}

			// Get the server
			server, exists := registry.Get(serverName)
			if !exists {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: fmt.Sprintf("MCP server '%s' not found", serverName), Retryable: false}
			}

			if !server.Initialized {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("MCP server '%s' not initialized", serverName), Retryable: false}
			}

			// Find the tool
			var tool *mcp.Tool
			for _, t := range server.Tools {
				if t.Name == toolName {
					tool = &t
					break
				}
			}

			if tool == nil {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: fmt.Sprintf("tool '%s' not found in server '%s'", toolName, serverName), Retryable: false}
			}

			// Get MCP client (we need to store clients in registry for this to work)
			// For now, we'll create a temporary client
			var mcpTransport mcp.Transport
			if server.Config.Transport == "http" {
				mcpTransport = mcp.NewHTTPTransport(server.Config.URL)
			} else if server.Config.Transport == "stdio" {
				mcpTransport = mcp.NewStdioTransport(server.Config.Command)
			}

			client := mcp.NewClient(mcpTransport)
			if err := client.Initialize(ctx); err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("failed to initialize MCP client: %v", err), Retryable: true}
			}
			defer client.Close()

			// Call the tool
			result, err := client.CallTool(ctx, toolName, arguments)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("failed to call tool '%s': %v", toolName, err), Retryable: true}
			}

			output := map[string]any{
				"server": serverName,
				"tool":   toolName,
				"result": result,
			}
			if intakeMeta := maybeQueueTransitOutput(ctx, serverName, toolName, arguments, result); intakeMeta != nil {
				output["intake"] = intakeMeta
			}
			return output, nil
		},
	}
}

func maybeQueueTransitOutput(ctx context.Context, serverName, toolName string, arguments map[string]any, result *mcp.ToolCallResult) map[string]any {
	server := strings.ToLower(strings.TrimSpace(serverName))
	tool := strings.ToLower(strings.TrimSpace(toolName))

	if server == "arxiv" {
		return map[string]any{
			"mode":      "transit_only",
			"persisted": false,
			"reason":    "arxiv is configured as transit-only",
		}
	}

	if server != "annas-archive" || !strings.Contains(tool, "download") {
		return nil
	}

	localFilePath := firstLocalPath(result)
	if localFilePath != "" {
		resolvedPath, resolveErr := resolveDownloadedPath(localFilePath, arguments)
		if resolveErr != nil {
			return map[string]any{
				"mode":        "transit_with_intake",
				"persisted":   false,
				"source_path": localFilePath,
				"reason":      resolveErr.Error(),
			}
		}
		localFilePath = resolvedPath

		b, err := os.ReadFile(localFilePath)
		if err != nil {
			return map[string]any{
				"mode":        "transit_with_intake",
				"persisted":   false,
				"source_path": localFilePath,
				"reason":      "read local download failed: " + err.Error(),
			}
		}
		if len(b) > 25*1024*1024 {
			return map[string]any{
				"mode":        "transit_with_intake",
				"persisted":   false,
				"source_path": localFilePath,
				"reason":      "file too large for intake (>25MB)",
			}
		}

		filename := path.Base(localFilePath)
		if strings.TrimSpace(filename) == "" {
			filename = "annas-download.bin"
		}
		queuedPath, qErr := enqueueRawToGitHub(ctx, "HIT-A/HITA_RagData", "main", defaultSkillRawDir, "annas", filename, b)
		if qErr != nil {
			return map[string]any{
				"mode":        "transit_with_intake",
				"persisted":   false,
				"source_path": localFilePath,
				"reason":      qErr.Error(),
			}
		}

		_ = os.Remove(localFilePath)
		return map[string]any{
			"mode":        "transit_with_intake",
			"persisted":   true,
			"source_path": localFilePath,
			"size":        len(b),
			"path":        queuedPath,
		}
	}

	downloadURL := firstTransitURL(result)
	if downloadURL == "" {
		return map[string]any{
			"mode":      "transit_with_intake",
			"persisted": false,
			"reason":    "no download url found in annas result",
		}
	}

	b, err := downloadFromURL(ctx, downloadURL)
	if err != nil {
		return map[string]any{
			"mode":       "transit_with_intake",
			"persisted":  false,
			"source_url": downloadURL,
			"reason":     "download failed: " + err.Error(),
		}
	}
	if len(b) > 25*1024*1024 {
		return map[string]any{
			"mode":       "transit_with_intake",
			"persisted":  false,
			"source_url": downloadURL,
			"reason":     "file too large for intake (>25MB)",
		}
	}

	filename := guessTransitFilename(arguments, downloadURL)
	queuedPath, qErr := enqueueRawToGitHub(ctx, "HIT-A/HITA_RagData", "main", defaultSkillRawDir, "annas", filename, b)
	if qErr != nil {
		return map[string]any{
			"mode":       "transit_with_intake",
			"persisted":  false,
			"source_url": downloadURL,
			"reason":     qErr.Error(),
		}
	}

	return map[string]any{
		"mode":       "transit_with_intake",
		"persisted":  true,
		"source_url": downloadURL,
		"size":       len(b),
		"path":       queuedPath,
	}
}

func firstLocalPath(result *mcp.ToolCallResult) string {
	if result == nil {
		return ""
	}
	for _, c := range result.Content {
		m := localPathRegexp.FindStringSubmatch(c.Text)
		if len(m) < 2 {
			continue
		}
		p := strings.TrimSpace(m[1])
		p = strings.Trim(p, "\"")
		if strings.HasPrefix(p, "/") {
			return p
		}
	}
	return ""
}

func firstTransitURL(result *mcp.ToolCallResult) string {
	if result == nil {
		return ""
	}
	for _, c := range result.Content {
		for _, m := range transitURLRegexp.FindAllString(c.Text, -1) {
			trimmed := strings.TrimSpace(m)
			if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
				return trimmed
			}
		}
	}
	return ""
}

func guessTransitFilename(arguments map[string]any, rawURL string) string {
	candidates := []string{"filename", "file_name", "name", "title"}
	for _, k := range candidates {
		if v, ok := arguments[k].(string); ok && strings.TrimSpace(v) != "" {
			name := strings.TrimSpace(v)
			if path.Ext(name) == "" {
				name += ".bin"
			}
			return name
		}
	}
	if u, err := url.Parse(strings.TrimSpace(rawURL)); err == nil {
		base := path.Base(strings.TrimSpace(u.Path))
		if base != "" && base != "." && base != "/" {
			return base
		}
	}
	return "annas-download.bin"
}

func resolveDownloadedPath(localPath string, arguments map[string]any) (string, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return "", fmt.Errorf("stat local download failed: %w", err)
	}
	if !info.IsDir() {
		return localPath, nil
	}

	name := guessTransitFilename(arguments, "")
	candidate := filepath.Join(localPath, name)
	if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
		return candidate, nil
	}

	entries, err := os.ReadDir(localPath)
	if err != nil {
		return "", fmt.Errorf("read local download dir failed: %w", err)
	}
	var (
		latestPath string
		latestTime time.Time
	)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if latestPath == "" || fi.ModTime().After(latestTime) {
			latestPath = filepath.Join(localPath, e.Name())
			latestTime = fi.ModTime()
		}
	}
	if latestPath == "" {
		return "", fmt.Errorf("no file found under download directory: %s", localPath)
	}
	return latestPath, nil
}

// NewMCPListToolsSkill returns a skill that lists all tools from all registered MCP servers.
func NewMCPListToolsSkill(registry *mcp.Registry) Skill {
	return Skill{
		Name:    "mcp.list_tools",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace
			_ = input

			allTools := registry.ListTools()

			result := make(map[string]any)
			for serverName, tools := range allTools {
				result[serverName] = tools
			}

			return map[string]any{
				"tools": result,
			}, nil
		},
	}
}

// NewMCPSkillsFromEnv creates MCP skills from environment configuration
func NewMCPSkillsFromEnv(registry *mcp.Registry) []Skill {
	var skills []Skill

	// Register servers from environment
	if serversJSON := os.Getenv("MCP_SERVERS"); serversJSON != "" {
		var configs []mcp.ServerConfig
		if err := json.Unmarshal([]byte(serversJSON), &configs); err == nil {
			for _, config := range configs {
				if config.Enabled {
					_, _ = registry.Register(context.Background(), &config)
				}
			}
		}
	}

	// Add MCP management skills
	skills = append(skills, NewMCPListServersSkill(registry))
	skills = append(skills, NewMCPCallToolSkill(registry))
	skills = append(skills, NewMCPListToolsSkill(registry))

	return skills
}
