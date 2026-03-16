# Crawl4AI MCP Server

Crawl4AI MCP Server provides web crawling capabilities for the HITA agent-backend.

## Features

- **Single Page Crawl**: Extract content from a single URL
- **Full Site Crawl**: Crawl entire websites with configurable depth and limits
- **Structured Data Extraction**: Extract data using LLM or CSS selectors
- **AI Content Filtering**: Remove noise and boilerplate content
- **Multiple Output Formats**: Markdown, HTML, plain text

## Installation

### Option 1: Using Docker (Recommended)

```bash
cd mcp-servers/crawl4ai
docker build -t crawl4ai-mcp .
docker run -p 8080:8080 crawl4ai-mcp
```

### Option 2: Local Installation

```bash
pip install crawl4ai mcp
playwright install chromium
python server.py
```

## Configuration

Add to your agent-backend environment:

```bash
# MCP Server Configuration
MCP_SERVERS='[
  {
    "name": "crawl4ai",
    "transport": "stdio",
    "command": ["python", "mcp-servers/crawl4ai/server.py"],
    "enabled": true
  }
]'
```

## Tools

### 1. crawl_page

Crawl a single web page.

**Input:**
```json
{
  "url": "https://example.com/article",
  "wait_for": "article.content",
  "content_filter": true,
  "output_format": "markdown"
}
```

**Output:**
```json
{
  "url": "https://example.com/article",
  "success": true,
  "status_code": 200,
  "title": "Article Title",
  "content": "# Markdown content...",
  "fit_markdown": "Filtered content...",
  "metadata": {
    "links": {...},
    "images": [...],
    "word_count": 1500
  }
}
```

### 2. crawl_site

Start a full site crawl (async).

**Input:**
```json
{
  "start_url": "https://docs.example.com",
  "max_pages": 100,
  "max_depth": 3,
  "include_patterns": ["/docs/", "/api/"],
  "exclude_patterns": ["/blog/", "/admin/"],
  "use_sitemap": true,
  "concurrent_requests": 5,
  "content_filter": true
}
```

**Output:**
```json
{
  "crawl_id": "uuid",
  "status": "started",
  "message": "Site crawl started..."
}
```

### 3. get_crawl_status

Check crawl progress and get results.

**Input:**
```json
{
  "crawl_id": "uuid"
}
```

**Output:**
```json
{
  "crawl_id": "uuid",
  "status": "completed",
  "pages_crawled": 50,
  "total_pages": 50,
  "results": [
    {
      "url": "...",
      "title": "...",
      "markdown": "...",
      "word_count": 1200
    }
  ],
  "errors": [],
  "duration_seconds": 120
}
```

### 4. extract_structured_data

Extract structured data using LLM.

**Input:**
```json
{
  "url": "https://example.com/product",
  "schema": {
    "title": "string",
    "price": "number",
    "description": "string"
  },
  "extraction_method": "llm",
  "instructions": "Extract product information"
}
```

## Usage with agent-backend

### Register MCP Server

```bash
curl -X POST http://localhost:8080/v1/skills/mcp.register_server:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "name": "crawl4ai",
      "transport": "stdio",
      "command": ["python", "mcp-servers/crawl4ai/server.py"]
    }
  }'
```

### Call Crawl Tools

```bash
# Crawl single page
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "crawl4ai",
      "tool": "crawl_page",
      "arguments": {
        "url": "https://example.com",
        "output_format": "markdown"
      }
    }
  }'

# Start site crawl
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -H "Content-Type: application/json" \
  -d '{
    "input": {
      "server": "crawl4ai",
      "tool": "crawl_site",
      "arguments": {
        "start_url": "https://docs.example.com",
        "max_pages": 50
      }
    }
  }'
```

## RAG Integration

Crawled content can be directly ingested into RAG:

```bash
# 1. Crawl documentation
curl -X POST http://localhost:8080/v1/skills/mcp.call_tool:invoke \
  -d '{
    "input": {
      "server": "crawl4ai",
      "tool": "crawl_site",
      "arguments": {
        "start_url": "https://docs.python.org/3",
        "max_pages": 100,
        "include_patterns": ["/3/tutorial/", "/3/library/"]
      }
    }
  }'

# 2. Store crawl results to COS
curl -X POST http://localhost:8080/v1/skills/cos.save_file:invoke \
  -d '{
    "input": {
      "key": "crawls/python-docs/crawl_result.json",
      "content_base64": "..."
    }
  }'

# 3. Ingest to RAG
curl -X POST http://localhost:8080/v1/skills/rag.ingest:invoke \
  -d '{
    "input": {
      "repo": "hit/hita-crawled-docs",
      "ref": "main",
      "source": "python-docs"
    }
  }'
```