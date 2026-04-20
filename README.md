# Agent Backend v2

## 已配置的服务

### 必需服务
| 服务 | 状态 | 配置 |
|------|------|------|
| PR Server (data-backend) | ✅ | 47.115.160.70:8081 |
| COS (腾讯云对象存储) | ✅ | APPID: 1410950200 |

### 可选MCP服务
需要额外安装配置：

#### 1. Brave Search MCP (已配置API Key)
```bash
# 直接运行
npx @brave/brave-search-mcp-server --transport stdio

# 或HTTP模式
npx @brave/brave-search-mcp-server --transport http --port 8001
```
- API Key: 已配置
- 功能: 网页搜索、图片搜索、视频搜索、新闻搜索

#### 2. Crawl4AI MCP
```bash
# Docker方式
docker run -d -p 11235:11235 --name crawl4ai unclecode/crawl4ai:latest

# 或使用Python
pip install crawl4ai
python -m crawl4ai.server --port 11235
```

## 快速启动

```bash
cd /Users/jiaoziang/workspace/agent-backend

# 加载配置
export $(cat .env | xargs)

# 启动服务
go run ./cmd/server
```

## 完整 API 端点

### 系统
| 方法 | 路由 | 说明 |
|------|------|------|
| GET | `/health` | 健康检查 |

### 课表 API
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/courses/search` | 搜索课程（关键词 + 校区） |
| POST | `/api/courses/read` | 读取课程详情 |

### PR API
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/pr/preview` | PR 作业预览 |
| POST | `/api/pr/submit` | PR 作业提交 |
| POST | `/api/pr/lookup` | PR 作业查询 |

### 文件 API (COS)
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/files/upload` | 上传文件到 COS（仅管理员） |
| POST | `/api/files/download` | 从 COS 下载文件 |
| POST | `/api/files/list` | 列出 COS 文件 |
| POST | `/api/files/delete` | 删除 COS 文件 |

### RAG API
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/rag/query` | 向量数据库查询 |
| POST | `/api/rag/ingest` | RAG 内容摄入 |

### 爬虫 API
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/crawl/page` | 爬取单个页面 |
| POST | `/api/crawl/site` | 爬取整个站点 |
| POST | `/api/crawl/status` | 爬取状态查询 |

### 搜索 API
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/search/brave` | Brave 网页搜索 |

### 教师 API
| 方法 | 路由 | 说明 |
|------|------|------|------|
| POST | `/api/teachers/search` | 搜索教师信息 |
| POST | `/api/teachers/batch` | 批量搜索教师 |

### HITSZ API
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/hitsz/fetch` | 抓取 HITSZ 公开页面 |

### AI API
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/ai/chat` | 简单对话 |
| POST | `/api/ai/react` | ReAct 多步推理对话（调用工具） |

### 临时文件 API
| 方法 | 路由 | 说明 |
|------|------|------|
| POST | `/api/temp/upload` | 上传临时文件（24h 后删除） |
| GET | `/api/temp/download/{id}` | 下载临时文件 |
| GET | `/api/temp/list` | 列出临时文件 |

---

## API 详细说明

### 搜索课程
```bash
POST /api/courses/search
Body: {"keyword": "高等数学", "campus": "shenzhen", "limit": 10}
```

### 课表详情
```bash
POST /api/courses/read
Body: {"campus": "shenzhen", "course_code": "MA21003"}
```

### Brave 搜索
```bash
POST /api/search/brave
Body: {"query": "哈尔滨工业大学深圳", "count": 5}
```

### AI 对话
```bash
POST /api/ai/chat
Body: {"message": "你好", "system": "你是一个智能助手"}
```

### AI ReAct 对话
```bash
POST /api/ai/react
Body: {"message": "帮我搜索明天的高等数学课程"}
```

### 临时文件上传 (JSON Base64)
```bash
POST /api/temp/upload
Body: {
  "name": "test.pdf",
  "mime_type": "application/pdf",
  "content_base64": "JVBERi0x..."
}
```

### HITSZ 抓取
```bash
POST /api/hitsz/fetch
Body: {"url": "http://info.hitsz.edu.cn/..."}
```

---

## 环境变量说明

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PR_SERVER_URL` | Data Backend PR Server 地址 | `http://localhost:8081` |
| `PR_SERVER_TOKEN` | PR Server 鉴权 Token | 空（无鉴权） |
| `COS_SECRET_ID` | 腾讯云 COS SecretID | 已配置 |
| `COS_SECRET_KEY` | 腾讯云 COS SecretKey | 已配置 |
| `COS_REGION` | COS 区域 | `ap-guangzhou` |
| `COS_BUCKET` | COS 存储桶 | `hita-courses` |
| `QDRANT_URL` | Qdrant 向量数据库地址 | `http://localhost:6333` |
| `QDRANT_COLLECTION` | Qdrant Collection 名称 | `hita-docs` |
| `HITSZ_FETCHER_PATH` | hitsz-info-fetcher 二进制路径 | `./bin/hitsz-info-fetcher` |
| `DB_PATH` | SQLite 数据库路径 | `./data/jobs.db` |
| `WORKER_COUNT` | 异步任务工作线程数 | `4` |
| `BRAVE_API_KEY` | Brave Search API Key | 已配置 |
| `TEMP_DIR` | 临时文件存储目录 | `./data/temp/` |

## 项目结构

```
agent-backend/
├── cmd/server/main.go          # 服务入口，MCP 初始化
├── internal/
│   ├── hitsz/fetcher.go        # HITSZ SSO 封装
│   ├── httpserver/             # HTTP 路由
│   │   ├── routes.go           # 主路由注册
│   │   ├── courses.go          # 课表 API
│   │   ├── pr.go              # PR API
│   │   ├── files.go           # COS 文件 API
│   │   ├── rag.go             # RAG API
│   │   ├── crawl.go           # 爬虫 API
│   │   ├── search.go          # 搜索 API (Brave)
│   │   ├── teachers.go        # 教师 API
│   │   ├── hitsz.go           # HITSZ API
│   │   ├── ai.go              # AI 对话 API
│   │   └── temp_files.go      # 临时文件 API
│   ├── skills/                 # Skill 实现层
│   │   └── aggregator_search.go # 统一搜索（Brave/RAG/Arxiv/GitHub）
│   ├── mcp/                    # MCP 客户端
│   │   ├── registry.go        # MCP 服务器注册表
│   │   ├── client.go          # MCP 客户端封装
│   │   └── transport.go       # stdio/http 传输层
│   ├── cos/                    # COS 存储
│   ├── tempstore/              # 本地临时文件存储（24h TTL）
│   └── jobs/                   # 异步任务
├── .env                        # 环境变量配置
└── go.mod                      # Go 模块
```

## 测试命令

```bash
# 健康检查
curl http://localhost:8080/health

# 搜索课程
curl -X POST http://localhost:8080/api/courses/search \
  -H "Content-Type: application/json" \
  -d '{"keyword": "数学", "campus": "shenzhen"}'

# Brave 搜索
curl -X POST http://localhost:8080/api/search/brave \
  -H "Content-Type: application/json" \
  -d '{"query": "哈尔滨工业大学深圳", "count": 5}'

# AI 对话
curl -X POST http://localhost:8080/api/ai/chat \
  -H "Content-Type: application/json" \
  -d '{"message": "你好"}'

# 临时文件上传
curl -X POST http://localhost:8080/api/temp/upload \
  -H "Content-Type: application/json" \
  -d '{"name": "test.txt", "mime_type": "text/plain", "content_base64": "aGVsbG8gd29ybGQ="}'
```
