package skills

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPRPreviewSkill_InputValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skip pr-server dependent tests")
	}

	skill := NewPRPreviewSkill()

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		errCode string
	}{
		{
			name:    "missing campus",
			input:   map[string]any{},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name:    "missing course_code",
			input:   map[string]any{"campus": "shenzhen"},
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

func TestPRSubmitSkill_InputValidation(t *testing.T) {
	skill := NewPRSubmitSkill()

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		errCode string
	}{
		{
			name:    "missing campus",
			input:   map[string]any{},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name:    "missing course_code",
			input:   map[string]any{"campus": "shenzhen"},
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

func TestPRLookupSkill_InputValidation(t *testing.T) {
	skill := NewPRLookupSkill()

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		errCode string
	}{
		{
			name:    "missing org",
			input:   map[string]any{},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name:    "missing repo",
			input:   map[string]any{"org": "HITSZ-OpenAuto"},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name:    "missing pr",
			input:   map[string]any{"org": "HITSZ-OpenAuto", "repo": "COMP1011"},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name:    "invalid pr",
			input:   map[string]any{"org": "HITSZ-OpenAuto", "repo": "COMP1011", "pr": 0},
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

func TestTranslatePRServerError(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{"TOML_SCHEMA_ERROR", "INVALID_INPUT"},
		{"RENDER_FAILED", "INTERNAL"},
		{"REPO_NOT_FOUND", "NOT_FOUND"},
		{"TOML_NOT_FOUND", "NOT_FOUND"},
		{"UNKNOWN_ERROR", "UNKNOWN_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := translatePRServerError(tt.code)
			if got != tt.expected {
				t.Errorf("translatePRServerError() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		code     string
		expected bool
	}{
		{"TOML_SCHEMA_ERROR", false},
		{"RENDER_FAILED", true},
		{"REPO_NOT_FOUND", false},
		{"UNKNOWN_ERROR", true},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := isRetryable(tt.code)
			if got != tt.expected {
				t.Errorf("isRetryable() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestOpsMarshalling tests ops marshalling
func TestOpsMarshalling(t *testing.T) {
	ops := []map[string]any{
		{
			"op":            "add_lecturer_review",
			"lecturer_name": "Alice Smith",
			"content":       "Great professor!",
			"author": map[string]any{
				"name": "Student A",
				"link": "https://example.com",
				"date": "2025-01-15",
			},
		},
	}

	for _, op := range ops {
		data, err := json.Marshal(op)
		if err != nil {
			t.Fatalf("marshal op: %v", err)
		}

		var raw json.RawMessage
		err = json.Unmarshal(data, &raw)
		if err != nil {
			t.Fatalf("unmarshal to RawMessage: %v", err)
		}

		if len(raw) == 0 {
			t.Error("raw message is empty")
		}
	}
}
