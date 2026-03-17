package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"hoa-agent-backend/internal/cos"
)

const (
	defaultManualRawDir = "incoming/manual_raw"
	defaultSkillRawDir  = "incoming/skill_raw"
)

type RAGIntakeInput struct {
	Repo              string   `json:"repo"`
	Branch            string   `json:"branch"`
	IntakeDir         string   `json:"intake_dir"`
	IntakeDirs        []string `json:"intake_dirs"`
	CompatLegacy      bool     `json:"compat_legacy_folders"`
	MaxFileSizeMB     int64    `json:"max_file_size_mb"`
	Collection        string   `json:"collection"`
	StoreInCOS        bool     `json:"store_in_cos"`
	COSPrefix         string   `json:"cos_prefix"`
	DeleteInvalid     bool     `json:"delete_invalid"`
	DeleteOnSucceeded bool     `json:"delete_on_succeeded"`
	MaxFiles          int      `json:"max_files"`
}

type intakeFailure struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

func NewRAGIntakeManualFolderSkill(cosStorage *cos.Storage) Skill {
	return Skill{
		Name:    "rag.intake_manual_folder",
		IsAsync: true,
		Invoke: func(ctx context.Context, input map[string]any, trace map[string]any) (map[string]any, error) {
			_ = trace

			cfg := parseRAGIntakeInput(input)
			fetcher, err := NewGitHubFetcherFromEnv()
			if err != nil {
				return nil, err
			}
			metaStore, err := NewIngestMetaStoreFromEnv()
			if err != nil {
				return nil, err
			}
			defer func() { _ = metaStore.Close() }()

			qdrant, err := NewQdrantClientFromEnv()
			if err != nil {
				return nil, err
			}
			if cfg.Collection != "" {
				qdrant.Collection = cfg.Collection
			}

			embedder, err := NewEmbeddingProviderFromEnv()
			if err != nil {
				return nil, err
			}

			intakeDirs := buildIntakeDirs(cfg)
			entries := make([]RepoFile, 0)
			for _, d := range intakeDirs {
				files, err := fetcher.ListFilesAll(ctx, cfg.Repo, cfg.Branch, d)
				if err != nil {
					return nil, err
				}
				entries = append(entries, files...)
			}

			processed := 0
			skipped := 0
			invalidDeleted := 0
			sourceDeleted := 0
			embeddedChunks := 0
			upsertedPoints := 0
			failures := make([]intakeFailure, 0)

			maxBytes := cfg.MaxFileSizeMB * 1024 * 1024
			for _, e := range entries {
				if cfg.MaxFiles > 0 && processed >= cfg.MaxFiles {
					break
				}
				processed++

				if !isSupportedParseExt(e.Path) || e.Size <= 0 || int64(e.Size) > maxBytes {
					if cfg.DeleteInvalid {
						if err := fetcher.DeleteFile(ctx, cfg.Repo, cfg.Branch, e.Path, "chore: remove invalid intake file", e.SHA); err == nil {
							invalidDeleted++
						} else {
							failures = append(failures, intakeFailure{Path: e.Path, Reason: "delete invalid failed: " + err.Error()})
						}
					}
					skipped++
					continue
				}

				sourceID := fmt.Sprintf("github://%s/%s", cfg.Repo, e.Path)
				sourceCategory := classifyIntakePath(e.Path)
				payloadSource := "manual-intake"
				if sourceCategory == "skill" {
					payloadSource = "skill-intake"
				}
				shouldSkip, err := metaStore.ShouldSkip(ctx, sourceID, e.SHA)
				if err != nil {
					failures = append(failures, intakeFailure{Path: e.Path, Reason: "meta lookup failed: " + err.Error()})
					continue
				}
				if shouldSkip {
					skipped++
					if cfg.DeleteOnSucceeded {
						if err := fetcher.DeleteFile(ctx, cfg.Repo, cfg.Branch, e.Path, "chore: remove already-ingested intake file", e.SHA); err == nil {
							sourceDeleted++
						}
					}
					continue
				}

				fc, err := fetcher.GetFile(ctx, cfg.Repo, cfg.Branch, e.Path)
				if err != nil {
					failures = append(failures, intakeFailure{Path: e.Path, Reason: "download failed: " + err.Error()})
					_ = metaStore.Upsert(ctx, IngestRecord{
						SourceID:         sourceID,
						SourceRepo:       cfg.Repo,
						SourcePath:       e.Path,
						OriginalFilename: path.Base(e.Path),
						RemoteSHA:        e.SHA,
						Status:           "failed",
						ErrorMessage:     err.Error(),
					})
					continue
				}

				sum := sha256.Sum256(fc.Content)
				contentSHA := hex.EncodeToString(sum[:])
				cosKey := ""
				if cfg.StoreInCOS && cosStorage != nil {
					cosKey = fmt.Sprintf("%s/%s/%s/%s", strings.Trim(cfg.COSPrefix, "/"), contentSHA[:2], contentSHA, safeName(path.Base(e.Path)))
					if _, err := cosStorage.SaveFile(ctx, cosKey, fc.Content, "application/octet-stream"); err != nil {
						failures = append(failures, intakeFailure{Path: e.Path, Reason: "save cos failed: " + err.Error()})
						_ = metaStore.Upsert(ctx, IngestRecord{
							SourceID:         sourceID,
							SourceRepo:       cfg.Repo,
							SourcePath:       e.Path,
							OriginalFilename: path.Base(e.Path),
							RemoteSHA:        e.SHA,
							ContentSHA256:    contentSHA,
							ContentSize:      int64(len(fc.Content)),
							COSKey:           cosKey,
							QdrantCollection: qdrant.Collection,
							Status:           "failed",
							ErrorMessage:     err.Error(),
						})
						continue
					}
				}

				doc, err := ParseDocument(fc.Path, fc.Content)
				if err != nil || strings.TrimSpace(doc.Text) == "" {
					msg := "parse failed"
					if err != nil {
						msg = err.Error()
					}
					failures = append(failures, intakeFailure{Path: e.Path, Reason: msg})
					_ = metaStore.Upsert(ctx, IngestRecord{
						SourceID:         sourceID,
						SourceRepo:       cfg.Repo,
						SourcePath:       e.Path,
						OriginalFilename: path.Base(e.Path),
						RemoteSHA:        e.SHA,
						ContentSHA256:    contentSHA,
						ContentSize:      int64(len(fc.Content)),
						COSKey:           cosKey,
						QdrantCollection: qdrant.Collection,
						Status:           "failed",
						ErrorMessage:     msg,
					})
					continue
				}

				normalized := buildNormalizedMarkdown(doc)
				ragPath := fmt.Sprintf("sources/normalized/%s/%s.md", contentSHA[:2], contentSHA)
				existing, err := fetcher.GetContentMeta(ctx, cfg.Repo, cfg.Branch, ragPath)
				if err != nil {
					failures = append(failures, intakeFailure{Path: e.Path, Reason: "read rag path meta failed: " + err.Error()})
					continue
				}
				prevSHA := ""
				if existing != nil {
					prevSHA = existing.SHA
				}
				if err := fetcher.PutFile(ctx, cfg.Repo, cfg.Branch, ragPath, "chore: add normalized rag source", []byte(normalized), prevSHA); err != nil {
					failures = append(failures, intakeFailure{Path: e.Path, Reason: "write rag file failed: " + err.Error()})
					continue
				}

				docID := fmt.Sprintf("%s:%s", cfg.Repo, ragPath)
				_ = qdrant.DeleteByDocID(ctx, qdrant.Collection, docID)

				chunks := ChunkText(doc, 1400)
				if len(chunks) == 0 {
					failures = append(failures, intakeFailure{Path: e.Path, Reason: "empty chunks"})
					continue
				}
				points := make([]QdrantPoint, 0, len(chunks))
				for _, ch := range chunks {
					vec, err := embedder.Embed(ctx, ch.Text)
					if err != nil {
						failures = append(failures, intakeFailure{Path: e.Path, Reason: "embedding failed: " + err.Error()})
						points = nil
						break
					}
					payload := map[string]any{
						"doc_id":   docID,
						"chunk_id": ch.ChunkID,
						"title":    doc.Title,
						"snippet":  trimSnippet(ch.Text),
						"source":   payloadSource,
						"category": sourceCategory,
						"repo":     cfg.Repo,
						"path":     ragPath,
					}
					points = append(points, QdrantPoint{ID: stablePointID(docID, ch.ChunkID), Vector: vec, Payload: payload})
				}
				if len(points) == 0 {
					continue
				}
				if err := qdrant.Upsert(ctx, qdrant.Collection, points); err != nil {
					failures = append(failures, intakeFailure{Path: e.Path, Reason: "qdrant upsert failed: " + err.Error()})
					continue
				}
				embeddedChunks += len(chunks)
				upsertedPoints += len(points)

				if cfg.DeleteOnSucceeded {
					if err := fetcher.DeleteFile(ctx, cfg.Repo, cfg.Branch, e.Path, "chore: remove processed intake file", e.SHA); err == nil {
						sourceDeleted++
					} else {
						failures = append(failures, intakeFailure{Path: e.Path, Reason: "delete source failed: " + err.Error()})
					}
				}

				_ = metaStore.Upsert(ctx, IngestRecord{
					SourceID:         sourceID,
					SourceRepo:       cfg.Repo,
					SourcePath:       e.Path,
					OriginalFilename: path.Base(e.Path),
					RemoteSHA:        e.SHA,
					ContentSHA256:    contentSHA,
					ContentSize:      int64(len(fc.Content)),
					COSKey:           cosKey,
					RagDataPath:      ragPath,
					QdrantCollection: qdrant.Collection,
					Status:           "succeeded",
					ErrorMessage:     "",
				})
			}

			return map[string]any{
				"repo":                   cfg.Repo,
				"branch":                 cfg.Branch,
				"intake_dir":             cfg.IntakeDir,
				"intake_dirs":            intakeDirs,
				"compat_legacy_folders":  cfg.CompatLegacy,
				"processed":              processed,
				"skipped":                skipped,
				"invalid_deleted":        invalidDeleted,
				"source_deleted":         sourceDeleted,
				"embedded_chunks":        embeddedChunks,
				"upserted_points":        upsertedPoints,
				"qdrant_collection":      qdrant.Collection,
				"failures_count":         len(failures),
				"failures":               failures,
				"max_file_size_mb":       cfg.MaxFileSizeMB,
				"delete_on_succeeded":    cfg.DeleteOnSucceeded,
				"delete_invalid":         cfg.DeleteInvalid,
				"store_in_cos":           cfg.StoreInCOS,
				"metadata_db_configured": true,
			}, nil
		},
	}
}

func parseRAGIntakeInput(input map[string]any) RAGIntakeInput {
	cfg := RAGIntakeInput{
		Repo:              "HIT-A/HITA_RagData",
		Branch:            "main",
		IntakeDir:         defaultManualRawDir,
		IntakeDirs:        nil,
		CompatLegacy:      false,
		MaxFileSizeMB:     25,
		Collection:        "",
		StoreInCOS:        true,
		COSPrefix:         "rag-intake/raw",
		DeleteInvalid:     true,
		DeleteOnSucceeded: true,
		MaxFiles:          50,
	}
	if input == nil {
		return cfg
	}
	if v, ok := input["repo"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.Repo = strings.TrimSpace(v)
	}
	if v, ok := input["branch"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.Branch = strings.TrimSpace(v)
	}
	if v, ok := input["intake_dir"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.IntakeDir = strings.Trim(strings.TrimSpace(v), "/")
	}
	if rawDirs, ok := input["intake_dirs"].([]any); ok {
		dirs := make([]string, 0, len(rawDirs))
		for _, d := range rawDirs {
			s, ok := d.(string)
			if !ok {
				continue
			}
			s = strings.Trim(strings.TrimSpace(s), "/")
			if s == "" {
				continue
			}
			dirs = append(dirs, s)
		}
		if len(dirs) > 0 {
			cfg.IntakeDirs = dirs
		}
	}
	if v, ok := input["compat_legacy_folders"].(bool); ok {
		cfg.CompatLegacy = v
	}
	if v, ok := input["collection"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.Collection = strings.TrimSpace(v)
	}
	if v, ok := input["cos_prefix"].(string); ok && strings.TrimSpace(v) != "" {
		cfg.COSPrefix = strings.TrimSpace(v)
	}
	if v, ok := input["max_file_size_mb"].(float64); ok && int64(v) > 0 {
		cfg.MaxFileSizeMB = int64(v)
	}
	if v, ok := input["store_in_cos"].(bool); ok {
		cfg.StoreInCOS = v
	}
	if v, ok := input["delete_invalid"].(bool); ok {
		cfg.DeleteInvalid = v
	}
	if v, ok := input["delete_on_succeeded"].(bool); ok {
		cfg.DeleteOnSucceeded = v
	}
	if v, ok := input["max_files"].(float64); ok && int(v) > 0 {
		cfg.MaxFiles = int(v)
	}
	return cfg
}

func isSupportedParseExt(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	switch ext {
	case ".md", ".markdown", ".txt", ".html", ".htm":
		return true
	default:
		return false
	}
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

func buildIntakeDirs(cfg RAGIntakeInput) []string {
	seen := make(map[string]struct{})
	add := func(v string, out *[]string) {
		v = strings.Trim(strings.TrimSpace(v), "/")
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		*out = append(*out, v)
	}
	out := make([]string, 0, 4)
	if len(cfg.IntakeDirs) > 0 {
		for _, d := range cfg.IntakeDirs {
			add(d, &out)
		}
	} else {
		add(cfg.IntakeDir, &out)
		if strings.Trim(strings.TrimSpace(cfg.IntakeDir), "/") == defaultManualRawDir {
			add(defaultSkillRawDir, &out)
		}
	}
	if cfg.CompatLegacy {
		add("sources/github", &out)
		add("sources/crawled", &out)
	}
	return out
}

func classifyIntakePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if strings.HasPrefix(p, defaultManualRawDir+"/") {
		return "manual"
	}
	if strings.HasPrefix(p, defaultSkillRawDir+"/") {
		return "skill"
	}
	if strings.HasPrefix(p, "sources/github/") || strings.HasPrefix(p, "sources/crawled/") {
		return "legacy"
	}
	return "custom"
}
