package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestHealthRoute_GET(t *testing.T) {
	r := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var got map[string]string
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal body: %v (body=%q)", err, string(b))
	}

	want := map[string]string{"status": "ok"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected body %v, got %v", want, got)
	}
}

func TestHealthRoute_NonGET_Returns405AndAllow(t *testing.T) {
	r := NewRouter()

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", res.StatusCode)
	}

	if allow := res.Header.Get("Allow"); allow != "GET, HEAD" {
		t.Fatalf("expected Allow header %q, got %q", "GET, HEAD", allow)
	}
}

func TestSkillsRoute_GET(t *testing.T) {
	r := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.StatusCode)
	}

	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var got struct {
		Skills []struct {
			Name string `json:"name"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal body: %v (body=%q)", err, string(b))
	}

	if len(got.Skills) < 1 {
		t.Fatalf("expected at least 1 skill, got %d", len(got.Skills))
	}

	if got.Skills[0].Name != "placeholder" {
		t.Fatalf("expected first skill name %q, got %q", "placeholder", got.Skills[0].Name)
	}
}

func TestSkillsRoute_NonGET_Returns405AndAllow(t *testing.T) {
	r := NewRouter()

	req := httptest.NewRequest(http.MethodPost, "/v1/skills", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", res.StatusCode)
	}

	if allow := res.Header.Get("Allow"); allow != "GET" {
		t.Fatalf("expected Allow header %q, got %q", "GET", allow)
	}
}
