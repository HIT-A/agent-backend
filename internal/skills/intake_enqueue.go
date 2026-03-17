package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

func enqueueRawToGitHub(ctx context.Context, repo, branch, baseDir, sourceTag, originalFilename string, content []byte) (string, error) {
	if strings.TrimSpace(repo) == "" {
		return "", fmt.Errorf("repo is required")
	}
	if strings.TrimSpace(branch) == "" {
		return "", fmt.Errorf("branch is required")
	}
	if strings.TrimSpace(baseDir) == "" {
		return "", fmt.Errorf("baseDir is required")
	}
	if len(content) == 0 {
		return "", fmt.Errorf("content is empty")
	}

	fetcher, err := NewGitHubFetcherFromEnv()
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(content)
	contentSHA := hex.EncodeToString(hash[:])

	baseName := path.Base(strings.TrimSpace(originalFilename))
	if baseName == "" || baseName == "." || baseName == "/" {
		baseName = "file.bin"
	}
	ext := strings.ToLower(filepath.Ext(baseName))
	if ext == "" {
		ext = ".bin"
	}
	stem := strings.TrimSuffix(baseName, filepath.Ext(baseName))
	if strings.TrimSpace(stem) == "" {
		stem = "file"
	}

	baseDir = strings.Trim(strings.TrimSpace(baseDir), "/")
	sourceTag = strings.Trim(strings.TrimSpace(sourceTag), "/")
	targetPath := fmt.Sprintf("%s/%s/%s-%s%s", baseDir, sourceTag, contentSHA[:12], safeName(stem), ext)

	meta, err := fetcher.GetContentMeta(ctx, repo, branch, targetPath)
	if err != nil {
		return "", err
	}
	prevSHA := ""
	if meta != nil {
		prevSHA = meta.SHA
	}

	if err := fetcher.PutFile(ctx, repo, branch, targetPath, "chore: queue raw intake file", content, prevSHA); err != nil {
		return "", err
	}

	return targetPath, nil
}
