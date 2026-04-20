package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
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
	Command       []string
	Env           map[string]string
	LineDelimited bool
	idGenerator   *IDGenerator
	initialized   bool

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader
}

func NewStdioTransport(command []string, env map[string]string) *StdioTransport {
	return &StdioTransport{
		Command:     command,
		Env:         env,
		idGenerator: &IDGenerator{},
	}
}

func NewLineDelimitedTransport(command []string, env map[string]string) *StdioTransport {
	return &StdioTransport{
		Command:       command,
		Env:           env,
		LineDelimited: true,
		idGenerator:   &IDGenerator{},
	}
}

func (t *StdioTransport) Send(ctx context.Context, msg *Message) (*Message, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if err := t.ensureProcess(); err != nil {
		return nil, err
	}

	msg.ID = t.idGenerator.Next()
	msg.JSONRPC = "2.0"

	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("marshal message: %w", err)
	}

	if t.LineDelimited {
		if _, err := t.stdin.Write(body); err != nil {
			return nil, fmt.Errorf("write body: %w", err)
		}
		if _, err := t.stdin.Write([]byte("\n")); err != nil {
			return nil, fmt.Errorf("write newline: %w", err)
		}
	} else {
		header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
		if _, err := t.stdin.Write([]byte(header)); err != nil {
			return nil, fmt.Errorf("write header: %w", err)
		}
		if _, err := t.stdin.Write(body); err != nil {
			return nil, fmt.Errorf("write body: %w", err)
		}
	}

	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for response to message %v", msg.ID)
		}

		respBody, err := readMCPMessage(t.reader)
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}

		var resp Message
		if err := json.Unmarshal(respBody, &resp); err != nil {
			continue
		}

		if resp.ID == nil {
			continue
		}

		if sameMessageID(resp.ID, msg.ID) {
			return &resp, nil
		}
	}
}

func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stdin != nil {
		_ = t.stdin.Close()
		t.stdin = nil
	}
	if t.stdout != nil {
		_ = t.stdout.Close()
		t.stdout = nil
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_, _ = t.cmd.Process.Wait()
		t.cmd = nil
	}
	t.reader = nil
	t.initialized = false
	return nil
}

func (t *StdioTransport) ensureProcess() error {
	if t.cmd != nil {
		if t.cmd.ProcessState == nil || !t.cmd.ProcessState.Exited() {
			return nil
		}
	}
	if len(t.Command) == 0 {
		return fmt.Errorf("stdio command is empty")
	}

	cmd := exec.Command(t.Command[0], t.Command[1:]...)
	if len(t.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range t.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start stdio command: %w", err)
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout
	t.reader = bufio.NewReader(stdout)
	t.initialized = true
	return nil
}

func readMCPMessage(r *bufio.Reader) ([]byte, error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "content-length:") {
			contentLen, err := parseContentLength(trimmed)
			if err != nil {
				return nil, err
			}

			for {
				h, err := r.ReadString('\n')
				if err != nil {
					return nil, err
				}
				if strings.TrimSpace(h) == "" {
					break
				}
				th := strings.TrimSpace(h)
				if strings.HasPrefix(strings.ToLower(th), "content-length:") {
					if n, err := parseContentLength(th); err == nil {
						contentLen = n
					}
				}
			}

			body := make([]byte, contentLen)
			if _, err := io.ReadFull(r, body); err != nil {
				return nil, err
			}
			return body, nil
		}

		// Some MCP servers (e.g. brave-search) send raw JSON without Content-Length header
		if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
			return []byte(trimmed), nil
		}
	}
}

func parseContentLength(headerLine string) (int, error) {
	v := strings.TrimSpace(headerLine[len("Content-Length:"):])
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid content-length %q", v)
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid negative content-length %d", n)
	}
	return n, nil
}

func sameMessageID(a, b interface{}) bool {
	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)
	return as == bs
}
