package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestInvokeSkill_PathWithSlash_Returns404(t *testing.T) {
	r := NewRouter(Options{})

	req := httptest.NewRequest(http.MethodPost, "/v1/skills/no/slashes:invoke", strings.NewReader(`{"input": {"message": "hi"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", res.StatusCode)
	}
}

func TestInvokeSkill_Unknown_Returns404(t *testing.T) {
	r := NewRouter(Options{})

	req := httptest.NewRequest(http.MethodPost, "/v1/skills/does-not-exist:invoke", strings.NewReader(`{"input": {"message": "hi"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", res.StatusCode)
	}
}

func TestInvokeSkill_Echo_ReturnsOkWithOutput(t *testing.T) {
	r := NewRouter(Options{})

	req := httptest.NewRequest(http.MethodPost, "/v1/skills/echo:invoke", strings.NewReader(`{"input": {"message": "hi"}, "trace": {"id": "t1"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("expected status 200, got %d (body=%q)", res.StatusCode, string(b))
	}

	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var got struct {
		Ok     bool                   `json:"ok"`
		Output map[string]interface{} `json:"output"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal body: %v (body=%q)", err, string(b))
	}

	if got.Ok != true {
		t.Fatalf("expected ok=true, got %v", got.Ok)
	}

	wantOutput := map[string]interface{}{
		"input": map[string]interface{}{"message": "hi"},
		"trace": map[string]interface{}{"id": "t1"},
	}

	if !reflect.DeepEqual(got.Output, wantOutput) {
		t.Fatalf("expected output %v, got %v", wantOutput, got.Output)
	}
}

func TestInvokeSkill_NonPOST_Returns405AndAllow(t *testing.T) {
	r := NewRouter(Options{})

	req := httptest.NewRequest(http.MethodGet, "/v1/skills/echo:invoke", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", res.StatusCode)
	}

	if allow := res.Header.Get("Allow"); allow != "POST" {
		t.Fatalf("expected Allow header %q, got %q", "POST", allow)
	}
}
