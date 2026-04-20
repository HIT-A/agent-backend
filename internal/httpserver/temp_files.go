package httpserver

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"hoa-agent-backend/internal/mcp"
	"hoa-agent-backend/internal/tempstore"
)

func handleTempUpload(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		store := opts.TempStore
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "temp storage not configured")
			return
		}

		contentType := r.Header.Get("Content-Type")

		var meta *tempstore.FileMeta
		var err error

		if contentType != "" && len(contentType) > 19 && contentType[:20] == "multipart/form-data;" {
			r.ParseMultipartForm(50 << 20)
			file, header, ferr := r.FormFile("file")
			if ferr != nil {
				writeJSONError(w, http.StatusBadRequest, "no file in form: "+ferr.Error())
				return
			}
			defer file.Close()

			meta, err = store.Save(header.Filename, header.Header.Get("Content-Type"), file)
		} else {
			var input struct {
				Name       string `json:"name"`
				MimeType   string `json:"mime_type"`
				ContentB64 string `json:"content_base64"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
				return
			}

			if input.ContentB64 == "" {
				writeJSONError(w, http.StatusBadRequest, "content_base64 is required")
				return
			}

			meta, err = store.Save(input.Name, input.MimeType, &b64Reader{data: input.ContentB64})
		}

		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSONOK(w, map[string]any{
			"ok":         true,
			"id":         meta.ID,
			"name":       meta.Name,
			"size":       meta.Size,
			"mime_type":  meta.MimeType,
			"expires_at": meta.ExpiresAt,
		})
	}
}

func handleTempDownload(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodGet)
			w.Header().Add("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		store := opts.TempStore
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "temp storage not configured")
			return
		}

		var id string
		if r.Method == http.MethodGet {
			id = r.URL.Query().Get("id")
		} else {
			var input struct {
				ID string `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			id = input.ID
		}

		if id == "" {
			writeJSONError(w, http.StatusBadRequest, "id is required")
			return
		}

		meta, reader, err := store.Get(id)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		defer reader.Close()

		w.Header().Set("Content-Type", meta.MimeType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, meta.Name))
		w.Header().Set("X-Expires-At", meta.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
		io.Copy(w, reader)
	}
}

func handleTempList(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodGet)
			w.Header().Add("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		store := opts.TempStore
		if store == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "temp storage not configured")
			return
		}

		files := store.List()
		writeJSONOK(w, map[string]any{
			"ok":    true,
			"files": files,
			"count": len(files),
		})
	}
}

func handleTempParse(opts Options) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var input struct {
			ID            string `json:"id"`
			ContentBase64 string `json:"content_base64"`
			Filename      string `json:"filename"`
			MaxChars      int    `json:"max_chars"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}

		maxChars := input.MaxChars
		if maxChars <= 0 {
			maxChars = 5000
		}
		if maxChars > 50000 {
			maxChars = 50000
		}

		var content []byte
		var filename string

		if input.ID != "" {
			store := opts.TempStore
			if store == nil {
				writeJSONError(w, http.StatusServiceUnavailable, "temp storage not configured")
				return
			}

			meta, reader, err := store.Get(input.ID)
			if err != nil {
				writeJSONError(w, http.StatusNotFound, "temp file not found: "+err.Error())
				return
			}
			defer reader.Close()

			content, err = io.ReadAll(reader)
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, "read file failed: "+err.Error())
				return
			}
			filename = meta.Name
		} else if input.ContentBase64 != "" {
			var err error
			content, err = base64.StdEncoding.DecodeString(input.ContentBase64)
			if err != nil {
				writeJSONError(w, http.StatusBadRequest, "invalid base64: "+err.Error())
				return
			}
			filename = input.Filename
			if filename == "" {
				filename = "upload"
			}
		} else {
			writeJSONError(w, http.StatusBadRequest, "either id or content_base64 is required")
			return
		}

		if len(content) == 0 {
			writeJSONError(w, http.StatusBadRequest, "file content is empty")
			return
		}

		mcpRegistry := opts.MCPRegistry
		var markdown string
		if mcpRegistry != nil {
			server, exists := mcpRegistry.Get("unstructured")
			log.Printf("[DEBUG] unstructured MCP: exists=%v initialized=%v", exists, server != nil && server.Initialized)
			if exists && server.Initialized {
				contentB64 := base64.StdEncoding.EncodeToString(content)
				result, err := callMCPTool(r.Context(), server, "convert_to_markdown", map[string]any{
					"content_base64": contentB64,
					"filename":       filename,
				})
				if err == nil {
					if md := extractMarkdownFromMCP(result); md != "" {
						markdown = md
					}
				}
			}
		}

		if markdown == "" {
			writeJSONError(w, http.StatusServiceUnavailable, "Unstructured MCP server not available or conversion failed")
			return
		}

		truncated := false
		if len(markdown) > maxChars {
			markdown = markdown[:maxChars]
			truncated = true
		}

		writeJSONOK(w, map[string]any{
			"ok":         true,
			"markdown":   markdown,
			"filename":   filename,
			"truncated":  truncated,
			"char_count": len(markdown),
		})
	}
}

func extractMarkdownFromMCP(result map[string]any) string {
	contentArr, ok := result["content"].([]any)
	if !ok || len(contentArr) == 0 {
		return ""
	}
	first, ok := contentArr[0].(map[string]any)
	if !ok {
		return ""
	}
	text, ok := first["text"].(string)
	if !ok {
		return ""
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		if text != "" {
			return text
		}
		return ""
	}
	if md, ok := parsed["markdown"].(string); ok && md != "" {
		return md
	}
	return text
}

func callMCPTool(ctx context.Context, server *mcp.RegisteredServer, toolName string, args map[string]any) (map[string]any, error) {
	if server.Client != nil && server.Initialized {
		result, err := server.Client.CallTool(ctx, toolName, args)
		if err != nil {
			return nil, fmt.Errorf("tool call failed: %w", err)
		}
		resultMap := map[string]any{}
		data, _ := json.Marshal(result)
		if err := json.Unmarshal(data, &resultMap); err != nil {
			return nil, fmt.Errorf("unmarshal result: %w", err)
		}
		return resultMap, nil
	}

	var transport mcp.Transport
	if server.Config.Transport == "http" {
		transport = mcp.NewHTTPTransport(server.Config.URL)
	} else if server.Config.Transport == "stdio" {
		if server.Config.LineDelimited {
			transport = mcp.NewLineDelimitedTransport(server.Config.Command, server.Config.Env)
		} else {
			transport = mcp.NewStdioTransport(server.Config.Command, server.Config.Env)
		}
	} else {
		return nil, fmt.Errorf("unsupported transport: %s", server.Config.Transport)
	}

	client := mcp.NewClient(transport)

	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Initialize(ctx2); err != nil {
		return nil, fmt.Errorf("failed to initialize MCP client: %w", err)
	}
	defer client.Close()

	ctx3, cancel2 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel2()

	result, err := client.CallTool(ctx3, toolName, args)
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}

	resultMap := map[string]any{}
	data, _ := json.Marshal(result)
	if err := json.Unmarshal(data, &resultMap); err != nil {
		return nil, fmt.Errorf("unmarshal result: %w", err)
	}
	return resultMap, nil
}

type b64Reader struct {
	data string
	pos  int
}

func (r *b64Reader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return
}
