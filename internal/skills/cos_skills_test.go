package skills

import (
	"context"
	"testing"

	"hoa-agent-backend/internal/cos"
)

func TestCOSSaveFileSkill_InputValidation(t *testing.T) {
	storage := cos.NewDefaultStorage()
	skill := NewCOSSaveFileSkill(storage)

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		errCode string
	}{
		{
			name:    "missing key",
			input:   map[string]any{},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name: "missing content",
			input: map[string]any{
				"key": "test.txt",
			},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name: "invalid base64",
			input: map[string]any{
				"key":            "test.txt",
				"content_base64": "invalid-base64!!!",
			},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name: "valid input",
			input: map[string]any{
				"key":            "test.txt",
				"content_base64": "SGVsbG8gV29ybGQ=", // "Hello World"
				"content_type":   "text/plain",
			},
			wantErr: false,
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

func TestCOSDeleteFileSkill_InputValidation(t *testing.T) {
	storage := cos.NewDefaultStorage()
	skill := NewCOSDeleteFileSkill(storage)

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		errCode string
	}{
		{
			name:    "missing key",
			input:   map[string]any{},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name: "valid key",
			input: map[string]any{
				"key": "test.txt",
			},
			wantErr: false,
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

func TestCOSListFilesSkill(t *testing.T) {
	storage := cos.NewDefaultStorage()
	skill := NewCOSListFilesSkill(storage)

	result, err := skill.Invoke(context.Background(), map[string]any{}, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	if _, ok := result["files"]; !ok {
		t.Error("files field not found in result")
	}

	if _, ok := result["count"]; !ok {
		t.Error("count field not found in result")
	}
}

func TestCOSGetPresignedURLSkill_InputValidation(t *testing.T) {
	storage := cos.NewDefaultStorage()
	skill := NewCOSGetPresignedURLSkill(storage)

	tests := []struct {
		name    string
		input   map[string]any
		wantErr bool
		errCode string
	}{
		{
			name:    "missing key",
			input:   map[string]any{},
			wantErr: true,
			errCode: "INVALID_INPUT",
		},
		{
			name: "valid key",
			input: map[string]any{
				"key":             "test.txt",
				"expires_minutes": 60.0,
			},
			wantErr: false,
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

func TestCOSGetQuotaSkill(t *testing.T) {
	storage := cos.NewDefaultStorage()
	skill := NewCOSGetQuotaSkill(storage)

	result, err := skill.Invoke(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}

	requiredFields := []string{"used_bytes", "limit_bytes", "used_gb", "limit_gb", "percentage"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			t.Errorf("field %s not found in result", field)
		}
	}
}
