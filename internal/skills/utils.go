package skills

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func downloadFromURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed: status=%d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func safeName(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	if s == "" {
		return "file.bin"
	}
	return s
}

func buildNormalizedMarkdown(doc ParsedDoc) string {
	title := strings.TrimSpace(doc.Title)
	if title == "" {
		title = "Untitled"
	}
	text := strings.TrimSpace(doc.Text)
	return fmt.Sprintf("# %s\n\n%s\n", title, text)
}

func trimSnippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 280 {
		return s
	}
	return s[:280]
}
