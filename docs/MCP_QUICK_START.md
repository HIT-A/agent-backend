# MCP Servers 集成说明

## 快速开始

### 方式 1: 使用快速启动脚本（推荐）

```bash
# 进入项目目录
cd hoa-agent-backend

# 运行快速启动脚本（开发模式）
./scripts/quick_start_mcp.sh --dev

# 或者运行完整模式（Docker Compose）
./scripts/quick_start_mcp.sh
```

### 方式 2: 手动启动

```bash
# 1. 创建 HITA_Project 目录
mkdir -p ./HITA_Project

# 2. 设置环境变量
export MCP_SERVERS=$(cat config/mcp_servers_recommended.json)
export GITHUB_TOKEN="your-github-token"  # 可选
export BRAVE_API_KEY="your-api-key"     # 可选

# 3. 启动 agent-backend
go run cmd/server/main.go

# 或者使用 Docker Compose
docker-compose -f docker-compose.mcp.yml up -d
```

## MCP 服务器列表

| MCP 服务器 | 功能 | 状态 | 说明 |
|-----------|------|------|------|
| filesystem | 本地文件访问 | ✅ 启用 | 读取/写入 HITA_Project 目录 |
| github | GitHub 仓库管理 | ✅ 启用 | 读取文件、创建 PR |
| fetch | 网页转 Markdown | ✅ 启用 | CSDN、新闻等 |
| browser-use | 浏览器自动化 | ⚠️ 需要配置 | 教务系统登录、抓取 |
| brave-search | 外部搜索 | ⚠️ 需要配置 | 需要 API Key |
| qdrant-mcp | 向量搜索 | ⚠️ 已有 rag.query | 可选的替代方案 |

## 使用示例

### 1. 文件系统操作

```bash
# 读取文件
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "filesystem",
      "tool": "read_file",
      "arguments": {
        "path": "/data/test.txt"
      }
    }
  }'

# 写入文件
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "filesystem",
      "tool": "write_file",
      "arguments": {
        "path": "/data/output.txt",
        "content": "Hello, HITA!"
      }
    }
  }'

# 列出目录
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "filesystem",
      "tool": "list_directory",
      "arguments": {
        "path": "/data"
      }
    }
  }'
```

### 2. GitHub 仓库操作

```bash
# 读取文件
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "github",
      "tool": "read_file",
      "arguments": {
        "owner": "HITSZ-OpenAuto",
        "repo": "COMP1011",
        "path": "README.md"
      }
    }
  }'

# 创建 PR
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "github",
      "tool": "create_pull_request",
      "arguments": {
        "owner": "HITSZ-OpenAuto",
        "repo": "COMP1011",
        "title": "Update course materials",
        "body": "This PR updates course materials..."
      }
    }
  }'
```

### 3. 网页内容获取

```bash
# 获取 CSDN 博客内容
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "fetch",
      "tool": "fetch",
      "arguments": {
        "url": "https://blog.csdn.net/xxx/article/details/xxx"
      }
    }
  }'

# 保存到本地文件
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "filesystem",
      "tool": "write_file",
      "arguments": {
        "path": "/data/courses/csdn-post.md",
        "content": "CSDN 博客内容..."
      }
    }
  }'
```

### 4. 浏览器自动化

```bash
# 首先启动 browser-use MCP
docker run -d -p 7777:7777 browser-use/browser-use:latest

# 注册到 agent-backend
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "name": "browser-use",
      "transport": "http",
      "url": "http://localhost:7777"
    }
  }'

# 打开教务系统
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "browser-use",
      "tool": "navigate",
      "arguments": {
        "url": "https://教务系统域名/login"
      }
    }
  }'

# 填写用户名
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "browser-use",
      "tool": "fill_form",
      "arguments": {
        "selector": "#username",
        "value": "your-username"
      }
    }
  }'

# 点击登录按钮
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "browser-use",
      "tool": "click",
      "arguments": {
        "selector": "#login-button"
      }
    }
  }'

# 截图
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "browser-use",
      "tool": "screenshot",
      "arguments": {
        "path": "/data/screenshots/login.png"
      }
    }
  }'
```

### 5. 外部搜索（Brave）

```bash
# 首先获取 API Key
# 访问: https://api.search.brave.com/app/dashboard

# 设置环境变量
export BRAVE_API_KEY="your-api-key-here"

# 搜索
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "brave-search",
      "tool": "search",
      "arguments": {
        "query": "哈工大 线性代数 课程大纲"
        "count": 10
      }
    }
  }'
```

## 端侧集成示例

### 端侧 Orchestrator 使用流程

```javascript
// 1. 列出可用的 MCP 工具
const tools = await invoke('mcp.list_tools', {});

// 2. 使用 filesystem 读取本地配置
const config = await invoke('mcp.call_tool', {
  server: 'filesystem',
  tool: 'read_file',
  arguments: { path: '/data/config.json' }
});

// 3. 使用 browser-use 登录教务系统
await invoke('mcp.call_tool', {
  server: 'browser-use',
  tool: 'navigate',
  arguments: { url: 'https://教务系统/login' }
});

await invoke('mcp.call_tool', {
  server: 'browser-use',
  tool: 'fill_form',
  arguments: { selector: '#username', value: config.username }
});

await invoke('mcp.call_tool', {
  server: 'browser-use',
  tool: 'click',
  arguments: { selector: '#login-button' }
});

// 4. 保存登录凭证到本地
await invoke('mcp.call_tool', {
  server: 'filesystem',
  tool: 'write_file',
  arguments: {
    path: '/data/credentials.json',
    content: JSON.stringify({ username: config.username, token: '...' })
  }
});
```

## 故障排除

### Filesystem MCP 问题

**问题：** Permission denied
**解决：**
```bash
# 检查目录权限
ls -la ./HITA_Project

# 修改权限
chmod 755 ./HITA_Project
```

### GitHub MCP 问题

**问题：** GitHub API authentication failed
**解决：**
```bash
# 检查 token
echo $GITHUB_TOKEN

# 重新设置
export GITHUB_TOKEN="your-new-token"
```

### Browser-use MCP 问题

**问题：** Connection refused
**解决：**
```bash
# 检查 Docker 容器
docker ps | grep browser-use

# 查看日志
docker logs browser-use

# 检查端口
curl http://localhost:7777
```

### Brave Search MCP 问题

**问题：** API quota exceeded
**解决：**
```bash
# 访问 dashboard 检查使用情况
# https://api.search.brave.com/app/dashboard

# 更换 API Key
export BRAVE_API_KEY="new-api-key"
```

## 配置选项

### 启用所有 MCP 服务器

```bash
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
  },
  {
    "name": "fetch",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-fetch"],
    "enabled": true
  },
  {
    "name": "browser-use",
    "transport": "http",
    "url": "http://localhost:7777",
    "enabled": false
  },
  {
    "name": "brave-search",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-brave-search"],
    "enabled": false
  }
]'
```

### 仅启用基础 MCP 服务器

```bash
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
  },
  {
    "name": "fetch",
    "transport": "stdio",
    "command": ["npx", "-y", "@modelcontextprotocol/server-fetch"],
    "enabled": true
  }
]'
```

## 监控和日志

### 查看 MCP 服务器状态

```bash
# 列出所有已注册的服务器
curl -X POST http://localhost:8080/v1/skills/mcp.list_servers:invoke \
  -H "Content-Type: application/json" \
  -d '{"input": {}}'

# 查看每个服务器的工具
curl -X POST http://localhost:8080/v1/skills/mcp.list_tools:invoke \
  -H "Content-Type: application/json" \
  -d '{"input": {}}'
```

### Langfuse 集成

MCP 工具调用可以通过 Langfuse 进行追踪：

```javascript
// 端侧调用
const trace = {
  mcp_server: 'filesystem',
  tool: 'read_file',
  arguments: { path: '/data/test.txt' }
};

const result = await invoke('mcp.call_tool', { input, trace });
```

## 安全注意事项

1. **文件系统访问**
   - Filesystem MCP 只能访问 HITA_Project 目录
   - 验证路径以防止目录遍历
   - 不暴露系统路径

2. **GitHub Token**
   - 存储在环境变量中，不在配置文件中
   - 使用最小权限（只读用于大多数操作）
   - 定期轮换 token

3. **浏览器自动化**
   - Browser-use MCP 只访问允许的域名
   - 不用于恶意抓取
   - 限制会话持续时间

4. **API Keys**
   - 不要将 API keys 提交到仓库
   - 使用环境变量
   - 监控 API 使用和配额

## 下一步

1. ✅ **测试基础 MCP 服务器**（filesystem, github, fetch）
2. 🔄 **配置 browser-use MCP**（Docker + 教务系统测试）
3. 🚀 **集成 Brave Search**（获取 API Key）
4. 📊 **添加 Langfuse 追踪**（监控 MCP 调用）
5. 🎯 **优化性能**（缓存、连接池）

## 文档

- [MCP_INTEGRATION_DESIGN.md](MCP_INTEGRATION_DESIGN.md) - MCP 集成设计
- [MCP_USAGE.md](MCP_USAGE.md) - MCP 使用指南
- [MCP_README.md](MCP_README.md) - MCP 集成说明
- [MCP_SERVERS_INTEGRATION.md](MCP_SERVERS_INTEGRATION.md) - MCP 服务器集成计划

## 支持

遇到问题？

1. 查看故障排除部分
2. 查看 GitHub Issues
3. 查看文档
4. 联系开发团队
