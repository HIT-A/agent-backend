#!/usr/bin/env python3
import json
import os
from typing import Any, Dict, Iterable, List

import requests
from mcp.server import Server
from mcp.types import TextContent, Tool

server = Server("bocha-search-local")

API_URL = "https://api.bocha.cn/v1/web-search"
ANSWER_API_URL = "https://api.search.brave.com/res/v1/chat/completions"


def _api_key() -> str:
    return (
        os.environ.get("BOCHA_API_KEY", "")
        or os.environ.get("BRAVE_API_KEY", "")
        or os.environ.get("BRAVE_SEARCH_API_KEY", "")
    )


def _answer_key() -> str:
    return os.environ.get("BRAVE_ANSWER_API_KEY", "")


def _proxies() -> Dict[str, str]:
    proxies: Dict[str, str] = {}
    for env_name, proxy_key in (
        ("HTTP_PROXY", "http"),
        ("HTTPS_PROXY", "https"),
        ("ALL_PROXY", "all"),
    ):
        value = os.environ.get(env_name)
        if value:
            proxies[proxy_key] = value
    return proxies


def _usage_headers(headers: Dict[str, Any]) -> Dict[str, Any]:
    usage: Dict[str, Any] = {}
    for key, value in headers.items():
        lower = key.lower()
        if lower.startswith("x-request-"):
            usage[key] = value
    return usage


def _pick_results(data: Any) -> List[Dict[str, str]]:
    candidates: Iterable[Any] = []
    if isinstance(data, dict):
        for key in ("data", "results", "items", "web", "webPages", "list"):
            value = data.get(key)
            if isinstance(value, dict):
                web_pages = value.get("webPages")
                if isinstance(web_pages, dict) and isinstance(web_pages.get("value"), list):
                    candidates = web_pages.get("value", [])
                    break
                if isinstance(value.get("results"), list):
                    candidates = value.get("results", [])
                    break
                if isinstance(value.get("items"), list):
                    candidates = value.get("items", [])
                    break
                if isinstance(value.get("value"), list):
                    candidates = value.get("value", [])
                    break
                if isinstance(value.get("list"), list):
                    candidates = value.get("list", [])
                    break
            elif isinstance(value, list):
                candidates = value
                break
    elif isinstance(data, list):
        candidates = data

    results: List[Dict[str, str]] = []
    for item in candidates:
        if not isinstance(item, dict):
            continue
        title = str(item.get("title") or item.get("name") or item.get("headline") or "").strip()
        url = str(item.get("url") or item.get("link") or item.get("href") or item.get("sourceUrl") or "").strip()
        desc = str(
            item.get("description")
            or item.get("snippet")
            or item.get("summary")
            or item.get("content")
            or ""
        ).strip()
        if title or url or desc:
            results.append({"title": title, "url": url, "description": desc})
    return results


@server.list_tools()
async def list_tools() -> List[Tool]:
    return [
        Tool(
            name="brave_web_search",
            description="Search the web with Bocha web-search API.",
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
                    "summary": {
                        "type": "boolean",
                        "description": "Whether to request a summary from Bocha",
                        "default": True,
                    },
                    "freshness": {
                        "type": "string",
                        "description": "Freshness filter like noLimit, day, week, month",
                        "default": "noLimit",
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
                    type="text", text=json.dumps({"error": "Missing BOCHA_API_KEY"})
                )
            ]

        query = arguments.get("query", "")
        count = max(1, min(int(arguments.get("count", 5)), 20))

        payload: Dict[str, Any] = {
            "query": query,
            "summary": bool(arguments.get("summary", True)),
            "freshness": arguments.get("freshness", "noLimit"),
            "count": count,
        }
        country = arguments.get("country")
        if country:
            payload["country"] = country
        search_lang = arguments.get("search_lang")
        if search_lang:
            payload["search_lang"] = search_lang

        headers = {
            "Accept": "application/json",
            "Authorization": f"Bearer {key}",
            "Content-Type": "application/json",
        }

        try:
            resp = requests.post(
                API_URL,
                json=payload,
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
                                "error": "Bocha API request failed",
                                "status_code": resp.status_code,
                                "body": resp.text[:1200],
                            }
                        ),
                    )
                ]

            data = resp.json()
            results = _pick_results(data)
            if not results:
                return [
                    TextContent(
                        type="text",
                        text=json.dumps(
                            {
                                "query": query,
                                "count": 0,
                                "results": [],
                                "warning": "Bocha API returned no recognizable search results",
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
                            "count": len(results),
                            "results": results,
                            "raw_top_keys": list(data.keys())
                            if isinstance(data, dict)
                            else [],
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
