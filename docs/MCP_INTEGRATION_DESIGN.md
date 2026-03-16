# MCP Integration Design

## Overview

Integrate MCP (Model Context Protocol) support into agent-backend to enable dynamic connection to external MCP servers (e.g., Playwright MCP, custom MCP tools).

## Architecture

```
端侧 Orchestrator
    ↓
POST /v1/skills/mcp.{server}.{tool}:invoke
    ↓
hoa-agent-backend
    ↓
MCP Client (HTTP/stdio)
    ↓
External MCP Server
    ↓
Execute tool / resource / prompt
```

## Design Goals

1. **Dynamic MCP Server Registration** - Add MCP servers at runtime
2. **Tool Discovery** - Auto-discover tools from MCP servers
3. **Unified Skill Interface** - MCP tools exposed as agent-backend skills
4. **Transport Support** - Both stdio and HTTP(S) transports
5. **Error Handling** - MCP errors mapped to SSOT error codes

## Implementation Plan

### Phase 1: MCP Client Core
- `internal/mcp/client.go` - MCP client implementation
- `internal/mcp/transport.go` - Transport layer (stdio/HTTP)
- `internal/mcp/types.go` - MCP protocol types
- Support:
  - Server initialization
  - Tool listing
  - Tool execution
  - Resource listing
  - Resource reading

### Phase 2: MCP Manager
- `internal/mcp/manager.go` - MCP server lifecycle management
- `internal/mcp/registry.go` - MCP server registry (in-memory)
- Features:
  - Register MCP servers (name, transport, config)
  - List registered servers
  - Get MCP client by name
  - Dynamic skill generation from MCP tools

### Phase 3: Skills Integration
- `internal/skills/mcp_skills.go` - MCP-related skills
- Skills:
  - `mcp.list_servers` - List registered MCP servers
  - `mcp.register_server` - Register a new MCP server
  - `mcp.unregister_server` - Remove an MCP server
  - Dynamic skills: `mcp.{server_name}.{tool_name}` - Execute MCP tool

### Phase 4: Configuration
- Environment variables for MCP server configuration
- Optional config file support

## MCP Client Protocol

### Initialization
```
Client                    Server
  |                         |
  |----- initialize ------->|
  |                         |
  |<------ capabilities ------|
  |                         |
```

### Tool Execution
```
Client                    Server
  |                         |
  |-- tools/list --------->|
  |                         |
  |<--- tools (schema) ----|
  |                         |
  |-- tools/call ------->|
  |                         |
  |<--- result/error ------|
  |                         |
```

## Configuration

### Environment Variables

```bash
# MCP servers configuration (JSON array)
MCP_SERVERS='[
  {
    "name": "playwright",
    "transport": "http",
    "url": "http://localhost:3000",
    "enabled": true
  },
  {
    "name": "filesystem",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"],
    "enabled": true
  }
]'
```

### Alternative: Config File

```json
// config/mcp_servers.json
{
  "servers": [
    {
      "name": "playwright",
      "transport": "http",
      "url": "http://localhost:3000",
      "enabled": true
    }
  ]
}
```

## Skills

### mcp.list_servers

List all registered MCP servers.

**Input:** `{}`

**Output:**
```json
{
  "servers": [
    {
      "name": "playwright",
      "transport": "http",
      "url": "http://localhost:3000",
      "enabled": true,
      "capabilities": ["tools", "resources"],
      "tools": ["navigate", "click", "screenshot", ...]
    }
  ]
}
```

### mcp.register_server

Register a new MCP server.

**Input:**
```json
{
  "name": "my-server",
  "transport": "http",
  "url": "http://localhost:3001",
  "enabled": true
}
```

**Output:**
```json
{
  "name": "my-server",
  "status": "registered"
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
  "name": "my-server",
  "status": "unregistered"
}
```

### Dynamic MCP Tool Skills

Each MCP tool is exposed as a skill:
- `mcp.{server_name}.{tool_name}`

Example:
```
POST /v1/skills/mcp.playwright.navigate:invoke
{
  "input": {
    "url": "https://example.com"
  }
}
```

## Error Handling

| MCP Error | SSOT Code | Retryable |
|-----------|------------|------------|
| Server not found | NOT_FOUND | false |
| Tool not found | INVALID_INPUT | false |
| Execution timeout | INTERNAL | true |
| Invalid tool parameters | INVALID_INPUT | false |
| Server connection failed | INTERNAL | true |

## Testing

### Unit Tests
- MCP client tests
- Manager tests
- Registry tests

### Integration Tests
- Mock MCP server (stdio)
- Mock MCP server (HTTP)
- Skill integration tests

### Example Test Server

```bash
# Run a test MCP server
npx -y @modelcontextprotocol/server-filesystem /tmp/mcp-test

# Test via agent-backend
curl -X POST http://localhost:8080/v1/skills/mcp.list_servers:invoke \
  -H "Content-Type: application/json" \
  -d '{"input": {}}'
```

## Use Cases

### 1. Playwright MCP (Browser Automation)
```bash
# Register Playwright MCP server
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{
    "input": {
      "name": "playwright",
      "transport": "http",
      "url": "http://playwright-mcp:3000"
    }
  }'

# Use Playwright tool
curl -X POST http://localhost:8080/v1/skills/mcp.playwright.navigate:invoke \
  -d '{
    "input": {
      "url": "https://example.com"
    }
  }'
```

### 2. Filesystem MCP (File Operations)
```bash
# Register Filesystem MCP server
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{
    "input": {
      "name": "filesystem",
      "transport": "stdio",
      "command": ["npx", "-y", "@modelcontextprotocol/server-filesystem", "/data"]
    }
  }'

# List files
curl -X POST http://localhost:8080/v1/skills/mcp.filesystem.read_file:invoke \
  -d '{
    "input": {
      "path": "/data/test.txt"
    }
  }'
```

## Implementation Details

### Transport Layer

#### Stdio Transport
```go
type StdioTransport struct {
    Command []string
    Process *exec.Cmd
    Stdin  io.WriteCloser
    Stdout io.ReadCloser
}
```

#### HTTP Transport
```go
type HTTPTransport struct {
    BaseURL string
    Client  *http.Client
}
```

### MCP Protocol Types

```go
type Message struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      interface{} `json:"id,omitempty"`
    Method  string      `json:"method,omitempty"`
    Params  interface{} `json:"params,omitempty"`
    Result  interface{} `json:"result,omitempty"`
    Error   *Error      `json:"error,omitempty"`
}

type Tool struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    InputSchema map[string]interface{} `json:"inputSchema"`
}

type Resource struct {
    URI         string `json:"uri"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}
```

## Future Enhancements

- [ ] MCP server health checks
- [ ] Tool caching
- [ ] MCP server hot-reload
- [ ] MCP server metrics (via Langfuse)
- [ ] WebSocket transport support
- [ ] MCP streaming tool execution
