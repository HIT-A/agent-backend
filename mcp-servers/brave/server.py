#!/usr/bin/env python3
import json
import os
import re
from html import unescape
from typing import Any, Dict, List
from urllib.parse import parse_qs, urlparse, unquote

import requests
from mcp.server import Server
from mcp.types import TextContent, Tool

server = Server("brave-search-local")

API_URL = "https://api.search.brave.com/res/v1/web/search"
ANSWER_API_URL = "https://api.search.brave.com/res/v1/chat/completions"
DDG_HTML_URL = "https://html.duckduckgo.com/html/"


def _api_key() -> str:
    return os.environ.get("BRAVE_API_KEY", "") or os.environ.get(
        "BRAVE_SEARCH_API_KEY", ""
    )


def _answer_key() -> str:
    return os.environ.get("BRAVE_ANSWER_API_KEY", "")


def _proxies() -> Dict[str, str]:
    http_proxy = os.environ.get("HTTP_PROXY")
    https_proxy = os.environ.get("HTTPS_PROXY")
    proxies: Dict[str, str] = {}
    if http_proxy:
        proxies["http"] = http_proxy
    if https_proxy:
        proxies["https"] = https_proxy
    return proxies


def _parse_monthly_limit(limit_header: str) -> int:
    # Brave rate-limit header is often like: "2, 0"
    # where the second value represents longer-window quota.
    try:
        parts = [p.strip() for p in (limit_header or "").split(",")]
        if len(parts) >= 2:
            return int(parts[1])
    except Exception:
        return -1
    return -1


def _usage_headers(headers: Dict[str, Any]) -> Dict[str, Any]:
    usage: Dict[str, Any] = {}
    for key, value in headers.items():
        lower = key.lower()
        if lower.startswith("x-request-"):
            usage[key] = value
    return usage


def _strip_tags(text: str) -> str:
    return re.sub(r"<[^>]+>", "", text or "")


def _ddg_fallback(query: str, count: int, timeout: int = 20) -> List[Dict[str, str]]:
    resp = requests.get(
        DDG_HTML_URL,
        params={"q": query},
        headers={"User-Agent": "Mozilla/5.0"},
        proxies=_proxies() or None,
        timeout=timeout,
    )
    if resp.status_code != 200:
        return []

    html = resp.text
    results: List[Dict[str, str]] = []
    blocks = re.findall(
        r'<div class="result__body">(.*?)</div>\s*</div>', html, flags=re.S
    )
    if not blocks:
        blocks = re.findall(
            r'<a rel="nofollow" class="result__a".*?</a>', html, flags=re.S
        )

    for block in blocks:
        a = re.search(
            r'<a[^>]*class="result__a"[^>]*href="([^"]+)"[^>]*>(.*?)</a>',
            block,
            flags=re.S,
        )
        if not a:
            continue
        url = unescape(a.group(1)).strip()
        if url.startswith("//"):
            url = "https:" + url
        if "duckduckgo.com/l/?" in url:
            qs = parse_qs(urlparse(url).query)
            if qs.get("uddg"):
                url = unquote(qs["uddg"][0])
        title = unescape(_strip_tags(a.group(2))).strip()

        sn = re.search(
            r'<a[^>]*class="result__snippet"[^>]*>(.*?)</a>', block, flags=re.S
        )
        if not sn:
            sn = re.search(
                r'<div[^>]*class="result__snippet"[^>]*>(.*?)</div>', block, flags=re.S
            )
        desc = unescape(_strip_tags(sn.group(1))).strip() if sn else ""

        if url and title:
            results.append({"title": title, "url": url, "description": desc})
        if len(results) >= count:
            break
    return results


@server.list_tools()
async def list_tools() -> List[Tool]:
    return [
        Tool(
            name="brave_web_search",
            description="Search the web with Brave Search API.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search query"},
                    "count": {
                        "type": "integer",
                        "description": "Number of results (1-20)",
                        "default": 5,
                    },
                    "country": {
                        "type": "string",
                        "description": "Country code like US/CN",
                        "default": "US",
                    },
                    "search_lang": {
                        "type": "string",
                        "description": "Language code",
                        "default": "en",
                    },
                },
                "required": ["query"],
            },
        ),
        Tool(
            name="brave_answer",
            description="Get an AI-grounded answer from Brave Answers API.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Question to answer"},
                    "model": {
                        "type": "string",
                        "description": "Brave Answers model",
                        "default": "brave",
                    },
                    "country": {
                        "type": "string",
                        "description": "Country code like us/cn",
                        "default": "us",
                    },
                    "language": {
                        "type": "string",
                        "description": "Language code",
                        "default": "en",
                    },
                    "enable_citations": {
                        "type": "boolean",
                        "description": "Enable inline citations in supported modes",
                        "default": False,
                    },
                    "enable_research": {
                        "type": "boolean",
                        "description": "Enable multi-search research mode",
                        "default": False,
                    },
                },
                "required": ["query"],
            },
        ),
    ]


@server.call_tool()
async def call_tool(name: str, arguments: Dict[str, Any]) -> List[TextContent]:
    if name == "brave_web_search":
        key = _api_key()
        if not key:
            return [
                TextContent(
                    type="text", text=json.dumps({"error": "Missing BRAVE_API_KEY"})
                )
            ]

        query = arguments.get("query", "")
        count = max(1, min(int(arguments.get("count", 5)), 20))

        params = {
            "q": query,
            "count": count,
            "country": arguments.get("country", "US"),
            "search_lang": arguments.get("search_lang", "en"),
        }

        headers = {
            "Accept": "application/json",
            "X-Subscription-Token": key,
        }

        try:
            resp = requests.get(
                API_URL,
                params=params,
                headers=headers,
                proxies=_proxies() or None,
                timeout=30,
            )
            if resp.status_code != 200:
                return [
                    TextContent(
                        type="text",
                        text=json.dumps(
                            {
                                "error": "Brave API request failed",
                                "status_code": resp.status_code,
                                "body": resp.text[:1200],
                            }
                        ),
                    )
                ]

            data = resp.json()
            web = data.get("web", {}) if isinstance(data, dict) else {}
            web_results = web.get("results", []) if isinstance(web, dict) else []

            # Some keys return HTTP 200 but only include query metadata, without web results.
            # This usually indicates key entitlement/quota issue instead of transport failure.
            if not web_results:
                limit_header = resp.headers.get("x-ratelimit-limit", "")
                monthly_limit = _parse_monthly_limit(limit_header)
                hint = "Brave API returned no web results"
                if monthly_limit == 0:
                    hint = "Brave API key appears to have no data quota/entitlement (x-ratelimit-limit second value is 0)"

                fallback_results = _ddg_fallback(query, count)
                if fallback_results:
                    return [
                        TextContent(
                            type="text",
                            text=json.dumps(
                                {
                                    "query": query,
                                    "count": len(fallback_results),
                                    "results": fallback_results,
                                    "warning": hint,
                                    "fallback": {
                                        "provider": "duckduckgo-html",
                                        "used": True,
                                    },
                                    "rate_limit": {
                                        "limit": resp.headers.get(
                                            "x-ratelimit-limit", ""
                                        ),
                                        "remaining": resp.headers.get(
                                            "x-ratelimit-remaining", ""
                                        ),
                                        "reset": resp.headers.get(
                                            "x-ratelimit-reset", ""
                                        ),
                                    },
                                    "raw_top_keys": list(data.keys())
                                    if isinstance(data, dict)
                                    else [],
                                },
                                ensure_ascii=False,
                            ),
                        )
                    ]

                return [
                    TextContent(
                        type="text",
                        text=json.dumps(
                            {
                                "query": query,
                                "count": 0,
                                "results": [],
                                "warning": hint,
                                "fallback": {
                                    "provider": "duckduckgo-html",
                                    "used": False,
                                },
                                "rate_limit": {
                                    "limit": resp.headers.get("x-ratelimit-limit", ""),
                                    "remaining": resp.headers.get(
                                        "x-ratelimit-remaining", ""
                                    ),
                                    "reset": resp.headers.get("x-ratelimit-reset", ""),
                                },
                                "raw_top_keys": list(data.keys())
                                if isinstance(data, dict)
                                else [],
                            },
                            ensure_ascii=False,
                        ),
                    )
                ]

            results = []
            for item in web_results:
                results.append(
                    {
                        "title": item.get("title", ""),
                        "url": item.get("url", ""),
                        "description": item.get("description", ""),
                    }
                )

            return [
                TextContent(
                    type="text",
                    text=json.dumps(
                        {
                            "query": query,
                            "count": len(results),
                            "results": results,
                        },
                        ensure_ascii=False,
                    ),
                )
            ]
        except Exception as e:
            return [TextContent(type="text", text=json.dumps({"error": str(e)}))]

    if name == "brave_answer":
        key = _answer_key()
        if not key:
            return [
                TextContent(
                    type="text",
                    text=json.dumps({"error": "Missing BRAVE_ANSWER_API_KEY"}),
                )
            ]

        query = arguments.get("query", "")
        payload = {
            "model": arguments.get("model", "brave"),
            "stream": False,
            "messages": [{"role": "user", "content": query}],
            "country": arguments.get("country", "us"),
            "language": arguments.get("language", "en"),
            "enable_citations": bool(arguments.get("enable_citations", False)),
            "enable_research": bool(arguments.get("enable_research", False)),
        }
        headers = {
            "Accept": "application/json",
            "Content-Type": "application/json",
            "X-Subscription-Token": key,
        }

        try:
            resp = requests.post(
                ANSWER_API_URL,
                json=payload,
                headers=headers,
                proxies=_proxies() or None,
                timeout=60,
            )
            if resp.status_code != 200:
                return [
                    TextContent(
                        type="text",
                        text=json.dumps(
                            {
                                "error": "Brave Answers API request failed",
                                "status_code": resp.status_code,
                                "body": resp.text[:1200],
                            },
                            ensure_ascii=False,
                        ),
                    )
                ]

            data = resp.json()
            choices = data.get("choices", []) if isinstance(data, dict) else []
            message = choices[0].get("message", {}) if choices else {}
            answer = message.get("content", "") if isinstance(message, dict) else ""
            if not answer:
                return [
                    TextContent(
                        type="text",
                        text=json.dumps(
                            {"error": "Brave Answers API returned empty content"},
                            ensure_ascii=False,
                        ),
                    )
                ]

            return [
                TextContent(
                    type="text",
                    text=json.dumps(
                        {
                            "query": query,
                            "answer": answer,
                            "model": data.get("model", payload["model"]),
                            "usage": _usage_headers(resp.headers),
                        },
                        ensure_ascii=False,
                    ),
                )
            ]
        except Exception as e:
            return [TextContent(type="text", text=json.dumps({"error": str(e)}))]

    else:
        return [
            TextContent(
                type="text", text=json.dumps({"error": f"Unknown tool: {name}"})
            )
        ]


async def main() -> None:
    from mcp.server.stdio import stdio_server

    async with stdio_server() as (read_stream, write_stream):
        await server.run(
            read_stream, write_stream, server.create_initialization_options()
        )


if __name__ == "__main__":
    import asyncio

    asyncio.run(main())
