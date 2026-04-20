package skills

import (
	"context"
	"hoa-agent-backend/internal/mcp"
	"log"
	"time"
)

// Global MCP registry
var mcpRegistry *mcp.Registry

// InitGlobals initializes global variables after .env is loaded
func InitGlobals() {
	mcpRegistry = mcp.NewRegistry()
}

// registerDefaultMCPServers registers default MCP servers
func registerDefaultMCPServers() {
	// Sequential Thinking MCP Server
	sequentialThinkingConfig := &mcp.ServerConfig{
		Name:      "sequential-thinking",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"npx", "-y", "@modelcontextprotocol/server-sequential-thinking"},
	}
	registerMCPServerAsync(sequentialThinkingConfig)

	// Anna's Archive MCP Server (文献搜索)
	annasArchiveConfig := &mcp.ServerConfig{
		Name:      "annas-archive",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"/root/agent-backend/bin/annas-mcp", "mcp"},
	}
	registerMCPServerAsync(annasArchiveConfig)

	// arXiv MCP Server (论文搜索)
	arxivConfig := &mcp.ServerConfig{
		Name:      "arxiv",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"python3", "/root/agent-backend/mcp-servers/arxiv/server.py"},
	}
	registerMCPServerAsync(arxivConfig)

	// Brave Search MCP Server (网页搜索)
	braveConfig := &mcp.ServerConfig{
		Name:      "brave-search",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"python3", "/root/agent-backend/mcp-servers/brave/server.py"},
	}
	registerMCPServerAsync(braveConfig)

	// Local Crawl4AI MCP Server (网页爬取)
	crawl4AIConfig := &mcp.ServerConfig{
		Name:      "crawl4ai",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"python3", "/root/agent-backend/mcp-servers/crawl4ai/server.py"},
	}
	registerMCPServerAsync(crawl4AIConfig)

	// Unstructured MCP Server (文档格式转换)
	unstructuredConfig := &mcp.ServerConfig{
		Name:      "unstructured",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"python3", "/root/agent-backend/mcp-servers/unstructured/server.py"},
	}
	registerMCPServerAsync(unstructuredConfig)
}

func registerMCPServerAsync(config *mcp.ServerConfig) {
	if config == nil || !config.Enabled {
		return
	}
	go func(cfg *mcp.ServerConfig) {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		if _, err := mcpRegistry.Register(ctx, cfg); err != nil {
			log.Printf("[mcp] default register failed: name=%s err=%v", cfg.Name, err)
		}
	}(config)
}

// GetMCPRegistry returns the global MCP registry
func GetMCPRegistry() *mcp.Registry {
	return mcpRegistry
}

// Registry stores available skills.
type Registry struct {
	skills []Skill
	index  map[string]Skill
}

func NewRegistry() *Registry {
	r := &Registry{}
	r.Register(NewEchoSkill())
	r.Register(NewSleepEchoSkill())
	r.Register(NewRAGQuerySkill())

	// Register MCP management skills
	for _, skill := range NewMCPSkillsFromEnv(mcpRegistry) {
		r.Register(skill)
	}

	// Register RAG ingestion skill
	r.Register(NewRAGIngestSkill())

	// Register unified search skill
	r.Register(NewUnifiedSearchSkill())

	// Register aggregator summarize skill
	r.Register(NewAggregatorSummarizeSkill())

	// Register Crawl4AI skills (requires MCP server registration first)
	r.Register(NewCrawl4AIPageSkill(mcpRegistry))
	r.Register(NewCrawl4AISiteSkill(mcpRegistry))
	r.Register(NewCrawl4AIStatusSkill(mcpRegistry))

	// Register HIT teacher search skills
	r.Register(NewTeacherSearchSkill(mcpRegistry))

	// Register course skills (pr-server)
	r.Register(NewCourseReadSkill())
	r.Register(NewCoursesSearchSkill())

	return r
}

func (r *Registry) Register(s Skill) {
	if r.index == nil {
		r.index = make(map[string]Skill)
	}
	r.skills = append(r.skills, s)
	r.index[s.Name] = s
}

func (r *Registry) List() []Skill {
	out := make([]Skill, len(r.skills))
	for i := range r.skills {
		out[i] = Skill{Name: r.skills[i].Name, IsAsync: r.skills[i].IsAsync}
	}
	return out
}

func (r *Registry) Get(name string) (Skill, bool) {
	if r.index == nil {
		return Skill{}, false
	}
	s, ok := r.index[name]
	return s, ok
}
