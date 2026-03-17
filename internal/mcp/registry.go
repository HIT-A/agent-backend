package mcp

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// Registry manages registered MCP servers
type Registry struct {
	servers map[string]*RegisteredServer
	mu      sync.RWMutex
}

func NewRegistry() *Registry {
	return &Registry{
		servers: make(map[string]*RegisteredServer),
	}
}

// Register registers a new MCP server
func (r *Registry) Register(ctx context.Context, config *ServerConfig) (*RegisteredServer, error) {
	name := strings.TrimSpace(config.Name)
	if name == "" {
		return nil, fmt.Errorf("server name is required")
	}

	r.mu.RLock()
	_, exists := r.servers[name]
	r.mu.RUnlock()
	if exists {
		return nil, fmt.Errorf("server '%s' already registered", name)
	}

	// Build and initialize transport/client outside lock to avoid blocking registry access.
	// A slow or failed MCP server init should not stall read operations like list/get.
	// Create MCP client
	var transport Transport
	switch config.Transport {
	case "http":
		if config.URL == "" {
			return nil, fmt.Errorf("URL is required for HTTP transport")
		}
		transport = NewHTTPTransport(config.URL)
	case "stdio":
		if len(config.Command) == 0 {
			return nil, fmt.Errorf("command is required for stdio transport")
		}
		transport = NewStdioTransport(config.Command)
	default:
		return nil, fmt.Errorf("unsupported transport: %s", config.Transport)
	}

	client := NewClient(transport)

	// Initialize the client
	if err := client.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("initialize MCP client: %w", err)
	}

	// Create server info
	server := &RegisteredServer{
		Config:       *config,
		Capabilities: *client.GetCapabilities(),
		Tools:        client.GetTools(),
		Resources:    client.GetResources(),
		Initialized:  true,
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.servers[name]; exists {
		return nil, fmt.Errorf("server '%s' already registered", name)
	}

	r.servers[name] = server

	return server, nil
}

// Unregister removes an MCP server
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("server name is required")
	}

	server, exists := r.servers[name]
	if !exists {
		return fmt.Errorf("server '%s' not found", name)
	}

	// Close the client
	if server.Config.Transport == "http" || server.Config.Transport == "stdio" {
		// Note: We don't have a reference to the client anymore
		// In production, you'd store the client and close it properly
	}

	delete(r.servers, name)

	return nil
}

// Get retrieves a registered MCP server
func (r *Registry) Get(name string) (*RegisteredServer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	server, ok := r.servers[name]
	return server, ok
}

// List returns all registered MCP servers
func (r *Registry) List() []*RegisteredServer {
	r.mu.RLock()
	defer r.mu.RUnlock()

	servers := make([]*RegisteredServer, 0, len(r.servers))
	for _, server := range r.servers {
		servers = append(servers, server)
	}

	return servers
}

// GetTool retrieves a tool from a specific server
func (r *Registry) GetTool(serverName, toolName string) (*Tool, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	server, ok := r.servers[serverName]
	if !ok {
		return nil, "", false
	}

	for _, tool := range server.Tools {
		if tool.Name == toolName {
			return &tool, serverName, true
		}
	}

	return nil, "", false
}

// ListTools lists all tools from all registered servers
func (r *Registry) ListTools() map[string][]Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]Tool)
	for name, server := range r.servers {
		result[name] = server.Tools
	}

	return result
}
