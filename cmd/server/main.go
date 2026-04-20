package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"hoa-agent-backend/internal/cos"
	"hoa-agent-backend/internal/httpserver"
	"hoa-agent-backend/internal/mcp"
	"hoa-agent-backend/internal/skills"
	syncknowledge "hoa-agent-backend/internal/sync"
	"hoa-agent-backend/internal/tempstore"
)

func main() {
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)

	if err := godotenv.Load(".env"); err != nil {
		log.Printf("DEBUG: Failed to load .env from current dir: %v", err)
	}
	if err := godotenv.Load(filepath.Join(execDir, ".env")); err != nil {
		log.Printf("DEBUG: Failed to load .env from exec dir: %v", err)
	}

	skills.InitGlobals()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	syncknowledge.StartAutoSync(ctx, 0)

	cosClient, err := cos.NewClientFromEnv()
	if err != nil {
		log.Printf("WARNING: COS initialization failed: %v", err)
	}

	mcpRegistry := skills.GetMCPRegistry()

	go registerMCPAsync(ctx, mcpRegistry, &mcp.ServerConfig{
		Name:      "brave-search",
		Transport: "stdio",
		Command:   []string{"python3", "/Users/jiaoziang/workspace/agent-backend/mcp-servers/brave/server.py"},
		Env: map[string]string{
			"BRAVE_API_KEY":        os.Getenv("BRAVE_API_KEY"),
			"BRAVE_ANSWER_API_KEY": os.Getenv("BRAVE_ANSWER_API_KEY"),
			"HTTP_PROXY":           os.Getenv("HTTP_PROXY"),
			"HTTPS_PROXY":          os.Getenv("HTTPS_PROXY"),
		},
		LineDelimited: true,
	})

	go registerMCPAsync(ctx, mcpRegistry, &mcp.ServerConfig{
		Name:          "crawl4ai",
		Transport:     "stdio",
		Command:       []string{"python3", "/Users/jiaoziang/workspace/agent-backend/mcp-servers/crawl4ai/server.py"},
		LineDelimited: true,
	})

	go registerMCPAsync(ctx, mcpRegistry, &mcp.ServerConfig{
		Name:          "unstructured",
		Transport:     "stdio",
		Command:       []string{"python3", "/Users/jiaoziang/workspace/agent-backend/mcp-servers/unstructured/server.py"},
		LineDelimited: true,
	})

	var cosStorage *cos.Storage
	if cosClient != nil {
		cosStorage = cos.NewStorage(cosClient, 10*1024*1024)
	}

	tempDir := os.Getenv("TEMP_DIR")
	if tempDir == "" {
		tempDir = "./data/temp"
	}
	tempStore, err := tempstore.New(tempDir, 0)
	if err != nil {
		log.Printf("WARNING: Temp store initialization failed: %v", err)
	}

	opts := httpserver.Options{
		COSStorage:  cosStorage,
		MCPRegistry: mcpRegistry,
		TempStore:   tempStore,
	}

	router := httpserver.NewRouter(opts)
	srv := &http.Server{Addr: ":8080", Handler: router}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	log.Printf("Server starting on :8080")
	log.Printf("Endpoints: /health, /api/*")

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
		return
	}
}

func registerMCPAsync(ctx context.Context, reg *mcp.Registry, cfg *mcp.ServerConfig) {
	log.Printf("DEBUG: Registering MCP server: %s (transport=%s)", cfg.Name, cfg.Transport)
	_, err := reg.Register(ctx, cfg)
	if err != nil {
		log.Printf("WARNING: %s MCP registration failed: %v", cfg.Name, err)
	} else {
		log.Printf("INFO: %s MCP registered successfully", cfg.Name)
	}
}
