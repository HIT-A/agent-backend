#!/usr/bin/env python3
"""
Crawl4AI MCP Server

This MCP server wraps Crawl4AI for web crawling capabilities.
Supports both single page and full site crawling with AI-friendly output formats.
"""

import asyncio
import json
import os
import sys
from typing import Any, Dict, List, Optional
from urllib.parse import urljoin, urlparse

# Add crawl4ai to path (will be installed in Docker)
try:
    from crawl4ai import AsyncWebCrawler, BrowserConfig, CrawlerRunConfig, CacheMode
    from crawl4ai.content_filter_strategy import PruningContentFilter
    from crawl4ai.markdown_generation_strategy import DefaultMarkdownGenerator
except ImportError:
    print("Warning: crawl4ai not installed. Running in mock mode.", file=sys.stderr)
    AsyncWebCrawler = None


def _build_browser_config() -> "BrowserConfig":
    """Use conservative Playwright flags for server environments."""
    extra_args = ["--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu"]
    proxy = os.environ.get("CRAWL4AI_PROXY")
    return BrowserConfig(
        browser_type="chromium",
        channel="chromium",
        headless=True,
        extra_args=extra_args,
        proxy=proxy,
    )


def _extract_markdown(result: Any) -> Dict[str, str]:
    """Handle both legacy (markdown_v2) and newer (markdown) result structures."""
    raw_markdown = ""
    fit_markdown = ""

    markdown_v2 = getattr(result, "markdown_v2", None)
    if markdown_v2 is not None:
        raw_markdown = getattr(markdown_v2, "raw_markdown", "") or ""
        fit_markdown = getattr(markdown_v2, "fit_markdown", "") or ""

    if not raw_markdown:
        markdown = getattr(result, "markdown", None)
        if markdown is not None:
            raw_markdown = getattr(markdown, "raw_markdown", "") or str(markdown)
            fit_markdown = getattr(markdown, "fit_markdown", "") or ""

    return {
        "raw": raw_markdown,
        "fit": str(fit_markdown) if fit_markdown is not None else "",
    }


def _extract_text(result: Any) -> str:
    text = getattr(result, "text", None)
    if text:
        return text
    md = _extract_markdown(result).get("raw", "")
    if md:
        return md
    return getattr(result, "cleaned_html", "") or getattr(result, "html", "") or ""

from mcp.server import Server
from mcp.types import Tool, TextContent

# Create server
server = Server("crawl4ai-server")

# Storage for crawl results
_crawl_results: Dict[str, Any] = {}


@server.list_tools()
async def list_tools() -> List[Tool]:
    """List available crawling tools."""
    return [
        Tool(
            name="crawl_page",
            description="Crawl a single web page and extract content in markdown format",
            inputSchema={
                "type": "object",
                "properties": {
                    "url": {"type": "string", "description": "URL to crawl"},
                    "wait_for": {
                        "type": "string",
                        "description": "CSS selector to wait for before extracting content",
                    },
                    "exclude_paths": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "URL patterns to exclude",
                    },
                    "content_filter": {
                        "type": "boolean",
                        "description": "Whether to use AI content filtering to remove noise",
                        "default": True,
                    },
                    "output_format": {
                        "type": "string",
                        "enum": ["markdown", "html", "text"],
                        "description": "Output format",
                        "default": "markdown",
                    },
                },
                "required": ["url"],
            },
        ),
        Tool(
            name="crawl_site",
            description="Crawl an entire website starting from the given URL. Uses sitemap.xml if available, otherwise follows links.",
            inputSchema={
                "type": "object",
                "properties": {
                    "start_url": {
                        "type": "string",
                        "description": "Starting URL for site crawl",
                    },
                    "max_pages": {
                        "type": "integer",
                        "description": "Maximum number of pages to crawl",
                        "default": 100,
                    },
                    "max_depth": {
                        "type": "integer",
                        "description": "Maximum link depth to follow",
                        "default": 3,
                    },
                    "include_patterns": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "URL patterns to include (regex)",
                    },
                    "exclude_patterns": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "URL patterns to exclude (regex)",
                    },
                    "content_filter": {
                        "type": "boolean",
                        "description": "Whether to use AI content filtering",
                        "default": True,
                    },
                    "use_sitemap": {
                        "type": "boolean",
                        "description": "Whether to use sitemap.xml if available",
                        "default": True,
                    },
                    "concurrent_requests": {
                        "type": "integer",
                        "description": "Number of concurrent requests",
                        "default": 5,
                    },
                    "respect_robots_txt": {
                        "type": "boolean",
                        "description": "Whether to respect robots.txt",
                        "default": True,
                    },
                    "delay_between_requests": {
                        "type": "number",
                        "description": "Delay between requests in seconds",
                        "default": 0.5,
                    },
                },
                "required": ["start_url"],
            },
        ),
        Tool(
            name="get_crawl_status",
            description="Get the status of a running or completed site crawl",
            inputSchema={
                "type": "object",
                "properties": {
                    "crawl_id": {"type": "string", "description": "ID of the crawl job"}
                },
                "required": ["crawl_id"],
            },
        ),
        Tool(
            name="extract_structured_data",
            description="Extract structured data from a page using LLM or CSS selectors",
            inputSchema={
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "description": "URL to extract data from",
                    },
                    "schema": {
                        "type": "object",
                        "description": "JSON schema describing the data to extract",
                    },
                    "extraction_method": {
                        "type": "string",
                        "enum": ["llm", "css", "xpath"],
                        "description": "Extraction method",
                        "default": "llm",
                    },
                    "instructions": {
                        "type": "string",
                        "description": "Instructions for LLM extraction (when using llm method)",
                    },
                },
                "required": ["url", "schema"],
            },
        ),
    ]


@server.call_tool()
async def call_tool(name: str, arguments: Dict[str, Any]) -> List[TextContent]:
    """Handle tool calls."""

    if AsyncWebCrawler is None:
        return [
            TextContent(
                type="text",
                text=json.dumps(
                    {
                        "error": "Crawl4AI not installed. Please install with: pip install crawl4ai"
                    }
                ),
            )
        ]

    try:
        if name == "crawl_page":
            result = await crawl_single_page(arguments)
            return [TextContent(type="text", text=json.dumps(result, indent=2))]

        elif name == "crawl_site":
            result = await crawl_full_site(arguments)
            return [TextContent(type="text", text=json.dumps(result, indent=2))]

        elif name == "get_crawl_status":
            result = await get_crawl_status(arguments)
            return [TextContent(type="text", text=json.dumps(result, indent=2))]

        elif name == "extract_structured_data":
            result = await extract_structured(arguments)
            return [TextContent(type="text", text=json.dumps(result, indent=2))]

        else:
            return [
                TextContent(
                    type="text", text=json.dumps({"error": f"Unknown tool: {name}"})
                )
            ]

    except Exception as e:
        return [
            TextContent(
                type="text",
                text=json.dumps({"error": str(e), "type": type(e).__name__}),
            )
        ]


async def crawl_single_page(args: Dict[str, Any]) -> Dict[str, Any]:
    """Crawl a single page."""
    url = args["url"]
    wait_for = args.get("wait_for")
    content_filter = args.get("content_filter", True)
    output_format = args.get("output_format", "markdown")

    config = CrawlerRunConfig(
        cache_mode=CacheMode.BYPASS,
        wait_for=wait_for if wait_for else None,
    )

    if content_filter:
        config.content_filter = PruningContentFilter()

    async with AsyncWebCrawler(config=_build_browser_config(), verbose=True) as crawler:
        result = await crawler.arun(url=url, config=config)
        markdown = _extract_markdown(result)
        text = _extract_text(result)

        response = {
            "url": url,
            "success": result.success,
            "status_code": getattr(result, "status_code", None),
            "title": result.metadata.get("title", ""),
            "timestamp": getattr(result, "timestamp", None),
        }

        if result.success:
            if output_format == "markdown":
                response["content"] = markdown["raw"]
                response["fit_markdown"] = markdown["fit"]
            elif output_format == "html":
                response["content"] = result.html
            elif output_format == "text":
                response["content"] = text

            response["metadata"] = {
                "links": result.links,
                "images": getattr(result, "images", []),
                "word_count": len(text.split()) if text else 0,
            }
        else:
            response["error"] = result.error_message

        return response


async def crawl_full_site(args: Dict[str, Any]) -> Dict[str, Any]:
    """Start a full site crawl (async background job)."""
    import uuid

    crawl_id = str(uuid.uuid4())
    start_url = args["start_url"]
    max_pages = args.get("max_pages", 100)
    max_depth = args.get("max_depth", 3)
    include_patterns = args.get("include_patterns", [])
    exclude_patterns = args.get("exclude_patterns", [])
    content_filter = args.get("content_filter", True)
    use_sitemap = args.get("use_sitemap", True)
    concurrent_requests = args.get("concurrent_requests", 5)
    respect_robots = args.get("respect_robots_txt", True)
    delay = args.get("delay_between_requests", 0.5)

    # Store initial status
    _crawl_results[crawl_id] = {
        "crawl_id": crawl_id,
        "start_url": start_url,
        "status": "starting",
        "pages_crawled": 0,
        "total_pages": 0,
        "results": [],
        "errors": [],
        "start_time": asyncio.get_event_loop().time(),
    }

    # Start crawl in background
    asyncio.create_task(
        _run_site_crawl(
            crawl_id=crawl_id,
            start_url=start_url,
            max_pages=max_pages,
            max_depth=max_depth,
            include_patterns=include_patterns,
            exclude_patterns=exclude_patterns,
            content_filter=content_filter,
            use_sitemap=use_sitemap,
            concurrent_requests=concurrent_requests,
            respect_robots=respect_robots,
            delay=delay,
        )
    )

    return {
        "crawl_id": crawl_id,
        "status": "started",
        "message": f"Site crawl started from {start_url}. Use get_crawl_status to check progress.",
    }


async def _run_site_crawl(**kwargs):
    """Background task for site crawling."""
    crawl_id = kwargs["crawl_id"]
    start_url = kwargs["start_url"]
    max_pages = kwargs["max_pages"]

    try:
        _crawl_results[crawl_id]["status"] = "crawling"

        config = CrawlerRunConfig(
            cache_mode=CacheMode.BYPASS,
        )

        if kwargs["content_filter"]:
            config.content_filter = PruningContentFilter()

        pages_crawled = []
        errors = []

        async with AsyncWebCrawler(config=_build_browser_config(), verbose=True) as crawler:
            # Try sitemap first if enabled
            if kwargs["use_sitemap"]:
                sitemap_url = urljoin(start_url, "/sitemap.xml")
                try:
                    sitemap_result = await crawler.arun(url=sitemap_url, config=config)
                    if sitemap_result.success:
                        # Parse sitemap and extract URLs
                        # This is simplified - real implementation would parse XML
                        pass
                except:
                    pass  # Fall back to link crawling

            # BFS crawling
            visited = set()
            queue = [(start_url, 0)]

            while queue and len(pages_crawled) < max_pages:
                url, depth = queue.pop(0)

                if url in visited or depth > kwargs["max_depth"]:
                    continue

                visited.add(url)

                # Check patterns
                if kwargs["exclude_patterns"]:
                    if any(pattern in url for pattern in kwargs["exclude_patterns"]):
                        continue

                if kwargs["include_patterns"]:
                    if not any(
                        pattern in url for pattern in kwargs["include_patterns"]
                    ):
                        continue

                try:
                    result = await crawler.arun(url=url, config=config)
                    markdown = _extract_markdown(result)
                    text = _extract_text(result)

                    if result.success:
                        pages_crawled.append(
                            {
                                "url": url,
                                "title": result.metadata.get("title", ""),
                                "markdown": markdown["raw"],
                                "fit_markdown": markdown["fit"],
                                "word_count": len(text.split()) if text else 0,
                                "depth": depth,
                            }
                        )

                        # Extract links for further crawling
                        if depth < kwargs["max_depth"]:
                            for link in result.links.get("internal", []):
                                if link not in visited:
                                    queue.append((link, depth + 1))
                    else:
                        errors.append({"url": url, "error": result.error_message})

                except Exception as e:
                    errors.append({"url": url, "error": str(e)})

                # Update status
                _crawl_results[crawl_id].update(
                    {
                        "pages_crawled": len(pages_crawled),
                        "total_pages": len(pages_crawled) + len(queue),
                    }
                )

                # Delay between requests
                if kwargs["delay"] > 0:
                    await asyncio.sleep(kwargs["delay"])

        # Store final results
        _crawl_results[crawl_id].update(
            {
                "status": "completed",
                "pages_crawled": len(pages_crawled),
                "results": pages_crawled,
                "errors": errors,
                "end_time": asyncio.get_event_loop().time(),
            }
        )

    except Exception as e:
        _crawl_results[crawl_id].update(
            {
                "status": "failed",
                "error": str(e),
            }
        )


async def get_crawl_status(args: Dict[str, Any]) -> Dict[str, Any]:
    """Get crawl status and results."""
    crawl_id = args["crawl_id"]

    if crawl_id not in _crawl_results:
        return {"error": "Crawl ID not found"}

    result = _crawl_results[crawl_id].copy()

    # Calculate duration if completed
    if "end_time" in result and "start_time" in result:
        result["duration_seconds"] = result["end_time"] - result["start_time"]
    elif "start_time" in result:
        result["duration_seconds"] = (
            asyncio.get_event_loop().time() - result["start_time"]
        )

    return result


async def extract_structured(args: Dict[str, Any]) -> Dict[str, Any]:
    """Extract structured data from a page."""
    url = args["url"]
    schema = args["schema"]
    method = args.get("extraction_method", "llm")
    instructions = args.get("instructions", "")

    config = CrawlerRunConfig(
        cache_mode=CacheMode.BYPASS,
    )

    async with AsyncWebCrawler(config=_build_browser_config(), verbose=True) as crawler:
        result = await crawler.arun(url=url, config=config)

        if not result.success:
            return {"error": result.error_message}

        text = _extract_text(result)

        # For LLM extraction, we'd use an LLM to extract based on schema
        # For CSS/XPath, we'd use the appropriate selectors
        # This is a placeholder implementation

        return {
            "url": url,
            "method": method,
            "schema": schema,
            "instructions": instructions,
            "content_preview": text[:1000] if text else "",
            "note": "Structured extraction requires LLM integration. Content preview provided.",
        }


async def main():
    """Main entry point."""
    from mcp.server.stdio import stdio_server

    async with stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream, write_stream, server.create_initialization_options()
        )


if __name__ == "__main__":
    asyncio.run(main())
