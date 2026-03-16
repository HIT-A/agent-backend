package skills

import (
	"context"
	"hoa-agent-backend/internal/cos"
	"hoa-agent-backend/internal/mcp"
)

// Global MCP registry
var mcpRegistry *mcp.Registry

// Global COS storage
var cosStorage *cos.Storage

func init() {
	mcpRegistry = mcp.NewRegistry()
	cosStorage = cos.NewDefaultStorage()

	// Register default MCP servers
	registerDefaultMCPServers()
}

// registerDefaultMCPServers registers default MCP servers
func registerDefaultMCPServers() {
	// Sequential Thinking MCP Server
	// See: https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking
	sequentialThinkingConfig := &mcp.ServerConfig{
		Name:      "sequential-thinking",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"npx", "-y", "@modelcontextprotocol/server-sequential-thinking"},
	}
	_, _ = mcpRegistry.Register(context.Background(), sequentialThinkingConfig)

	// Anna's Archive MCP Server (文献搜索)
	// See: https://github.com/iosifache/annas-mcp
	// API key: 6qr5npo8S1ec3VZTmXhTwneHjaBAw
	// Set API key via environment variable before starting the server
	annasArchiveConfig := &mcp.ServerConfig{
		Name:      "annas-archive",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"npx", "-y", "annas-mcp"},
	}
	_, _ = mcpRegistry.Register(context.Background(), annasArchiveConfig)

	// arXiv MCP Server (论文搜索)
	// See: https://github.com/blazickjp/arxiv-mcp-server
	arxivConfig := &mcp.ServerConfig{
		Name:      "arxiv",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"npx", "-y", "arxiv-mcp-server"},
	}
	_, _ = mcpRegistry.Register(context.Background(), arxivConfig)

	// Brave Search MCP Server (网页搜索)
	// See: https://github.com/brave/brave-search-mcp-server
	// API Key: BSApGz16G0EmJB5CvCnGsjyTL4yZV5f
	braveConfig := &mcp.ServerConfig{
		Name:      "brave-search",
		Enabled:   true,
		Transport: "stdio",
		Command:   []string{"npx", "-y", "brave-search-mcp-server"},
	}
	_, _ = mcpRegistry.Register(context.Background(), braveConfig)
}

// GetMCPRegistry returns the global MCP registry
func GetMCPRegistry() *mcp.Registry {
	return mcpRegistry
}

// GetCOSStorage returns the global COS storage
func GetCOSStorage() *cos.Storage {
	return cosStorage
}

// Registry stores available skills.
// Minimal implementation: in-memory list.
type Registry struct {
	skills []Skill
	index  map[string]Skill
}

func NewRegistry() *Registry {
	r := &Registry{}
	r.Register(NewEchoSkill())
	r.Register(NewSleepEchoSkill())
	r.Register(NewRAGQuerySkill())
	r.Register(NewPRPreviewSkill())
	r.Register(NewPRSubmitSkill())
	r.Register(NewPRLookupSkill())

	// Register MCP management skills
	for _, skill := range NewMCPSkillsFromEnv(mcpRegistry) {
		r.Register(skill)
	}

	// Register COS storage skills
	r.Register(NewCOSSaveFileSkill(cosStorage))
	r.Register(NewCOSDeleteFileSkill(cosStorage))
	r.Register(NewCOSListFilesSkill(cosStorage))
	r.Register(NewCOSGetPresignedURLSkill(cosStorage))
	r.Register(NewCOSGetQuotaSkill(cosStorage))

	// Register RAG ingestion skill
	r.Register(NewRAGIngestSkill(cosStorage))

	// Register file upload/download skills
	r.Register(NewFilesUploadSkill(cosStorage))
	r.Register(NewFilesDownloadSkill(cosStorage))

	// Register unified search skill
	r.Register(NewUnifiedSearchSkill())

	// Register aggregator summarize skill
	r.Register(NewAggregatorSummarizeSkill())

	// Register data ingest skills
	r.Register(NewDataIngestSkill())
	r.Register(NewBatchDataIngestSkill())

	// Register Crawl4AI skills (requires MCP server registration first)
	r.Register(NewCrawl4AIPageSkill(mcpRegistry))
	r.Register(NewCrawl4AISiteSkill(mcpRegistry, cosStorage))
	r.Register(NewCrawl4AIStatusSkill(mcpRegistry))

	// Register HIT teacher search skills
	r.Register(NewTeacherSearchSkill(mcpRegistry))
	r.Register(NewTeacherBatchSearchSkill(mcpRegistry))

	// Register course skills (pr-server)
	r.Register(NewCourseReadSkill())
	r.Register(NewCoursesSearchSkill())

	// Register GitHub batch download skills
	r.Register(NewGitHubBatchDownloadSkill(mcpRegistry, cosStorage))
	r.Register(NewDocumentConverterSkill(mcpRegistry))
	r.Register(NewRAGIngestFromGitHubSkill(mcpRegistry, cosStorage))

	// Register RAG sync to repo skill
	r.Register(NewRAGSyncToRepoSkill(cosStorage))

	return r
}

func (r *Registry) Register(s Skill) {
	if r.index == nil {
		r.index = make(map[string]Skill)
	}
	// Keep deterministic list order: append in registration order.
	r.skills = append(r.skills, s)
	r.index[s.Name] = s
}

func (r *Registry) List() []Skill {
	// Return a copy to keep registry immutable from callers.
	//
	// NOTE: We intentionally exclude Invoke from the returned slice so callers can
	// safely compare and/or serialize the list.
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
