# MCP Integration for hoa-agent-backend

## Overview

This integration adds [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) support to hoa-agent-backend, enabling dynamic connection to external MCP servers and using their tools as skills.

## Features

- ✅ **Dynamic MCP Server Registration** - Register MCP servers at runtime
- ✅ **HTTP Transport** - Connect to HTTP-based MCP servers
- ✅ **Stdio Transport** - Connect to stdio-based MCP servers (basic support)
- ✅ **Tool Discovery** - Auto-discover tools from MCP servers
- ✅ **Unified Skill Interface** - MCP tools exposed as agent-backend skills
- ✅ **Error Mapping** - MCP errors mapped to SSOT error codes
- ✅ **Server Lifecycle Management** - Register, list, unregister servers
- ✅ **Environment Configuration** - Configure MCP servers via environment variables

## Architecture

```
端侧 Orchestrator
    ↓
POST /v1/skills/mcp.*:invoke
    ↓
hoa-agent-backend
    ├─ MCP Registry (in-memory)
    ├─ MCP Client (HTTP/stdio transport)
    └─ MCP Server (external)
    ↓
Execute MCP tool / resource / prompt
```

## Quick Start

### 1. Start Agent Backend

```bash
# With MCP servers configured via environment
export MCP_SERVERS='[
  {
    "name": "playwright",
    "transport": "http",
    "url": "http://playwright:3000",
    "enabled": true
  }
]'

go run cmd/server/main.go
```

### 2. Register an MCP Server

```bash
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "name": "playwright",
      "transport": "http",
      "url": "http://playwright:3000"
    }
  }'
```

### 3. List Registered Servers

```bash
curl -X POST http://localhost:8080/v1/skills/mcp.list_servers:invoke \
  -H "Content-Type: application/json" \
  -d '{"input": {}}'
```

### 4. Call an MCP Tool

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

## Available Skills

### Management Skills

| Skill | Description |
|-------|-------------|
| `mcp.list_servers` | List all registered MCP servers |
| `mcp.register_server` | Register a new MCP server |
| `mcp.unregister_server` | Unregister an MCP server |
| `mcp.list_tools` | List all tools from all registered servers |
| `mcp.call_tool` | Call a specific tool on an MCP server |

See [MCP_USAGE.md](MCP_USAGE.md) for detailed usage.

## Configuration

### Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `MCP_SERVERS` | JSON array of MCP server configurations | See below |
| `PR_SERVER_URL` | Base URL of pr-server | `http://localhost:8080` |

### MCP_SERVERS Format

```json
[
  {
    "name": "playwright",
    "transport": "http",
    "url": "http://playwright:3000",
    "enabled": true
  },
  {
    "name": "filesystem",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-filesystem", "/data"],
    "enabled": false
  }
]
```

### Example Config File

See `config/mcp_servers.example.json` for a complete example.

## Implementation Details

### Core Components

- **`internal/mcp/types.go`** - MCP protocol types
- **`internal/mcp/transport.go`** - Transport layer (HTTP/stdio)
- **`internal/mcp/client.go`** - MCP client implementation
- **`internal/mcp/registry.go`** - MCP server lifecycle management
- **`internal/skills/mcp_skills.go`** - MCP management skills

### Key Features

#### 1. Transport Abstraction

```go
type Transport interface {
    Send(ctx context.Context, msg *Message) (*Message, error)
    Close() error
}
```

Supports:
- **HTTPTransport** - Full implementation
- **StdioTransport** - Basic implementation (not fully tested)

#### 2. MCP Client

```go
client := mcp.NewClient(transport)
client.Initialize(ctx)
tools := client.ListTools(ctx)
result := client.CallTool(ctx, "tool_name", arguments)
```

#### 3. Server Registry

```go
registry := mcp.NewRegistry()
server, err := registry.Register(ctx, config)
servers := registry.List()
server, ok := registry.Get(name)
```

## Testing

### Run Tests

```bash
# Run MCP-specific tests
go test ./internal/skills -v -run "MCP"

# Run all tests
go test ./...
```

### Test Results

```bash
$ go test ./internal/skills -v -run "MCP"
=== RUN   TestMCPListServersSkill
--- PASS: TestMCPListServersSkill (0.00s)
=== RUN   TestMCPRegisterServerSkill_InputValidation
--- PASS: TestMCPRegisterServerSkill_InputValidation (0.00s)
=== RUN   TestMCPUnregisterServerSkill_InputValidation
--- PASS: TestMCPUnregisterServerSkill_InputValidation (0.00s)
=== RUN   TestMCPCallToolSkill_InputValidation
--- PASS: TestMCPCallToolSkill_InputValidation (0.00s)
=== RUN   TestMCPListToolsSkill
--- PASS: TestMCPListToolsSkill (0.00s)
=== RUN   TestMCPRegistryOperations
--- PASS: TestMCPRegistryOperations (0.00s)
PASS
ok  	hoa-agent-backend/internal/skills	0.813s
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
```

## Error Handling

All MCP skills follow SSOT contract (HTTP 200):

| MCP Error | SSOT Code | Retryable |
|-----------|------------|------------|
| Server not found | NOT_FOUND | false |
| Tool not found | INVALID_INPUT | false |
| Execution timeout | INTERNAL | true |
| Invalid tool parameters | INVALID_INPUT | false |
| Server connection failed | INTERNAL | true |

## Use Cases

### 1. Browser Automation

```bash
# Navigate to a page
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -d '{
    "input": {
      "server": "playwright",
      "tool": "navigate",
      "arguments": {"url": "https://example.com"}
    }
  }'

# Take screenshot
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -d '{
    "input": {
      "server": "playwright",
      "tool": "screenshot",
      "arguments": {"path": "/tmp/screenshot.png"}
    }
  }'
```

### 2. File Operations

```bash
# Read file
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -d '{
    "input": {
      "server": "filesystem",
      "tool": "read_file",
      "arguments": {"path": "/data/test.txt"}
    }
  }'
```

### 3. End-side Integration

The end-side Orchestrator can:

1. **Discover tools** via `mcp.list_tools`
2. **Register servers** dynamically via `mcp.register_server`
3. **Execute tools** via `mcp.call_tool`

Example:
```
User: "Open https://hitsz.edu.cn and take a screenshot"
  ↓
Orchestrator: mcp.call_tool("playwright", "navigate", {url: "..."})
  ↓
Orchestrator: mcp.call_tool("playwright", "screenshot", {...})
  ↓
Orchestrator: Display screenshot to user
```

## Future Enhancements

- [ ] Full stdio transport implementation
- [ ] MCP server health checks
- [ ] Tool caching
- [ ] MCP server metrics (via Langfuse)
- [ ] WebSocket transport support
- [ ] MCP streaming tool execution
- [ ] Dynamic skill generation for each MCP tool
- [ ] MCP resource support (read/write)

## Documentation

- [MCP_INTEGRATION_DESIGN.md](MCP_INTEGRATION_DESIGN.md) - Design document
- [MCP_USAGE.md](MCP_USAGE.md) - Usage guide
- [config/mcp_servers.example.json](config/mcp_servers.example.json) - Example configuration

## License

This integration is part of hoa-agent-backend and follows the same license.
