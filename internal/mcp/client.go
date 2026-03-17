package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// Client is an MCP client
type Client struct {
	transport    Transport
	serverInfo   *MCPServerInfo
	capabilities *ServerCapabilities
	tools        []Tool
	resources    []Resource
	initialized  bool
}

func NewClient(transport Transport) *Client {
	return &Client{
		transport: transport,
	}
}

// Initialize initializes the MCP server connection
func (c *Client) Initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}

	// Build initialize request
	req := &Message{
		Method: "initialize",
		Params: InitializeParams{
			ProtocolVersion: "2025-03-26",
			Capabilities: ClientCapabilities{
				Resources: &ResourcesCapability{},
				Tools:     &ToolsCapability{},
			},
			ClientInfo: ClientInfo{
				Name:    "hoa-agent-backend",
				Version: "1.0.0",
			},
		},
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return fmt.Errorf("send initialize: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s (code: %d)", resp.Error.Message, resp.Error.Code)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid initialize result")
	}

	// Parse result
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	var initResult InitializeResult
	if err := json.Unmarshal(resultBytes, &initResult); err != nil {
		return fmt.Errorf("unmarshal init result: %w", err)
	}

	c.serverInfo = &initResult.ServerInfo
	c.capabilities = &initResult.Capabilities
	c.initialized = true

	// If server supports tools, list them
	if c.capabilities.Tools != nil {
		tools, err := c.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("list tools: %w", err)
		}
		c.tools = tools
	}

	// If server supports resources, list them
	if c.capabilities.Resources != nil {
		resources, err := c.ListResources(ctx)
		if err != nil {
			return fmt.Errorf("list resources: %w", err)
		}
		c.resources = resources
	}

	return nil
}

// ListTools lists available tools from the MCP server
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	req := &Message{
		Method: "tools/list",
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send tools/list: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s (code: %d)", resp.Error.Message, resp.Error.Code)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid tools/list result")
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	var listResult ToolsListResult
	if err := json.Unmarshal(resultBytes, &listResult); err != nil {
		return nil, fmt.Errorf("unmarshal tools/list result: %w", err)
	}

	return listResult.Tools, nil
}

// CallTool calls a tool on the MCP server
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*ToolCallResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	req := &Message{
		Method: "tools/call",
		Params: ToolCallParams{
			Name:      name,
			Arguments: arguments,
		},
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send tools/call: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("tools/call error: %s (code: %d)", resp.Error.Message, resp.Error.Code)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid tools/call result")
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	var callResult ToolCallResult
	if err := json.Unmarshal(resultBytes, &callResult); err != nil {
		return nil, fmt.Errorf("unmarshal tools/call result: %w", err)
	}

	return &callResult, nil
}

// ListResources lists available resources from the MCP server
func (c *Client) ListResources(ctx context.Context) ([]Resource, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	req := &Message{
		Method: "resources/list",
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send resources/list: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("resources/list error: %s (code: %d)", resp.Error.Message, resp.Error.Code)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid resources/list result")
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	var listResult ResourcesListResult
	if err := json.Unmarshal(resultBytes, &listResult); err != nil {
		return nil, fmt.Errorf("unmarshal resources/list result: %w", err)
	}

	return listResult.Resources, nil
}

// ReadResource reads a resource from the MCP server
func (c *Client) ReadResource(ctx context.Context, uri string) (*ResourceReadResult, error) {
	if !c.initialized {
		return nil, fmt.Errorf("client not initialized")
	}

	req := &Message{
		Method: "resources/read",
		Params: ResourceReadParams{
			URI: uri,
		},
	}

	resp, err := c.transport.Send(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("send resources/read: %w", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("resources/read error: %s (code: %d)", resp.Error.Message, resp.Error.Code)
	}

	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid resources/read result")
	}

	resultBytes, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}

	var readResult ResourceReadResult
	if err := json.Unmarshal(resultBytes, &readResult); err != nil {
		return nil, fmt.Errorf("unmarshal resources/read result: %w", err)
	}

	return &readResult, nil
}

// Close closes the MCP client connection
func (c *Client) Close() error {
	if c.transport != nil {
		return c.transport.Close()
	}
	return nil
}

// GetServerInfo returns the server info
func (c *Client) GetServerInfo() *MCPServerInfo {
	return c.serverInfo
}

// GetCapabilities returns the server capabilities
func (c *Client) GetCapabilities() *ServerCapabilities {
	return c.capabilities
}

// GetTools returns the available tools
func (c *Client) GetTools() []Tool {
	return c.tools
}

// GetResources returns the available resources
func (c *Client) GetResources() []Resource {
	return c.resources
}
