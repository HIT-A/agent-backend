package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// IDGenerator generates unique JSON-RPC message IDs
type IDGenerator struct {
	counter uint64
}

func (g *IDGenerator) Next() uint64 {
	return atomic.AddUint64(&g.counter, 1)
}

// Transport is the interface for MCP transport
type Transport interface {
	Send(ctx context.Context, msg *Message) (*Message, error)
	Close() error
}

// HTTPTransport implements MCP over HTTP(S)
type HTTPTransport struct {
	BaseURL     string
	Client      *http.Client
	idGenerator *IDGenerator
}

func NewHTTPTransport(baseURL string) *HTTPTransport {
	return &HTTPTransport{
		BaseURL:     baseURL,
		Client:      &http.Client{Timeout: 30 * time.Second},
		idGenerator: &IDGenerator{},
	}
}

func (t *HTTPTransport) Send(ctx context.Context, msg *Message) (*Message, error) {
	msg.ID = t.idGenerator.Next()
	msg.JSONRPC = "2.0"

	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", t.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result Message
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

func (t *HTTPTransport) Close() error {
	return nil
}

// StdioTransport implements MCP over stdio
type StdioTransport struct {
	Command     []string
	idGenerator *IDGenerator
	initialized bool
}

func NewStdioTransport(command []string) *StdioTransport {
	return &StdioTransport{
		Command:     command,
		idGenerator: &IDGenerator{},
	}
}

func (t *StdioTransport) Send(ctx context.Context, msg *Message) (*Message, error) {
	return nil, fmt.Errorf("stdio transport not yet implemented")
}

func (t *StdioTransport) Close() error {
	return nil
}
