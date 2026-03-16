# MCP Integration Usage Guide

## Overview

MCP (Model Context Protocol) support has been integrated into hoa-agent-backend, allowing you to connect to external MCP servers and use their tools as skills.

## Quick Start

### 1. Register an MCP Server via API

```bash
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "name": "playwright",
      "transport": "http",
      "url": "http://localhost:3000"
    }
  }'
```

### 2. List Registered Servers

```bash
curl -X POST http://localhost:8080/v1/skills/mcp.list_servers:invoke \
  -H "Content-Type: application/json" \
  -d '{"input": {}}'
```

### 3. Call an MCP Tool

```bash
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "playwright",
      "tool": "navigate",
      "arguments": {
        "url": "https://example.com"
      }
    }
  }'
```

## Configuration

### Environment Variables

Configure MCP servers at startup using the `MCP_SERVERS` environment variable:

```bash
export MCP_SERVERS='[
  {
    "name": "playwright",
    "transport": "http",
    "url": "http://playwright-mcp:3000",
    "enabled": true
  },
  {
    "name": "filesystem",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-filesystem", "/data"],
    "enabled": true
  }
]'
```

### Docker Compose Example

```yaml
version: '3.8'
services:
  agent-backend:
    build: .
    ports:
      - "8080:8080"
    environment:
      - MCP_SERVERS=[{"name":"playwright","transport":"http","url":"http://playwright:3000","enabled":true}]
      - QDRANT_URL=http://qdrant:6333
    depends_on:
      - playwright-mcp
      - qdrant

  playwright-mcp:
    image: your-registry/playwright-mcp:latest
    ports:
      - "3000:3000"
```

## MCP Skills

### mcp.list_servers

List all registered MCP servers and their capabilities.

**Input:**
```json
{}
```

**Output:**
```json
{
  "ok": true,
  "output": {
    "servers": [
      {
        "name": "playwright",
        "transport": "http",
        "url": "http://localhost:3000",
        "enabled": true,
        "initialized": true,
        "tools": [...],
        "resources": [...]
      }
    ]
  }
}
```

### mcp.register_server

Register a new MCP server.

**Input:**
```json
{
  "name": "my-server",
  "transport": "http",
  "url": "http://localhost:3001"
}
```

**Output:**
```json
{
  "ok": true,
  "output": {
    "name": "my-server",
    "status": "registered",
    "tools_count": 5,
    "initialized": true
  }
}
```

### mcp.unregister_server

Unregister an MCP server.

**Input:**
```json
{
  "name": "my-server"
}
```

**Output:**
```json
{
  "ok": true,
  "output": {
    "name": "my-server",
    "status": "unregistered"
  }
}
```

### mcp.call_tool

Call a specific tool on an MCP server.

**Input:**
```json
{
  "server": "playwright",
  "tool": "navigate",
  "arguments": {
    "url": "https://example.com"
  }
}
```

**Output:**
```json
{
  "ok": true,
  "output": {
    "server": "playwright",
    "tool": "navigate",
    "result": {
      "content": [...],
      "isError": false
    }
  }
}
```

### mcp.list_tools

List all available tools from all registered MCP servers.

**Input:**
```json
{}
```

**Output:**
```json
{
  "ok": true,
  "output": {
    "tools": {
      "playwright": [
        {
          "name": "navigate",
          "description": "Navigate to a URL",
          "inputSchema": {...}
        }
      ],
      "filesystem": [...]
    }
  }
}
```

## Transport Types

### HTTP Transport

For MCP servers exposed over HTTP(S):

```json
{
  "name": "playwright",
  "transport": "http",
  "url": "http://localhost:3000"
}
```

### Stdio Transport

For MCP servers using stdio:

```json
{
  "name": "filesystem",
  "transport": "stdio",
  "command": ["npx", "-y", "@modelcontextprotocol/server-filesystem", "/data"]
}
```

## Popular MCP Servers

### Playwright MCP

Browser automation capabilities.

```bash
# Install
npm install -g @modelcontextprotocol/server-playwright

# Register
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{
    "input": {
      "name": "playwright",
      "transport": "http",
      "url": "http://localhost:3000"
    }
  }'

# Use
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -d '{
    "input": {
      "server": "playwright",
      "tool": "navigate",
      "arguments": {"url": "https://example.com"}
    }
  }'
```

### Filesystem MCP

File system operations.

```bash
# Register
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{
    "input": {
      "name": "filesystem",
      "transport": "stdio",
      "command": ["npx", "-y", "@modelcontextprotocol/server-filesystem", "/data"]
    }
  }'

# Use
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -d '{
    "input": {
      "server": "filesystem",
      "tool": "read_file",
      "arguments": {"path": "/data/test.txt"}
    }
  }'
```

## Error Handling

All MCP skills follow SSOT contract (HTTP 200):

| Error Code | Description | Retryable |
|-----------|-------------|-----------|
| INVALID_INPUT | Missing or invalid parameters | false |
| NOT_FOUND | Server or tool not found | false |
| INTERNAL | MCP server connection or execution error | true |

## Testing

### Run Tests

```bash
# Run MCP skills tests
go test ./internal/skills -v -run "MCP"

# Run all tests
go test ./...
```

### Test with a Mock MCP Server

You can test with a simple HTTP MCP server:

```go
// mock_mcp_server.go
package main

import (
    "encoding/json"
    "net/http"
)

func main() {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        var msg struct {
            JSONRPC string          `json:"jsonrpc"`
            ID      int             `json:"id"`
            Method  string          `json:"method"`
        }
        json.NewDecoder(r.Body).Decode(&msg)

        // Respond to initialize
        response := map[string]interface{}{
            "jsonrpc": "2.0",
            "id":      msg.ID,
            "result": map[string]interface{}{
                "protocolVersion": "2024-11-05",
                "capabilities": map[string]interface{}{
                    "tools": map[string]interface{}{
                        "listChanged": false,
                    },
                },
                "serverInfo": map[string]interface{}{
                    "name":    "mock-server",
                    "version": "1.0.0",
                },
            },
        }
        json.NewEncoder(w).Encode(response)
    })
    http.ListenAndServe(":3000", nil)
}
```

Run the mock server:
```bash
go run mock_mcp_server.go
```

Then register it:
```bash
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{
    "input": {
      "name": "mock",
      "transport": "http",
      "url": "http://localhost:3000"
    }
  }'
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│               端侧 Orchestrator                   │
└────────────────────┬────────────────────────────────────┘
                     │ HTTP /v1/skills/mcp.*
                     ▼
┌─────────────────────────────────────────────────────────┐
│            hoa-agent-backend                       │
│  ┌────────────────────────────────────────────┐    │
│  │  MCP Registry                            │    │
│  │  - Server lifecycle management            │    │
│  │  - Tool discovery                       │    │
│  └────────────────────────────────────────────┘    │
│          │                                          │
│          ▼                                          │
│  ┌────────────────────────────────────────────┐    │
│  │  MCP Client                              │    │
│  │  - HTTP/Stdio transport                │    │
│  │  - Tool execution                       │    │
│  └────────────────────────────────────────────┘    │
└────────────────────┬────────────────────────────────────┘
                     │ HTTP / stdio
                     ▼
┌─────────────────────────────────────────────────────────┐
│              External MCP Servers                  │
│  ┌────────────┐  ┌────────────┐  ┌───────────┐│
│  │ Playwright │  │ Filesystem │  │ Custom... ││
│  │   MCP      │  │    MCP     │  │   MCP     ││
│  └────────────┘  └────────────┘  └───────────┘│
└─────────────────────────────────────────────────────────┘
```

## Integration with End-side

The end-side Orchestrator (HITA_L/HITA_X) can now:

1. **Discover available MCP tools** via `mcp.list_tools`
2. **Register new MCP servers** dynamically via `mcp.register_server`
3. **Execute MCP tools** via `mcp.call_tool`

Example flow:
```
User: "Open https://example.com and take a screenshot"
  ↓
Orchestrator: Analyze request
  ↓
Orchestrator: Call mcp.call_tool
  - server: "playwright"
  - tool: "navigate"
  - arguments: {"url": "https://example.com"}
  ↓
Orchestrator: Call mcp.call_tool again
  - server: "playwright"
  - tool: "screenshot"
  - arguments: {...}
  ↓
Orchestrator: Return screenshot to user
```

## Future Enhancements

- [ ] MCP server health checks
- [ ] Tool caching
- [ ] MCP server metrics (via Langfuse)
- [ ] WebSocket transport support
- [ ] MCP streaming tool execution
- [ ] Dynamic skill generation for each MCP tool
