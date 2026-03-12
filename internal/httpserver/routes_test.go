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
