# Agent Backend 自检报告（2026-04-22）

## 结论摘要
- ❌ 课程搜索存在后端配置问题（PR Server 配置缺失/错误）
- ❌ RAG 查询存在后端配置问题（QDRANT_URL 未配置）
- ⚠️ 教师搜索当前主故障不是 `teacher.name` 为空，而是 MCP 未注册；在该故障修复前，无法有效验证 `teacher.name` 数据质量

## 自检范围与方法
- 检查对象：本机运行中的 `agent-backend`（systemd 服务）
- 检查方式：
  - 直接调用 API
  - 检查进程环境变量
  - 检查服务日志
  - 对照代码路径行为

## 详细结果

### 1) 课程搜索（/api/courses/search）

#### 复现请求
```bash
curl -s -X POST http://127.0.0.1:8080/api/courses/search \
  -H 'Content-Type: application/json' \
  -d '{"keyword":"高等数学","campus":"shenzhen","limit":3}'
```

#### 实际返回
```json
{"error":{"code":"INTERNAL","message":"decode response: json: cannot unmarshal number into Go value of type map[string]interface {}","retryable":false},"ok":false}
```

#### 证据与根因
- 代码中 `getPRServerBaseURL()` 在 `PR_SERVER_URL` 未设置时，默认返回 `http://localhost:8080`。
- 课程 skill 随后请求 `POST /v1/courses:search`（即 `http://localhost:8080/v1/courses:search`）。
- 该路径在当前服务不存在，直接请求返回：
  - `HTTP/1.1 404 Not Found`
  - body: `404 page not found`
- JSON 解码器把 `404 ...` 的开头数字按 number 解析，导致 `cannot unmarshal number into map`。

#### 结论
- ❌ 后端配置问题成立：PR Server 未正确配置（或未接入），导致课程搜索失败。

#### 修复建议
- 设置正确的 `PR_SERVER_URL`（如真实 pr-server 地址），并在启动服务时注入环境变量。
- 若需要鉴权，配置 `PR_SERVER_TOKEN`。
- 可在课程 skill 增加对非 JSON 响应的兜底错误信息（避免误导）。

---

### 2) RAG 查询（/api/rag/query）

#### 复现请求
```bash
curl -s -X POST http://127.0.0.1:8080/api/rag/query \
  -H 'Content-Type: application/json' \
  -d '{"query":"什么是RAG","top_k":3}'
```

#### 实际返回
```json
{"error":{"code":"INTERNAL","message":"qdrant config error: QDRANT_URL is required","retryable":false},"ok":false}
```

#### 结论
- ❌ 后端配置问题成立：`QDRANT_URL` 缺失，RAG 查询不可用。

#### 修复建议
- 配置 `QDRANT_URL`（以及对应 collection 配置）。
- 确保向量库服务可达，再回归 `POST /api/rag/query`。

---

### 3) 教师搜索（/api/teachers/search）

#### 复现请求
```bash
curl -s -X POST http://127.0.0.1:8080/api/teachers/search \
  -H 'Content-Type: application/json' \
  -d '{"name":"秦阳"}'
```

#### 实际返回
```json
{"error":{"code":"NOT_FOUND","message":"crawl4ai MCP server not registered","retryable":false},"ok":false}
```

#### 日志证据
- 服务启动日志显示：
  - `.env` 未加载（当前目录和 `bin/.env` 都不存在）
  - MCP 注册命令使用了本地开发机路径：
    - `/Users/jiaoziang/workspace/agent-backend/mcp-servers/crawl4ai/server.py`
    - `/Users/jiaoziang/workspace/agent-backend/mcp-servers/brave/server.py`
    - `/Users/jiaoziang/workspace/agent-backend/mcp-servers/unstructured/server.py`
  - 这些路径在服务器不存在，导致 `python3: can't open file ...`，最终 `MCP registration failed`

#### 结论
- ⚠️ 你提出的“`teacher.name` 为空”目前无法作为主根因成立；当前请求在 MCP 未注册处就失败，尚未进入稳定的数据抽取阶段。
- ✅ 可确认的后端问题是：MCP 路径硬编码错误，导致教师搜索依赖链失效。

#### 修复建议
- 将 `cmd/server/main.go` 中 MCP 脚本路径改为服务器实际路径（如 `/root/agent-backend/mcp-servers/...`），或改为可配置环境变量。
- 修复后再复测教师搜索，再判断是否存在 `teacher.name` 数据抽取为空的问题。

## 优先级建议
1. P0：修复 MCP 路径配置（影响教师搜索、爬虫、文档解析等多条链路）
2. P0：配置 `PR_SERVER_URL`（恢复课程/PR 相关能力）
3. P0：配置 `QDRANT_URL`（恢复 RAG 查询）
4. P1：课程 skill 错误处理增强（区分 404 非 JSON 与业务 JSON）
5. P1：教师数据抽取质量回归（在 MCP 可用后验证 `name/title/department`）
