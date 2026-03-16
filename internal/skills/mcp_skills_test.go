package skills

import (
	"context"
	"testing"

	"hoa-agent-backend/internal/mcp"
)

func TestMCPListServersSkill(t *testing.T) {
	registry := mcp.NewRegistry()
	skill := NewMCPListServersSkill(registry)

	result, err := skill.Invoke(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	servers, ok := result["servers"].([]map[string]any)
	if !ok {
		t.Fatalf("servers not found in result")
	}

	if len(servers) == 0 {
		// No servers registered yet, which is fine
		return
	}

	// Check server structure
	for _, server := range servers {
		if _, ok := server["name"]; !ok {
			t.Error("server missing name field")
		}
		if _, ok := server["transport"]; !ok {
			t.Error("server missing transport field")
		}
	}
}

func TestMCPRegisterServerSkill_InputValidation(t *testing.T) {
	registry := mcp.NewRegistry()
	skill := NewMCPRegisterServerSkill(registry)

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		errCode string
	}{
		{
			name:    "missing name",
			input:   map[string]any{},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name:    "missing transport",
			input:   map[string]any{"name": "test"},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := skill.Invoke(context.Background(), tt.input, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Invoke() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errCode != "" {
				if invokeErr, ok := err.(*InvokeError); ok {
					if invokeErr.Code != tt.errCode {
						t.Errorf("Invoke() error code = %v, want %v", invokeErr.Code, tt.errCode)
					}
				} else {
					t.Errorf("Invoke() error is not InvokeError")
				}
			}
		})
	}
}

func TestMCPUnregisterServerSkill_InputValidation(t *testing.T) {
	registry := mcp.NewRegistry()
	skill := NewMCPUnregisterServerSkill(registry)

	t.Run("missing name", func(t *testing.T) {
		_, err := skill.Invoke(context.Background(), map[string]any{}, nil)
		if err == nil {
			t.Error("expected error for missing name")
		}
	})
}

func TestMCPCallToolSkill_InputValidation(t *testing.T) {
	registry := mcp.NewRegistry()
	skill := NewMCPCallToolSkill(registry)

	t.Run("missing server", func(t *testing.T) {
		_, err := skill.Invoke(context.Background(), map[string]any{}, nil)
		if err == nil {
			t.Error("expected error for missing server")
		}
	})

	t.Run("missing tool", func(t *testing.T) {
		_, err := skill.Invoke(context.Background(), map[string]any{
			"server": "test",
		}, nil)
		if err == nil {
			t.Error("expected error for missing tool")
		}
	})
}

func TestMCPListToolsSkill(t *testing.T) {
	registry := mcp.NewRegistry()
	skill := NewMCPListToolsSkill(registry)

	result, err := skill.Invoke(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	tools, ok := result["tools"].(map[string]any)
	if !ok {
		t.Fatalf("tools not found in result")
	}

	if tools == nil {
		t.Fatal("tools is nil")
	}
}

func TestMCPRegistryOperations(t *testing.T) {
	registry := mcp.NewRegistry()

	// Test listing empty registry
	servers := registry.List()
	if len(servers) != 0 {
		t.Errorf("expected empty registry, got %d servers", len(servers))
	}

	// Test getting non-existent server
	_, ok := registry.Get("nonexistent")
	if ok {
		t.Error("expected false for non-existent server")
	}
}
