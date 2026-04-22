# 远端环境变量模板与部署检查（2026-04-22）

## 已落地文件
- 配置模板: config/agent-backend.env.example
- 运行配置: config/agent-backend.env

## 已应用的关键改动
1. 启动脚本已改为自动加载 `config/agent-backend.env`
2. 后端 MCP 路径已改为服务器路径/可配置，不再写死 `/Users/...`
3. 服务已重启并生效

## 当前运行配置（关键项）
- PR_SERVER_URL=http://47.115.160.70:8081
- QDRANT_URL=http://localhost:6333
- QDRANT_COLLECTION=hit-courses
- EMBEDDING_PROVIDER=bigmodel
- BRAVE_API_KEY=（当前为空）

## 接口回归结果

### 1) 课程搜索
- 请求: `POST /api/courses/search`
- 结果: ✅ 已恢复
- 说明: 返回课程列表，说明 PR_SERVER_URL 配置已生效

### 2) RAG 查询
- 请求: `POST /api/rag/query`
- 结果: ❌ 未恢复
- 当前错误: `embedding provider config error: BIGMODEL_API_KEY is required`
- 说明: `QDRANT_URL` 已配置，但 Embedding 密钥缺失，流程在嵌入阶段失败

### 3) 教师搜索
- 请求: `POST /api/teachers/search`
- 结果: ❌ 未恢复
- 当前错误: `crawl4ai MCP server not registered`
- 说明: brave/unstructured 已成功注册；crawl4ai 仍未注册成功，需要单独排查该 MCP server 运行条件与协议

## 你提供的模板（整理版）

```env
# PR Server
PR_SERVER_URL=http://47.115.160.70:8081
PR_SERVER_TOKEN=

# Brave MCP（如果远端启动本地 Brave MCP）
BRAVE_MCP_URL=http://localhost:8001
BRAVE_API_KEY=YOUR_BRAVE_API_KEY
BRAVE_ANSWER_API_KEY=YOUR_BRAVE_ANSWER_API_KEY

# 代理（如需要）
HTTP_PROXY=http://YOUR_PROXY_IP:PORT
HTTPS_PROXY=http://YOUR_PROXY_IP:PORT

# Qdrant
QDRANT_URL=http://localhost:6333
QDRANT_COLLECTION=hit-courses

# Embedding
EMBEDDING_PROVIDER=bigmodel
EMBEDDING_API_KEY=YOUR_BIGMODEL_KEY
EMBEDDING_MODEL=embedding-3

# LLM
MINIMAX_BASE_URL=https://api.minimaxi.com/v1
MINIMAX_API_KEY=YOUR_MINIMAX_KEY
BIGMODEL_API_KEY=YOUR_BIGMODEL_KEY

# HITSZ（可选）
HITSZ_USERNAME=YOUR_ACCOUNT
HITSZ_PASSWORD=YOUR_PASSWORD

# Database
DB_PATH=./data/jobs.db

# Server
WORKER_COUNT=4
```

## 部署检查清单
1. Qdrant 启动
```bash
docker run -d --name qdrant -p 6333:6333 qdrant/qdrant
```
2. 配置 BIGMODEL_API_KEY（或 EMBEDDING_API_KEY）
3. 重启服务并回归
```bash
systemctl restart agent-backend
curl -s http://127.0.0.1:8080/health
curl -s -X POST http://127.0.0.1:8080/api/courses/search -H 'Content-Type: application/json' -d '{"keyword":"高等数学"}'
curl -s -X POST http://127.0.0.1:8080/api/rag/query -H 'Content-Type: application/json' -d '{"query":"什么是RAG","top_k":3}'
curl -s -X POST http://127.0.0.1:8080/api/teachers/search -H 'Content-Type: application/json' -d '{"name":"秦阳"}'
```
