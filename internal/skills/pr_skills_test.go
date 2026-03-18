package skills

import (
	"context"
	"encoding/json"
	"testing"
)

func mustRawOp(t *testing.T, op map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("marshal op: %v", err)
	}
	return json.RawMessage(b)
}

func decodeRawOp(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal op: %v", err)
	}
	return out
}

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

func TestNormalizeAndValidatePROps_RewriteAppendCourseReview(t *testing.T) {
	ops := []json.RawMessage{mustRawOp(t, map[string]any{
		"op":          "append_course_review",
		"course_name": "AUTO1001",
		"topic":       "课程评价",
		"content":     "整体不错",
		"author": map[string]any{
			"name": "A",
		},
	})}

	out, err := normalizeAndValidatePROps("shenzhen", ops)
	if err != nil {
		t.Fatalf("normalizeAndValidatePROps() error = %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("unexpected output length: %d", len(out))
	}

	got := decodeRawOp(t, out[0])
	if got["op"] != "append_course_section_item" {
		t.Fatalf("op = %v, want append_course_section_item", got["op"])
	}
	if got["section_title"] != "课程评价" {
		t.Fatalf("section_title = %v, want 课程评价", got["section_title"])
	}
	item, ok := got["item"].(map[string]any)
	if !ok {
		t.Fatalf("item missing or invalid: %T", got["item"])
	}
	if item["content"] != "整体不错" {
		t.Fatalf("item.content = %v, want 整体不错", item["content"])
	}
}

func TestNormalizeAndValidatePROps_RewriteTeacherReview(t *testing.T) {
	ops := []json.RawMessage{mustRawOp(t, map[string]any{
		"op":           "append_course_review",
		"course_name":  "AUTO1001",
		"teacher_name": "王老师",
		"content":      "讲得很好",
	})}

	out, err := normalizeAndValidatePROps("shenzhen", ops)
	if err != nil {
		t.Fatalf("normalizeAndValidatePROps() error = %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("unexpected output length: %d", len(out))
	}

	got := decodeRawOp(t, out[0])
	if got["op"] != "add_course_teacher_review" {
		t.Fatalf("op = %v, want add_course_teacher_review", got["op"])
	}
	if got["teacher_name"] != "王老师" {
		t.Fatalf("teacher_name = %v, want 王老师", got["teacher_name"])
	}
}

func TestNormalizeAndValidatePROps_BlockMultiForHarbinWeihai(t *testing.T) {
	ops := []json.RawMessage{mustRawOp(t, map[string]any{
		"op":            "append_course_section_item",
		"course_name":   "AUTO1001",
		"section_title": "课程评价",
		"item": map[string]any{
			"content": "ok",
		},
	})}

	for _, campus := range []string{"harbin", "weihai"} {
		t.Run(campus, func(t *testing.T) {
			_, err := normalizeAndValidatePROps(campus, ops)
			if err == nil {
				t.Fatalf("expected error for campus %s", campus)
			}
			invokeErr, ok := err.(*InvokeError)
			if !ok {
				t.Fatalf("error type = %T, want *InvokeError", err)
			}
			if invokeErr.Code != "INVALID_OPS" {
				t.Fatalf("error code = %s, want INVALID_OPS", invokeErr.Code)
			}
		})
	}
}
