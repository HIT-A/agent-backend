package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"hoa-agent-backend/internal/mcp"
)

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

// NewMCPRegisterServerSkill returns a skill that registers a new MCP server.
func NewMCPRegisterServerSkill(registry *mcp.Registry) Skill {
	return Skill{
		Name:    "mcp.register_server",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Extract required fields
			name, ok := input["name"].(string)
			if !ok || strings.TrimSpace(name) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "name is required", Retryable: false}
			}

			transport, ok := input["transport"].(string)
			if !ok || strings.TrimSpace(transport) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "transport is required (http or stdio)", Retryable: false}
			}

			// Build server config
			config := &mcp.ServerConfig{
				Name:    name,
				Enabled: true,
			}

			// Transport-specific config
			if transport == "http" {
				url, ok := input["url"].(string)
				if !ok || strings.TrimSpace(url) == "" {
					return nil, &InvokeError{Code: "INVALID_INPUT", Message: "url is required for http transport", Retryable: false}
				}
				config.Transport = "http"
				config.URL = url
			} else if transport == "stdio" {
				commandRaw, ok := input["command"]
				if !ok {
					return nil, &InvokeError{Code: "INVALID_INPUT", Message: "command is required for stdio transport", Retryable: false}
				}

				var command []string
				switch v := commandRaw.(type) {
				case string:
					command = []string{v}
				case []string:
					command = v
				case []any:
					command = make([]string, len(v))
					for i, item := range v {
						if str, ok := item.(string); ok {
							command[i] = str
						}
					}
				default:
					return nil, &InvokeError{Code: "INVALID_INPUT", Message: "command must be a string or array", Retryable: false}
				}

				config.Transport = "stdio"
				config.Command = command
			} else {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: fmt.Sprintf("unsupported transport: %s", transport), Retryable: false}
			}

			// Register the server
			server, err := registry.Register(ctx, config)
			if err != nil {
				return nil, &InvokeError{Code: "INTERNAL", Message: fmt.Sprintf("failed to register MCP server: %v", err), Retryable: false}
			}

			return map[string]any{
				"name":        server.Config.Name,
				"status":      "registered",
				"tools_count": len(server.Tools),
				"initialized": server.Initialized,
			}, nil
		},
	}
}

// NewMCPUnregisterServerSkill returns a skill that unregisters an MCP server.
func NewMCPUnregisterServerSkill(registry *mcp.Registry) Skill {
	return Skill{
		Name:    "mcp.unregister_server",
		IsAsync: false,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			// Extract required fields
			name, ok := input["name"].(string)
			if !ok || strings.TrimSpace(name) == "" {
				return nil, &InvokeError{Code: "INVALID_INPUT", Message: "name is required", Retryable: false}
			}

			// Unregister the server
			err := registry.Unregister(name)
			if err != nil {
				return nil, &InvokeError{Code: "NOT_FOUND", Message: err.Error(), Retryable: false}
			}

			return map[string]any{
				"name":   name,
				"status": "unregistered",
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

			return map[string]any{
				"server": serverName,
				"tool":   toolName,
				"result": result,
			}, nil
		},
	}
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
	skills = append(skills, NewMCPRegisterServerSkill(registry))
	skills = append(skills, NewMCPUnregisterServerSkill(registry))
	skills = append(skills, NewMCPCallToolSkill(registry))
	skills = append(skills, NewMCPListToolsSkill(registry))

	return skills
}
