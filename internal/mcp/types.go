package mcp

import "encoding/json"

// MCP Protocol Types based on MCP specification

// Message is the base JSON-RPC message type
type Message struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Method  string      `json:"method,omitempty"`
	Params  interface{} `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

// Error represents an MCP error
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Error codes from MCP specification
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternalError  = -32603
)

// InitializeParams contains parameters for the initialize method
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

// ClientCapabilities describes client capabilities
type ClientCapabilities struct {
	Roots     *RootsCapability     `json:"roots,omitempty"`
	Sampling  *SamplingCapability  `json:"sampling,omitempty"`
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Tools     *ToolsCapability     `json:"tools,omitempty"`
}

// RootsCapability describes root list capability
type RootsCapability struct {
	ListChanged bool `json:"listChanged"`
}

// SamplingCapability describes sampling capability
type SamplingCapability struct{}

// ResourcesCapability describes resources capability
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsCapability describes tools capability
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ClientInfo describes the client
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult contains the result of initialize
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      MCPServerInfo      `json:"serverInfo"`
}

// ServerCapabilities describes server capabilities
type ServerCapabilities struct {
	Resources *ResourcesCapability `json:"resources,omitempty"`
	Tools     *ToolsCapability     `json:"tools,omitempty"`
}

// MCPServerInfo describes the MCP server info
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolsListResult contains the result of tools/list
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ToolCallParams contains parameters for tools/call
type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolCallResult contains the result of tools/call
type ToolCallResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Content represents tool output content
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Resource represents an MCP resource
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// ResourcesListResult contains the result of resources/list
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// ResourceReadParams contains parameters for resources/read
type ResourceReadParams struct {
	URI string `json:"uri"`
}

// ResourceReadResult contains the result of resources/read
type ResourceReadResult struct {
	Contents []ResourceContent `json:"contents"`
}

// ResourceContent represents resource content
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ServerConfig describes an MCP server configuration
type ServerConfig struct {
	Name      string   `json:"name"`
	Transport string   `json:"transport"` // "stdio" or "http"
	URL       string   `json:"url"`       // for HTTP transport
	Command   []string `json:"command"`   // for stdio transport
	Enabled   bool     `json:"enabled"`
}

// RegisteredServer describes a registered MCP server
type RegisteredServer struct {
	Config       ServerConfig       `json:"config"`
	Capabilities ServerCapabilities `json:"capabilities"`
	Tools        []Tool             `json:"tools"`
	Resources    []Resource         `json:"resources"`
	Initialized  bool               `json:"initialized"`
}

// UnmarshalJSON implements custom JSON unmarshaling for ServerConfig
func (s *ServerConfig) UnmarshalJSON(data []byte) error {
	type Alias ServerConfig
	tmp := &struct {
		Command interface{} `json:"command"`
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}
	if tmp.Command != nil {
		switch v := tmp.Command.(type) {
		case string:
			s.Command = []string{v}
		case []string:
			s.Command = v
		case []interface{}:
			s.Command = make([]string, len(v))
			for i, item := range v {
				if str, ok := item.(string); ok {
					s.Command[i] = str
				}
			}
		}
	}
	return nil
}
