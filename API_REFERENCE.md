# Agent-Backend API Reference

**Base URL**: `http://localhost:8080` (or your configured server URL)

**Content-Type**: All endpoints expect `application/json` unless otherwise specified.

---

## Table of Contents

- [Health Check](#health-check)
- [Courses API](#courses-api)
- [PR API](#pr-api)
- [Files API](#files-api)
- [RAG API](#rag-api)
- [Crawl API](#crawl-api)
- [Search API](#search-api)
- [Teachers API](#teachers-api)
- [HITSZ API](#hitsz-api)
- [AI API](#ai-api)
- [Temp Files API](#temp-files-api)

---

## Health Check

### GET /health

Check if the server is running.

**Response:**
```json
{
  "status": "ok"
}
```

**Curl Example:**
```bash
curl http://localhost:8080/health
```

---

## Courses API

### POST /api/courses/search

Search for courses by keyword.

**Request Body:**
```json
{
  "keyword": "string (required) - Search keyword",
  "campus": "string (optional) - shenzhen|weihai|harbin. Empty means all campuses",
  "limit": 10
}
```

**Response:**
```json
{
  "ok": true,
  "results": {
    "output": "<pr-server response>"
  }
}
```

**Error Response:**
```json
{
  "ok": false,
  "error": {
    "code": "INVALID_INPUT",
    "message": "keyword is required",
    "retryable": false
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/courses/search \
  -H "Content-Type: application/json" \
  -d '{
    "keyword": "computer",
    "campus": "shenzhen",
    "limit": 5
  }'
```

---

### POST /api/courses/read

Read detailed information about a specific course.

**Request Body:**
```json
{
  "course_code": "string (required) - The course code to read",
  "campus": "string (optional) - shenzhen|weihai|harbin. Default: shenzhen",
  "repo": "string (optional) - Passed through to pr-server",
  "org": "string (optional) - Passed through to pr-server",
  "include_toml": false
}
```

**Response:**
```json
{
  "ok": true,
  "course": {
    "output": "<pr-server response>"
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/courses/read \
  -H "Content-Type: application/json" \
  -d '{
    "course_code": "CS101",
    "campus": "shenzhen"
  }'
```

---

## PR API

### POST /api/pr/preview

Preview course material changes without submitting a PR.

**Request Body:**
```json
{
  "campus": "string (required) - Campus name",
  "course_code": "string (required) - Course code",
  "ops": [
    {
      "op": "string (required) - Operation type",
      ...
    }
  ]
}
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "base": {
      "org": "string",
      "repo": "string",
      "ref": "string",
      "toml_path": "string"
    },
    "result": {
      "readme_toml": "string (TOML content)",
      "readme_md": "string (rendered markdown)"
    },
    "summary": {
      "changed_files": ["readme.toml", "README.md"],
      "warnings": []
    }
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/pr/preview \
  -H "Content-Type: application/json" \
  -d '{
    "campus": "shenzhen",
    "course_code": "CS101",
    "ops": [{"op": "append_course_review", "content": "Great course!"}]
  }'
```

---

### POST /api/pr/submit

Submit course material changes by creating a GitHub PR.

**Request Body:**
```json
{
  "campus": "string (required) - Campus name",
  "course_code": "string (required) - Course code",
  "ops": [
    {
      "op": "string (required) - Operation type",
      ...
    }
  ],
  "idempotency_key": "string (optional) - If not provided, auto-generated"
}
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "pr_number": 123,
    "pr_url": "https://github.com/org/repo/pull/123",
    "branch": "update-course-cs101-abc123"
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/pr/submit \
  -H "Content-Type: application/json" \
  -d '{
    "campus": "shenzhen",
    "course_code": "CS101",
    "ops": [{"op": "append_course_review", "content": "Great course!"}],
    "idempotency_key": "unique-key-123"
  }'
```

---

### POST /api/pr/lookup

Query PR status.

**Request Body:**
```json
{
  "org": "string (required) - GitHub organization",
  "repo": "string (required) - GitHub repository",
  "number": 123,
  "pr": 123
}
```

**Note**: Either `number` OR `pr` must be provided.

**Response:**
```json
{
  "ok": true,
  "pr": {
    "number": 123,
    "state": "open|closed|merged",
    "url": "string",
    "merged": true,
    "checks": {
      "status": "string",
      "conclusion": "string (optional)"
    }
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/pr/lookup \
  -H "Content-Type: application/json" \
  -d '{
    "org": "myorg",
    "repo": "courses",
    "number": 123
  }'
```

---

## Files API

### POST /api/files/upload

Upload a file to COS (Cloud Object Storage).

**Request Body:**
```json
{
  "key": "string (required) - Destination path in COS",
  "content_base64": "string (required) - Base64 encoded file content",
  "mime_type": "string (optional) - MIME type of the file"
}
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "key": "string",
    "url": "string",
    "size": 1234
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/files/upload \
  -H "Content-Type: application/json" \
  -d '{
    "key": "documents/report.pdf",
    "content_base64": "JVBERi0xLjQK...",
    "mime_type": "application/pdf"
  }'
```

---

### POST /api/files/download

Download a file from COS.

**Request Body:**
```json
{
  "key": "string (required) - File path in COS"
}
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "key": "string",
    "content_base64": "string",
    "mime_type": "string",
    "size": 1234
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/files/download \
  -H "Content-Type: application/json" \
  -d '{"key": "documents/report.pdf"}'
```

---

### POST /api/files/list

List files in COS.

**Request Body:**
```json
{
  "prefix": "string (optional) - Path prefix filter",
  "max_keys": 100
}
```

**Response:**
```json
{
  "ok": true,
  "files": [
    {
      "key": "string",
      "size": 1234,
      "last_modified": "2024-01-01T00:00:00Z"
    }
  ],
  "prefix": "string",
  "count": 1
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/files/list \
  -H "Content-Type: application/json" \
  -d '{"prefix": "documents/", "max_keys": 50}'
```

---

### POST /api/files/delete

Delete a file from COS.

**Request Body:**
```json
{
  "key": "string (required) - File path in COS"
}
```

**Response:**
```json
{
  "ok": true,
  "deleted": true,
  "key": "string"
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/files/delete \
  -H "Content-Type: application/json" \
  -d '{"key": "documents/old-file.pdf"}'
```

---

## RAG API

### POST /api/rag/query

Query the RAG (Retrieval-Augmented Generation) system.

**Request Body:**
```json
{
  "query": "string (required) - Search query text",
  "top_k": 5
}
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "hits": [
      {
        "id": "string",
        "score": 0.95,
        "payload": {
          "content": "string",
          "source": "string"
        }
      }
    ]
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/rag/query \
  -H "Content-Type: application/json" \
  -d '{
    "query": "machine learning basics",
    "top_k": 10
  }'
```

---

### POST /api/rag/ingest

Ingest documents into the RAG system.

**Request Body:**
```json
{
  "repo": "string (required) - GitHub repository (owner/repo)",
  "ref": "string (optional) - Git reference/branch. Default: main",
  "path_prefix": "string (optional) - Filter files by path prefix",
  "source": "string (optional) - Source label stored in document payload",
  "collection": "string (optional) - Qdrant collection name (auto-generated if not provided)",
  "dry_run": false,
  "max_files": 100,
  "max_chunks": 5000,
  "workers": 4,
  "store_in_cos": false,
  "cos_prefix": "string (optional) - Default: rag-source"
}
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "processed_files": 50,
    "processed_chunks": 1200,
    "upserted_points": 1200,
    "skipped_files": 5,
    "errors_count": 2,
    "total_files": 57,
    "workers": 4
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/rag/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "repo": "owner/docs-repo",
    "ref": "main",
    "path_prefix": "docs/",
    "max_files": 100,
    "dry_run": false
  }'
```

---

## Crawl API

### POST /api/crawl/page

Crawl a single web page.

**Request Body:**
```json
{
  "url": "string (required) - URL to crawl",
  "wait_for": "string (optional) - CSS selector to wait for",
  "exclude_paths": ["string"],
  "content_filter": true,
  "output_format": "markdown",
  "store_in_cos": true,
  "cos_prefix": "string (optional) - Default: crawls",
  "auto_ingest_rag": true
}
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "markdown": "string - Crawled content in markdown",
    "title": "string",
    "url": "string",
    "cos": {
      "key": "string"
    },
    "rag": {
      "chunks": 10
    },
    "dedup": {
      "sha256": "string",
      "skipped": false
    }
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/crawl/page \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com/article",
    "content_filter": true,
    "store_in_cos": true,
    "auto_ingest_rag": true
  }'
```

---

### POST /api/crawl/site

Crawl an entire website (asynchronous operation).

**Request Body:**
```json
{
  "start_url": "string (required) - Starting URL",
  "max_pages": 100,
  "max_depth": 3,
  "include_patterns": ["string"],
  "exclude_patterns": ["string"],
  "content_filter": true,
  "use_sitemap": true,
  "concurrent_requests": 5,
  "respect_robots_txt": true,
  "delay_between_requests": 0.5,
  "store_in_cos": true,
  "cos_prefix": "string (optional) - Default: crawls",
  "auto_ingest_to_rag": false,
  "rag_source": "string"
}
```

**Response:**
```json
{
  "ok": true,
  "result": {
    "crawl_id": "string",
    "status": "started",
    "auto_ingest_config": {
      "enabled": false,
      "source": "string",
      "store_in_cos": true,
      "cos_prefix": "crawls"
    }
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/crawl/site \
  -H "Content-Type: application/json" \
  -d '{
    "start_url": "https://example.com",
    "max_pages": 50,
    "max_depth": 2,
    "content_filter": true
  }'
```

---

### POST /api/crawl/status

Check the status of a site crawl.

**Request Body:**
```json
{
  "crawl_id": "string (required) - Crawl job ID"
}
```

**Response:**
```json
{
  "ok": true,
  "status": {
    "crawl_id": "string",
    "status": "running|completed|failed",
    "pages_crawled": 50,
    "pages_total": 100,
    "progress": 50
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/crawl/status \
  -H "Content-Type: application/json" \
  -d '{"crawl_id": "abc123"}'
```

---

---

## Search API

### POST /api/search/brave

Perform a web search using Brave Search.

**Request Body:**
```json
{
  "query": "string (required) - Search query",
  "count": 5
}
```

**Response:**
```json
{
  "ok": true,
  "results": [
    {
      "source": "brave",
      "source_type": "web",
      "title": "string",
      "content": "string",
      "url": "string",
      "score": 0.7
    }
  ]
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/search/brave \
  -H "Content-Type: application/json" \
  -d '{"query": "哈尔滨工业大学深圳", "count": 5}'
```

---
## Teachers API

### POST /api/teachers/search

Search for teacher information.

**Request Body:**
```json
{
  "name": "string (required*) - Teacher name (Chinese or Pinyin)",
  "pinyin": "string (required*) - Pinyin name",
  "department": "string (optional) - Department",
  "save_to_file": false,
  "output_path": "string (optional)",
  "auto_ingest": false
}
```

**Note**: At least `name` OR `pinyin` must be provided.

**Response:**
```json
{
  "ok": true,
  "result": {
    "success": true,
    "teacher": {
      "name": "string",
      "pinyin": "string",
      "title": "string",
      "department": "string",
      "email": "string",
      "phone": "string",
      "office": "string",
      "research": "string",
      "homepage": "string",
      "profile": "string"
    },
    "homepage": "https://homepage.hit.edu.cn/xxx?lang=zh",
    "pinyin": "string",
    "raw_content": "string"
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/teachers/search \
  -H "Content-Type: application/json" \
  -d '{
    "name": "张三",
    "department": "计算机学院"
  }'
```

---

### POST /api/teachers/batch

Search for multiple teachers in batch.

**Request Body:**
```json
{
  "names": ["string"],
  "pinyins": ["string"],
  "max_workers": 4
}
```

**Note**: At least `names` OR `pinyins` must be provided (non-empty array).

**Response:**
```json
{
  "ok": true,
  "result": {
    "total": 10,
    "success": 8,
    "failed": 2,
    "results": [
      {
        "success": true,
        "teacher": { /* TeacherInfo object */ },
        "homepage": "string",
        "pinyin": "string",
        "raw_content": "string"
      },
      {
        "success": false,
        "error": "string",
        "pinyin": "string",
        "name": "string"
      }
    ]
  }
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/teachers/batch \
  -H "Content-Type: application/json" \
  -d '{
    "names": ["张三", "李四", "王五"],
    "max_workers": 4
  }'
```

---

## HITSZ API

### POST /api/hitsz/fetch

Fetch public information from HITSZ website.

**Request Body:**
```json
{
  "url": "string (optional) - URL to fetch. Default: https://info.hitsz.edu.cn/list.jsp?wbtreeid=1053"
}
```

**Response:**
```json
{
  "ok": true,
  "login_success": true,
  "page": {
    "title": "string",
    "content": "string",
    "links": ["string"]
  }
}
```

**Note**: Requires `HITSZ_USERNAME` and `HITSZ_PASSWORD` environment variables to be configured.

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/hitsz/fetch \
  -H "Content-Type: application/json" \
  -d '{"url": "https://info.hitsz.edu.cn/list.jsp?wbtreeid=1053"}'
```

---

## AI API

### POST /api/ai/chat

Simple chat with AI.

**Request Body:**
```json
{
  "message": "string (required) - User message",
  "system": "string (optional) - System prompt. Default: 'You are a helpful AI assistant.'"
}
```

**Response:**
```json
{
  "ok": true,
  "response": "string - AI response"
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/ai/chat \
  -H "Content-Type: application/json" \
  -d '{
    "message": "What is machine learning?",
    "system": "You are a helpful teaching assistant."
  }'
```

---

### POST /api/ai/react

ReAct (Reasoning + Acting) chat with AI using available tools.

**Request Body:**
```json
{
  "message": "string (required) - User message"
}
```

**Response:**
```json
{
  "ok": true,
  "response": "string - AI response"
}
```

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/ai/react \
  -H "Content-Type: application/json" \
  -d '{"message": "Search for information about CS101 and summarize it"}'
```

---

## Temp Files API

### POST /api/temp/upload

Upload a temporary file.

**Option 1: Multipart Form Data**
```bash
curl -X POST http://localhost:8080/api/temp/upload \
  -H "Content-Type: multipart/form-data" \
  -F "file=@/path/to/file.pdf"
```

**Option 2: JSON with Base64 Content**

**Request Body:**
```json
{
  "name": "string (required) - File name",
  "mime_type": "string (required) - MIME type",
  "content_base64": "string (required) - Base64 encoded content"
}
```

**Response:**
```json
{
  "ok": true,
  "id": "string - Temporary file ID",
  "name": "string - File name",
  "size": 1234,
  "mime_type": "string",
  "expires_at": "2024-01-01T00:00:00Z"
}
```

**Curl Example (JSON):**
```bash
curl -X POST http://localhost:8080/api/temp/upload \
  -H "Content-Type: application/json" \
  -d '{
    "name": "document.pdf",
    "mime_type": "application/pdf",
    "content_base64": "JVBERi0xLjQK..."
  }'
```

---

### GET /api/temp/download

Download a temporary file.

**Query Parameters:**
- `id` (required) - Temporary file ID

**Response**: File content as binary stream with appropriate Content-Type header.

**Curl Example:**
```bash
curl "http://localhost:8080/api/temp/download?id=abc123" -o downloaded_file.pdf
```

---

### POST /api/temp/download

Download a temporary file (POST variant).

**Request Body:**
```json
{
  "id": "string (required) - Temporary file ID"
}
```

**Response**: File content as binary stream with appropriate Content-Type header.

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/temp/download \
  -H "Content-Type: application/json" \
  -d '{"id": "abc123"}' \
  -o downloaded_file.pdf
```

---

### GET /api/temp/list

List all temporary files.

**Response:**
```json
{
  "ok": true,
  "files": [
    {
      "id": "string",
      "name": "string",
      "size": 1234,
      "mime_type": "string",
      "expires_at": "2024-01-01T00:00:00Z"
    }
  ],
  "count": 1
}
```

**Curl Example:**
```bash
curl http://localhost:8080/api/temp/list
```

---

### POST /api/temp/list

List all temporary files (POST variant).

**Response:** Same as GET variant.

**Curl Example:**
```bash
curl -X POST http://localhost:8080/api/temp/list
```

---

---

### POST /api/temp/parse

Parse a file (PDF, DOCX, PPTX, etc.) to Markdown using Unstructured MCP server. Supports two input modes: by temp file ID or by base64 content. **Returns max 5000 characters by default.**

**Request Body:**
```json
{
  "id": "string (optional) - Temp file ID from /api/temp/upload",
  "content_base64": "string (optional) - Base64 encoded file content",
  "filename": "string (optional) - Original filename (required if using content_base64)",
  "max_chars": 5000
}
```

**Response:**
```json
{
  "ok": true,
  "markdown": "string - Parsed content in Markdown format",
  "filename": "string - Original filename",
  "truncated": false,
  "char_count": 1234
}
```

**Curl Example:**
```bash
# Parse by temp file ID
curl -X POST http://localhost:8080/api/temp/parse \
  -H "Content-Type: application/json" \
  -d '{"id": "abc123"}'

# Parse by base64 content
curl -X POST http://localhost:8080/api/temp/parse \
  -H "Content-Type: application/json" \
  -d '{"content_base64": "JVBERi0x...", "filename": "slides.pptx", "max_chars": 5000}'
```

---

## Error Responses

All endpoints return errors in a consistent format:

### HTTP Error Response (Non-200 Status)
```json
{
  "ok": false,
  "error": {
    "code": "HTTP_ERROR",
    "message": "Method not allowed"
  }
}
```

### Invoke Error Response (200 Status with Error)
```json
{
  "ok": false,
  "error": {
    "code": "INVALID_INPUT|NOT_FOUND|INTERNAL",
    "message": "human-readable error message",
    "retryable": true|false
  }
}
```

**Error Codes:**
- `INVALID_INPUT` - Missing or invalid parameters
- `NOT_FOUND` - Resource not found (MCP server, file, etc.)
- `INTERNAL` - Internal server error
- `CONFIG` - Configuration error
- `HTTP_ERROR` - HTTP-level error (method not allowed, etc.)

**Retryable:**
- `true` - Safe to retry the request
- `false` - Fix the input before retrying

---

## Common Response Patterns

### Success Response
```json
{
  "ok": true,
  ...endpoint-specific fields
}
```

### Service Unavailable Response
```json
{
  "ok": false,
  "error": {
    "code": "HTTP_ERROR",
    "message": "MCP registry not configured"
  }
}
```

---

*Last updated: 2025-01*
