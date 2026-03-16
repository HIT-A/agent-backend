# MCP Servers Integration Plan

## Overview

Integrate recommended MCP servers into hoa-agent-backend to enhance agent capabilities.

## Recommended MCP Servers

### 1. Browser-use MCP (Browser Automation)
**GitHub:** https://github.com/browser-use/browser-use

**Features:**
- Browser automation using Playwright
- Navigate to URLs
- Click elements
- Fill forms
- Take screenshots
- Execute JavaScript

**Use Cases:**
- 自动登录教务系统
- 抓取需要复杂 JS 渲染的课程资源
- 处理没有 API 的校内网站

**Configuration:**
```json
{
  "name": "browser-use",
  "transport": "http",
  "url": "http://localhost:7777",
  "enabled": true
}
```

**Docker Setup:**
```bash
# Pull image
docker pull browser-use/browser-use:latest

# Run server
docker run -p 7777:7777 browser-use/browser-use:latest
```

### 2. Sequential Read & Fetch (Web Content to Markdown)
**GitHub:** https://github.com/modelcontextprotocol/servers/tree/main/src/fetch

**NPM:** `@modelcontextprotocol/server-fetch`

**Features:**
- Convert web pages to Markdown
- Remove HTML clutter
- Extract main content
- Support for dynamic pages

**Use Cases:**
- 获取 CSDN 博客的文本
- 获取公开新闻内容
- 将 HTML 网页转换为 LLM 友好的格式

**Configuration:**
```json
{
  "name": "fetch",
  "transport": "stdio",
  "command": ["npx", "-y", "@modelcontextprotocol/server-fetch"],
  "enabled": true
}
```

### 3. Qdrant MCP (Vector Database Connection)
**GitHub:** https://github.com/qdrant/mcp-server-qdrant

**Features:**
- Direct semantic search
- Connect to existing Qdrant instance
- Query vectors
- Upsert points

**Use Cases:**
- 搜索"薪火笔记"里的相关内容
- 检索课程文档
- RAG 查询（替代现有 rag.query 技能）

**Configuration:**
```json
{
  "name": "qdrant-mcp",
  "transport": "stdio",
  "command": ["npx", "-y", "@qdrant/mcp-server-qdrant"],
  "enabled": false
}
```

**Note:** Disable by default since we already have rag.query skill using Qdrant.

### 4. Filesystem MCP (Local Code & Documents Access)
**GitHub:** https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem

**Features:**
- Read local files
- Write local files
- List directories
- Search files

**Use Cases:**
- 读取 HITA_Project 目录
- 方便 Agent 自我迭代代码
- 读取/写入配置文件

**Configuration:**
```json
{
  "name": "filesystem",
  "transport": "stdio",
  "command": [
    "npx", "-y",
    "@modelcontextprotocol/server-filesystem",
    "/path/to/HITA_Project"
  ],
  "enabled": true
}
```

### 5. GitHub MCP (Repository Management)
**GitHub:** https://github.com/modelcontextprotocol/servers/tree/main/src/github

**Features:**
- Fetch latest course README
- Create PRs
- List repositories
- Read file contents

**Use Cases:**
- 自动拉取最新的课程 README
- 提交 PR 重构
- 管理课程代码仓库

**Configuration:**
```json
{
  "name": "github",
  "transport": "stdio",
  "command": ["npx", "-y", "@modelcontextprotocol/server-github"],
  "enabled": true
}
```

### 6. Brave Search MCP (External Information Search)
**GitHub:** https://github.com/modelcontextprotocol/servers/tree/main/src/brave-search

**Features:**
- Web search with Brave API
- More LLM-friendly than Google API
- Free API quota
- Get search results

**Use Cases:**
- 外部信息搜索
- 查找最新资讯
- 补充知识库

**Configuration:**
```json
{
  "name": "brave-search",
  "transport": "stdio",
  "command": ["npx", "-y", "@modelcontextprotocol/server-brave-search"],
  "enabled": false
}
```

**API Key Setup:**
```bash
# Get API key from: https://api.search.brave.com/app/dashboard
export BRAVE_API_KEY="your-api-key-here"

# Or set in environment when starting MCP server
```

## Integration Strategy

### Phase 1: Quick Wins (Immediate Value)

1. **Filesystem MCP** - Enable local file access
   - High value for agent self-iteration
   - Low complexity
   - Safe to enable by default

2. **GitHub MCP** - Enable repository management
   - Complements existing PR skills
   - Medium complexity
   - Safe to enable by default

### Phase 2: Core Capabilities

3. **Browser-use MCP** - Browser automation
   - High value for scraping教务系统
   - Medium complexity
   - Requires Docker setup

4. **Fetch MCP** - Web content to Markdown
   - Medium value for HTML processing
   - Low complexity
   - Safe to enable by default

### Phase 3: Advanced Features

5. **Brave Search MCP** - External search
   - Medium value for information retrieval
   - Low complexity
   - Requires API key (disable by default)

6. **Qdrant MCP** - Alternative vector search
   - Low priority (we already have rag.query)
   - Low complexity
   - Disable by default

## Docker Compose Setup

```yaml
version: '3.8'

services:
  # Agent Backend
  agent-backend:
    build: .
    ports:
      - "8080:8080"
    environment:
      - MCP_SERVERS=${MCP_SERVERS}
      - PR_SERVER_URL=http://pr-server:8080
      - QDRANT_URL=http://qdrant:6333
    depends_on:
      - browser-use
      - fetch-mcp
      - filesystem-mcp
      - github-mcp
    volumes:
      - ./HITA_Project:/data

  # Browser-use MCP
  browser-use:
    image: browser-use/browser-use:latest
    ports:
      - "7777:7777"
    restart: unless-stopped

  # Filesystem MCP (mounted to project directory)
  filesystem-mcp:
    image: node:20
    working_dir: /data
    command: >
      sh -c "npm install -g @modelcontextprotocol/server-filesystem &&
      npx @modelcontextprotocol/server-filesystem /data"
    volumes:
      - ./HITA_Project:/data
    restart: unless-stopped
```

## Environment Configuration

### Minimal Setup (Development)
```bash
# Only filesystem and GitHub MCPs
export MCP_SERVERS='[
  {
    "name": "filesystem",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-filesystem", "./HITA_Project"],
    "enabled": true
  },
  {
    "name": "github",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-github"],
    "enabled": true
  }
]'
```

### Full Setup (Production)
```bash
# All MCP servers except Qdrant (we have rag.query) and Brave (needs API key)
export MCP_SERVERS='[
  {
    "name": "browser-use",
    "transport": "http",
    "url": "http://browser-use:7777",
    "enabled": true
  },
  {
    "name": "fetch",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-fetch"],
    "enabled": true
  },
  {
    "name": "filesystem",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-filesystem", "./HITA_Project"],
    "enabled": true
  },
  {
    "name": "github",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-github"],
    "enabled": true
  },
  {
    "name": "brave-search",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-brave-search"],
    "enabled": false
  },
  {
    "name": "qdrant-mcp",
    "transport": "stdio",
    "command": ["npx", "-y", "@qdrant/mcp-server-qdrant"],
    "enabled": false
  }
]'

# Optional: Brave API key
export BRAVE_API_KEY="your-api-key-here"
```

## Testing

### Test Each MCP Server

```bash
# 1. Test Filesystem MCP
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{"input":{"name":"filesystem","transport":"stdio","command":["npx","-y","@modelcontextprotocol/server-filesystem","/tmp"]}}'

curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -d '{"input":{"server":"filesystem","tool":"read_file","arguments":{"path":"/tmp/test.txt"}}}'

# 2. Test GitHub MCP
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{"input":{"name":"github","transport":"stdio","command":["npx","-y","@modelcontextprotocol/server-github"]}}'

# 3. Test Browser-use MCP
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{"input":{"name":"browser-use","transport":"http","url":"http://browser-use:7777"}}'

# 4. Test Fetch MCP
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -d '{"input":{"name":"fetch","transport":"stdio","command":["npx","-y","@modelcontextprotocol/server-fetch"]}}'
```

## Use Case Examples

### 1. 教务系统登录 + 课程资源抓取

```bash
# Agent Orchestrator flow:
# 1. 使用 browser-use MCP 登录教务系统
POST /v1/skills/mcp.call_tool:invoke
{
  "input": {
    "server": "browser-use",
    "tool": "navigate",
    "arguments": {
      "url": "https://教务系统域名/login"
    }
  }
}

# 2. 填写登录表单
POST /v1/skills/mcp.call_tool:invoke
{
  "input": {
    "server": "browser-use",
    "tool": "fill_form",
    "arguments": {
      "selector": "#username",
      "value": "your-username"
    }
  }
}

# 3. 使用 filesystem MCP 保存登录凭证到本地
POST /v1/skills/mcp.call_tool:invoke
{
  "input": {
    "server": "filesystem",
    "tool": "write_file",
    "arguments": {
      "path": "/data/credentials.json",
      "content": "{\"username\": \"...\", \"password\": \"...\"}"
    }
  }
}
```

### 2. 课程文档检索 + 网页内容提取

```bash
# 1. 使用 rag.query 技能搜索课程文档
POST /v1/skills/rag.query:invoke
{
  "input": {
    "query": "线性代数 课程大纲",
    "top_k": 10
  }
}

# 2. 使用 fetch MCP 获取 CSDN 博客内容
POST /v1/skills/mcp.call_tool:invoke
{
  "input": {
    "server": "fetch",
    "tool": "fetch",
    "arguments": {
      "url": "https://blog.csdn.net/xxx/article/details/xxx"
    }
  }
}

# 3. 使用 filesystem MCP 保存处理后的内容
POST /v1/skills/mcp.call_tool:invoke
{
  "input": {
    "server": "filesystem",
    "tool": "write_file",
    "arguments": {
      "path": "/data/courses/线性代数/readme.md",
      "content": "# 线性代数\n\n..."
    }
  }
}
```

### 3. GitHub 仓库管理

```bash
# 1. 使用 github MCP 拉取课程 README
POST /v1/skills/mcp.call_tool:invoke
{
  "input": {
    "server": "github",
    "tool": "read_file",
    "arguments": {
      "owner": "HITSZ-OpenAuto",
      "repo": "COMP1011",
      "path": "README.md"
    }
  }
}

# 2. 使用 pr.preview 预览修改
POST /v1/skills/pr.preview:invoke
{
  "input": {
    "campus": "shenzhen",
    "course_code": "COMP1011",
    "ops": [
      {
        "op": "add_lecturer_review",
        "lecturer_name": "Alice Smith",
        "content": "Great professor!"
      }
    ]
  }
}

# 3. 使用 github MCP 创建 PR
POST /v1/skills/mcp.call_tool:invoke
{
  "input": {
    "server": "github",
    "tool": "create_pull_request",
    "arguments": {
      "owner": "HITSZ-OpenAuto",
      "repo": "COMP1011",
      "title": "Add lecturer review",
      "body": "This PR adds a lecturer review..."
    }
  }
}
```

## Troubleshooting

### Browser-use MCP

**Issue:** Connection refused
**Solution:**
```bash
# Check if Docker container is running
docker ps | grep browser-use

# Check logs
docker logs browser-use

# Ensure port 7777 is accessible
curl http://localhost:7777
```

### Filesystem MCP

**Issue:** Permission denied
**Solution:**
```bash
# Ensure directory exists and has proper permissions
mkdir -p ./HITA_Project
chmod 755 ./HITA_Project

# Verify MCP server can access the directory
ls -la ./HITA_Project
```

### GitHub MCP

**Issue:** GitHub API authentication
**Solution:**
```bash
# Set GitHub token
export GITHUB_TOKEN="your-github-token-here"

# Or set in environment when starting MCP server
export GITHUB_TOKEN="your-github-token-here" \
  npx @modelcontextprotocol/server-github
```

### Brave Search MCP

**Issue:** API quota exceeded
**Solution:**
```bash
# Check API key status
# Visit: https://api.search.brave.com/app/dashboard

# Rotate API key if needed
export BRAVE_API_KEY="new-api-key-here"
```

## Security Considerations

1. **Filesystem Access**
   - Filesystem MCP should only access HITA_Project directory
   - Don't expose system paths outside project
   - Validate paths to prevent directory traversal

2. **GitHub Token**
   - Store in environment variables, not in config files
   - Use minimal permissions (read-only for most operations)
   - Rotate tokens regularly

3. **Browser Automation**
   - Browser-use MCP should only access allowed domains
   - Don't use for malicious scraping
   - Limit session duration

4. **API Keys**
   - Don't commit API keys to repository
   - Use environment variables
   - Monitor API usage and quotas

## Performance Optimization

1. **Tool Caching**
   - Cache frequently accessed files
   - Cache search results
   - Implement TTL for cache entries

2. **Connection Pooling**
   - Reuse HTTP connections to MCP servers
   - Implement connection timeouts
   - Handle connection failures gracefully

3. **Async Execution**
   - Long-running tools should be async skills
   - Use jobs system for background tasks
   - Implement progress reporting

## Next Steps

1. ✅ **Phase 1** - Filesystem + GitHub MCP
   - Implement basic file access
   - Implement repository management
   - Test end-to-end flows

2. 🔄 **Phase 2** - Browser + Fetch MCP
   - Set up Docker for browser-use
   - Implement web scraping capabilities
   - Test教务系统 login flow

3. 🚀 **Phase 3** - Brave Search + Advanced Features
   - Integrate Brave Search (optional)
   - Implement advanced browser automation
   - Optimize performance

4. 📊 **Phase 4** - Monitoring & Observability
   - Add Langfuse tracing for MCP calls
   - Implement MCP server health checks
   - Add metrics for tool execution
