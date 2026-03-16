package skills

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GitHubFetcher is a minimal GitHub content fetcher.
type GitHubFetcher struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

type RepoFile struct {
	Path string
	SHA  string
	Size int
}

type RepoFileContent struct {
	Path    string
	SHA     string
	Content []byte
}

func NewGitHubFetcherFromEnv() (*GitHubFetcher, error) {
	base := strings.TrimSpace(os.Getenv("GITHUB_API_BASE_URL"))
	if base == "" {
		base = "https://api.github.com"
	}
	if _, err := url.Parse(base); err != nil {
		return nil, fmt.Errorf("invalid GITHUB_API_BASE_URL: %w", err)
	}

	// Create HTTP client that respects proxy environment variables
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &GitHubFetcher{
		BaseURL: strings.TrimRight(base, "/"),
		Token:   strings.TrimSpace(os.Getenv("GITHUB_TOKEN")),
		HTTP:    httpClient,
	}, nil
}

func (f *GitHubFetcher) ListFiles(ctx context.Context, repoFullName, ref, pathPrefix string) ([]RepoFile, error) {
	owner, repo, err := splitRepoFullName(repoFullName)
	if err != nil {
		return nil, err
	}
	if ref == "" {
		return nil, errors.New("ref is required")
	}

	httpClient := f.httpClient()
	endpoint := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", strings.TrimRight(f.baseURL(), "/"), owner, repo, url.PathEscape(ref))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	f.addAuth(req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("github list files failed: status=%s body=%q", resp.Status, string(b))
	}

	var decoded struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
			Size int    `json:"size"`
		} `json:"tree"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}

	prefix := strings.Trim(pathPrefix, "/")
	allowed := map[string]struct{}{
		".txt":  {},
		".md":   {},
		".html": {},
		".htm":  {},
	}

	out := make([]RepoFile, 0, len(decoded.Tree))
	for _, e := range decoded.Tree {
		if e.Type != "blob" {
			continue
		}
		if prefix != "" {
			if e.Path != prefix && !strings.HasPrefix(e.Path, prefix+"/") {
				continue
			}
		}
		ext := strings.ToLower(filepath.Ext(e.Path))
		if _, ok := allowed[ext]; !ok {
			continue
		}

		out = append(out, RepoFile{Path: e.Path, SHA: e.SHA, Size: e.Size})
	}

	return out, nil
}

func (f *GitHubFetcher) GetFile(ctx context.Context, repoFullName, ref, path string) (RepoFileContent, error) {
	owner, repo, err := splitRepoFullName(repoFullName)
	if err != nil {
		return RepoFileContent{}, err
	}
	if ref == "" {
		return RepoFileContent{}, errors.New("ref is required")
	}
	if strings.TrimSpace(path) == "" {
		return RepoFileContent{}, errors.New("path is required")
	}

	httpClient := f.httpClient()

	// Encode each segment but keep slashes.
	segments := strings.Split(path, "/")
	for i := range segments {
		segments[i] = url.PathEscape(segments[i])
	}
	encodedPath := strings.Join(segments, "/")

	endpoint := fmt.Sprintf("%s/repos/%s/%s/contents/%s?ref=%s", strings.TrimRight(f.baseURL(), "/"), owner, repo, encodedPath, url.QueryEscape(ref))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return RepoFileContent{}, err
	}
	f.addAuth(req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return RepoFileContent{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return RepoFileContent{}, fmt.Errorf("github get file failed: status=%s body=%q", resp.Status, string(b))
	}

	var decoded struct {
		Type     string `json:"type"`
		Path     string `json:"path"`
		SHA      string `json:"sha"`
		Encoding string `json:"encoding"`
		Content  string `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return RepoFileContent{}, err
	}

	if decoded.Encoding != "base64" {
		return RepoFileContent{}, fmt.Errorf("unsupported encoding: %q", decoded.Encoding)
	}

	// GitHub may include newlines; remove all whitespace.
	compact := strings.Join(strings.Fields(decoded.Content), "")
	b, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return RepoFileContent{}, err
	}

	return RepoFileContent{
		Path:    decoded.Path,
		SHA:     decoded.SHA,
		Content: b,
	}, nil
}

func (f *GitHubFetcher) httpClient() *http.Client {
	if f != nil && f.HTTP != nil {
		return f.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (f *GitHubFetcher) baseURL() string {
	if f != nil && strings.TrimSpace(f.BaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(f.BaseURL), "/")
	}
	return "https://api.github.com"
}

func (f *GitHubFetcher) addAuth(req *http.Request) {
	if f == nil {
		return
	}
	tok := strings.TrimSpace(f.Token)
	if tok == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+tok)
}

func splitRepoFullName(repoFullName string) (owner string, repo string, err error) {
	parts := strings.Split(strings.TrimSpace(repoFullName), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repoFullName %q, expected owner/repo", repoFullName)
	}
	return parts[0], parts[1], nil
}
