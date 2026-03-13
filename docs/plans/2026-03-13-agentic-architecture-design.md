# Agentic Architecture SSOT (v0)

> **SSOT:** This document is the single source of truth for responsibilities, privacy boundaries, end-to-end flows, and contracts.
>
> **API Versioning:** URL versioning. This SSOT defines **/v1** contracts. Breaking changes require **/v2**.

**Goal:** Enable a multi-campus student assistant where:
- **Private data + EAS sessions** stay on-device.
- A **server-side public skills platform** provides high-concurrency PR automation, RAG, and public aggregations.
- A **client-side Orchestrator** runs Agentic workflows (ReAct/state machine), invoking local (private) and remote (public) skills.

---

## Section 1 — Responsibilities & Privacy Boundary

### Principle: Separation of agents

#### Client (HITA_L/HITA_X; future Web client)
**Role:** Orchestrator + Private Skills + Local memory.

**Client must do (v0):**
- Handle all **EAS auth/session** (WebView iVPN/WebVPN + cookie export; incoSpringBoot bearer+cookie; etc.).
- Execute any tool requiring **personal credentials** or **campus intranet access tied to user identity**.
- Run **ReAct / state machine** locally.
- Persist local memory and caches:
  - default TTL: user-configurable (e.g., 7 days)
  - snapshots for diffs/notifications/widgets

**Client must NOT do (v0):**
- Send any P0 secrets (passwords, cookies, tokens, reusable sessions) to the server.

#### Server (agent-backend, Go)
**Role:** Public Skills only.

**Server must do (v0):**
- Provide **skill registry + invocation protocol + async jobs**.
- Execute **public compute** skills (PR/RAG/search) with strict log/trace scrubbing.

**Explicitly forbidden on server (v0):**
- Capturing or persisting user credentials, cookies, tokens, or EAS session data.
- Running private EAS fetches or campus-only scraping tied to user identity.

#### Planner (LLM decision)
- Planner can run client-side or cloud-side.
- **Any step requiring sensitive data must execute only on client-side tools.**

### Data classification & handling

- **Private (P0; never leaves device):** credentials, session cookies/tokens, student ID, grades, personal timetable.
- **Derived Local:** normalized schemas produced by private skills; stored locally with TTL.
- **Public (P2):** course materials metadata, PR submissions, public search results, RAG indexes without private data.
- **Telemetry/Tracing:** allowed only in redacted form (no IDs, names, tokens, cookies; no request/response bodies).

#### Revised Addendum (approved)
- **Derived Local 可选上云：**默认不上传；仅在用户显式开启时允许上传脱敏摘要（P1），禁止上传原始结构化成绩/课表（P0）。
- **Public Skills 输入仍可能含隐私：**服务端按“敏感输入”处理（默认不记录 body、不入 trace），并做基础敏感词/格式检测（至少 warn）。
- **Public-but-Intranet 资源：**校内网但不绑定个人身份的公共资源，归类为 Public-but-Intranet，允许服务端通过自有 VPN/代理抓取并作为 Public Skills 提供（仍需脱敏与日志约束）。

---

## Section 2 — End-to-End Data Flow (incl. TTL & cache semantics)

### Flow A: 刷新成绩（含 TTL 策略）

**Trigger sources (v0):**
- user manual refresh
- background worker refresh
- term change / session change triggers refresh

**Flow:**
1) 用户请求“刷新成绩”。端侧 Orchestrator 读取本地 `UnifiedScoreResult`（若存在）。
2) 若缓存未过期（`now < expires_at`）且非强制刷新：
   - 直接返回缓存结果
   - 写入“cache hit”事件（本地，仅用于调试/统计）
3) 若缓存过期或用户强制刷新：
   - 端侧调用私有 EAS Skill：`eas.scores.fetch`
   - 得到标准化 `UnifiedScoreItem[]`
   - 写入本地持久化：更新 `cached_at` 与 `expires_at`
   - 触发本地通知/小组件刷新（基于 diff）
4) 若用户开启“可选上云摘要”：
   - 仅上传脱敏摘要（P1）到 server（默认关闭）

**TTL definition (write this in client):**
- TTL 基于 `cached_at + ttl_seconds` 计算得到 `expires_at`（不依赖 lastValidatedAt）。

**Failure strategy (v0):**
- fetchScores 失败时：
  - 若存在历史缓存：返回 `stale=true` 的旧数据 + `error`（并记录失败次数，避免无限重试）
  - 若无缓存：返回 `stale=true` + 空 data + `error`

**Flow A return structure (write this shape as SSOT):**
```json
{
  "data": [
    {
      "campus_id": "HITSZ|HITH|HITWH",
      "term_id": "string",
      "course_id": "string",
      "course_name": "string",
      "score_value": 90,
      "score_text": "A",
      "status": "normal",
      "credit": 3.0
    }
  ],
  "stale": false,
  "cached_at": "2026-03-13T00:00:00Z",
  "expires_at": "2026-03-20T00:00:00Z",
  "source": "cache|network",
  "error": null
}
```

### Flow B: 提交 PR（公共能力）
1) 端侧 Orchestrator 组装 PR payload（不含个人凭证）。
2) 调用 server public skill（异步）：`hoa.courses.submit_ops`。
3) Server 对输入做敏感检测（warn）；默认不记录 body；不入 trace。
4) Server 返回 `job_id`；端侧按退避策略轮询 `GET /v1/jobs/{job_id}`。
5) 端侧拿到结果摘要并本地存档（含 PR URL）。

**Job polling backoff (v0):**
- 0.5s → 1s → 2s → 5s (cap 5s), max 2 minutes.

### Flow C: RAG 查询（公共能力）
1) 端侧判断查询需要公共知识：调用 `rag.query`。
2) Server 返回标准化 hits。
3) 端侧合成答案并本地存档摘要（Derived Local）。

---

## Section 3 — Contracts (v0)

### 3.1 Common (跨端通用)

#### 3.1.1 Trace (字段白名单 + 默认不入 Langfuse)
- `trace` 为可选对象，仅允许白名单字段；服务端必须丢弃未知字段。
- 默认不进入 Langfuse；只有 `trace.loggable=true` 时才允许记录**元数据**（不含请求体/输出体）。
- 禁止字段：任何 P0/P1 个人数据、cookie/token、学号/姓名、原始课表/成绩结构。

**Trace Schema (v0)**
```json
{
  "trace": {
    "trace_id": "string",
    "parent_id": "string",
    "request_id": "string",
    "loggable": false,
    "ttl_hint_seconds": 21600,
    "client": {
      "app": "string",
      "version": "string",
      "platform": "android|ios|web|server"
    },
    "tags": ["string"]
  }
}
```

#### 3.1.2 Error (最小字段)
```json
{
  "error": {
    "code": "string",
    "message": "string",
    "retryable": true
  }
}
```

#### 3.1.3 时间字段命名与格式
- 所有 `*_at` 与 `expires_at` 为 RFC3339 UTC。
- `*_date` 允许使用 `YYYY-MM-DD`（date-only 例外）。

---

### 3.2 Server: Skill Platform (/v1)

#### 3.2.1 `GET /v1/skills`
**Response**
```json
{
  "skills": [
    {
      "name": "rag.query",
      "is_async": false,
      "description": "string",
      "input_schema": {},
      "output_schema": {}
    }
  ]
}
```
- `input_schema` / `output_schema` v0 可为空对象，但字段必须保留（便于迁移 OpenAPI/JSON Schema）。

#### 3.2.2 `POST /v1/skills/{name}:invoke`

**Request**
```json
{
  "input": {},
  "trace": {}
}
```

**Sync Response (HTTP 200)**
```json
{
  "ok": true,
  "output": {}
}
```

**Async Response (HTTP 200)**
```json
{
  "ok": true,
  "job_id": "string"
}
```

**Failure Response (HTTP 200)**
```json
{
  "ok": false,
  "error": { "code": "string", "message": "string", "retryable": false }
}
```

**Rules (write this as enforcement):**
- **invoke 永远 200**（成功/失败都 200）。
- **禁止** `POST /v1/skills/{name}:invoke` 返回 404/400。
- skill 不存在也必须返回 200 + `ok:false` + `error.code="SKILL_NOT_FOUND"`。
- `ok` 必须存在；失败必须带 `error`。

#### 3.2.3 `GET /v1/jobs/{job_id}`

**Found (HTTP 200)**
```json
{
  "ok": true,
  "job": {
    "id": "string",
    "status": "queued|running|succeeded|failed",
    "skill_name": "string",
    "input_json": null,
    "output_json": null,
    "error": null,
    "created_at": "2026-03-13T00:00:00Z",
    "updated_at": "2026-03-13T00:00:00Z"
  }
}
```

**Not Found (HTTP 404)**
- 推荐返回 body：
```json
{
  "ok": false,
  "error": { "code": "JOB_NOT_FOUND", "message": "string", "retryable": false }
}
```

**Server Error (HTTP 500)**
- 可返回 error，但**不得包含敏感 input_json**。

**Field rules (must implement):**
- `status` v0 固定为 `queued|running|succeeded|failed`。
- `input_json` / `output_json` 的 JSON 类型为 **any JSON**（示例中用 `null` 仅表示当前状态为空）。
- `output_json` 仅在 `succeeded` 时非空，否则为 `null`。
- `error` 仅在 `failed` 时非空，否则为 `null`。
- `input_json` 可在 DB 内持久化，但**日志/trace 永不记录**；服务端可配置仅存 hash/size（v1 扩展）。

---

### 3.3 Public Skills（服务端执行）

#### 3.3.1 `rag.query`
**Input**
```json
{
  "query": "string",
  "top_k": 10,
  "filters": {}
}
```

**Output**
```json
{
  "hits": [
    {
      "doc_id": "string",
      "chunk_id": "string",
      "title": "string",
      "url": "string",
      "snippet": "string",
      "score": 0.0,
      "source": "string"
    }
  ]
}
```
- `hits[*].doc_id` 与 `snippet` 必须稳定。
- `chunk_id/title/url/source` 为可选字段但字段名固定。

#### 3.3.2 HOA PR skills（v0 最小 I/O）

**hoa.courses.lookup**
Input
```json
{
  "query": "string",
  "campus_id": "HITSZ|HITH|HITWH"
}
```
Output
```json
{
  "courses": [
    {
      "course_id": "string",
      "name": "string",
      "campus_id": "HITSZ|HITH|HITWH",
      "term_id": "string",
      "url": "string"
    }
  ]
}
```

**hoa.courses.submit_ops (async)**
Input
```json
{
  "campus_id": "HITSZ|HITH|HITWH",
  "items": [
    {
      "course_id": "string",
      "name": "string",
      "action": "create|update|delete",
      "payload": {}
    }
  ]
}
```
Output
- 异步：返回 `job_id`。

**hoa.requests.get_status**
Input
```json
{
  "job_id": "string"
}
```
Output
```json
{
  "job": { "id": "string", "status": "queued|running|succeeded|failed" }
}
```
- v0 可作为 `/v1/jobs/{id}` 的兼容封装。

---

### 3.4 Private Schemas（端侧执行，仅结构定义）

#### CampusSession
```json
{
  "campus_id": "HITSZ|HITH|HITWH",
  "cookies_by_host": {
    "host.example.edu": {
      "cookie_name": "cookie_value"
    }
  },
  "bearer_token": "string",
  "created_at": "2026-03-13T00:00:00Z",
  "updated_at": "2026-03-13T00:00:00Z",
  "last_validated_at": "2026-03-13T00:00:00Z"
}
```

#### UnifiedTerm
```json
{
  "campus_id": "HITSZ|HITH|HITWH",
  "term_id": "string",
  "name": "string",
  "start_date": "2026-03-13",
  "end_date": "2026-07-01"
}
```

#### UnifiedCourseItem (最小字段)
```json
{
  "campus_id": "HITSZ|HITH|HITWH",
  "term_id": "string",
  "course_id": "string",
  "course_name": "string",
  "teacher": "string",
  "location": "string",
  "weekday": 1,
  "start_period": 1,
  "end_period": 2,
  "weeks": [1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16],
  "weeks_text": "1-16"
}
```
- `weekday` 取值 1-7（周一=1）。
- `end_period` **inclusive**。
- `weeks_text` 可选，仅用于 UI。

#### UnifiedScoreItem (最小字段)
```json
{
  "campus_id": "HITSZ|HITH|HITWH",
  "term_id": "string",
  "course_id": "string",
  "course_name": "string",
  "score_value": 90,
  "score_text": "A",
  "status": "normal",
  "credit": 3.0
}
```
- `score_value` 允许为 `null`。
- `score_text` 必填。
- `status` 可选，建议枚举 `normal|pass|fail|absent|deferred`。

#### UnifiedScoreResult (与 Flow A 对齐)
```json
{
  "data": [ { "course_id": "string", "score_value": 90 } ],
  "stale": false,
  "cached_at": "2026-03-13T00:00:00Z",
  "expires_at": "2026-03-20T00:00:00Z",
  "source": "cache|network",
  "error": null
}
```
- `error` 成功时为 `null`；失败时为 Error 对象。

---

### 3.5 Minimal Consistency Constraints (v0)
- 字段命名必须为 `snake_case`，新增字段只能 **additive**。
- **禁止同义字段并存**：本 SSOT 选定字段名后（例如 `weekday`），不得在其他位置改写为 `dow/day_of_week`。
- `ok` 永远存在；失败必须包含 `error`。
- `is_async=true` 的 skill，`invoke` 一律返回 `job_id`，最终结果只从 `/v1/jobs/{id}` 获取。
