#!/usr/bin/env python3
import json
import re
import xml.etree.ElementTree as ET
from typing import Any, Dict, List
from urllib.parse import quote_plus

import requests
from mcp.server import Server
from mcp.types import TextContent, Tool

server = Server("arxiv-local")

ARXIV_API = "https://export.arxiv.org/api/query"
ATOM_NS = {"atom": "http://www.w3.org/2005/Atom"}


def _proxies() -> Dict[str, str]:
    import os

    p: Dict[str, str] = {}
    hp = os.environ.get("HTTP_PROXY")
    sp = os.environ.get("HTTPS_PROXY")
    if hp:
        p["http"] = hp
    if sp:
        p["https"] = sp
    return p


def _clean_text(v: str) -> str:
    return re.sub(r"\s+", " ", (v or "").strip())


def _parse_entries(xml_text: str) -> List[Dict[str, Any]]:
    root = ET.fromstring(xml_text)
    out: List[Dict[str, Any]] = []
    for e in root.findall("atom:entry", ATOM_NS):
        title = _clean_text(e.findtext("atom:title", default="", namespaces=ATOM_NS))
        summary = _clean_text(e.findtext("atom:summary", default="", namespaces=ATOM_NS))
        published = _clean_text(e.findtext("atom:published", default="", namespaces=ATOM_NS))
        updated = _clean_text(e.findtext("atom:updated", default="", namespaces=ATOM_NS))

        authors = []
        for a in e.findall("atom:author", ATOM_NS):
            name = _clean_text(a.findtext("atom:name", default="", namespaces=ATOM_NS))
            if name:
                authors.append(name)

        links = e.findall("atom:link", ATOM_NS)
        entry_url = ""
        pdf_url = ""
        for l in links:
            href = l.attrib.get("href", "")
            rel = l.attrib.get("rel", "")
            ltype = l.attrib.get("type", "")
            if rel == "alternate" and href:
                entry_url = href
            if ltype == "application/pdf" and href:
                pdf_url = href

        arxiv_id = ""
        id_text = _clean_text(e.findtext("atom:id", default="", namespaces=ATOM_NS))
        if "/abs/" in id_text:
            arxiv_id = id_text.rsplit("/abs/", 1)[-1]

        out.append(
            {
                "id": arxiv_id,
                "title": title,
                "summary": summary,
                "authors": authors,
                "published": published,
                "updated": updated,
                "url": entry_url,
                "pdf_url": pdf_url,
            }
        )
    return out


@server.list_tools()
async def list_tools() -> List[Tool]:
    return [
        Tool(
            name="search_arxiv",
            description="Search papers from arXiv.",
            inputSchema={
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search query"},
                    "max_results": {
                        "type": "integer",
                        "description": "Number of papers to return",
                        "default": 5,
                    },
                    "start": {
                        "type": "integer",
                        "description": "Pagination start offset",
                        "default": 0,
                    },
                    "sort_by": {
                        "type": "string",
                        "enum": ["relevance", "lastUpdatedDate", "submittedDate"],
                        "default": "relevance",
                    },
                    "sort_order": {
                        "type": "string",
                        "enum": ["ascending", "descending"],
                        "default": "descending",
                    },
                },
                "required": ["query"],
            },
        )
    ]


@server.call_tool()
async def call_tool(name: str, arguments: Dict[str, Any]) -> List[TextContent]:
    if name != "search_arxiv":
        return [TextContent(type="text", text=json.dumps({"error": f"Unknown tool: {name}"}))]

    query = str(arguments.get("query", "")).strip()
    if not query:
        return [TextContent(type="text", text=json.dumps({"error": "query is required"}))]

    max_results = max(1, min(int(arguments.get("max_results", 5)), 50))
    start = max(0, int(arguments.get("start", 0)))
    sort_by = str(arguments.get("sort_by", "relevance"))
    sort_order = str(arguments.get("sort_order", "descending"))

    params = {
        "search_query": f"all:{query}",
        "start": start,
        "max_results": max_results,
        "sortBy": sort_by,
        "sortOrder": sort_order,
    }

    try:
        resp = requests.get(
            ARXIV_API,
            params=params,
            timeout=30,
            proxies=_proxies() or None,
            headers={"Accept": "application/atom+xml"},
        )
        if resp.status_code != 200:
            return [
                TextContent(
                    type="text",
                    text=json.dumps(
                        {
                            "error": "arXiv API request failed",
                            "status_code": resp.status_code,
                            "body": resp.text[:1000],
                        }
                    ),
                )
            ]

        papers = _parse_entries(resp.text)
        return [
            TextContent(
                type="text",
                text=json.dumps(
                    {
                        "query": query,
                        "start": start,
                        "max_results": max_results,
                        "returned": len(papers),
                        "papers": papers,
                    },
                    ensure_ascii=False,
                ),
            )
        ]
    except Exception as e:
        return [TextContent(type="text", text=json.dumps({"error": str(e)}))]


async def main() -> None:
    from mcp.server.stdio import stdio_server

    async with stdio_server() as (read_stream, write_stream):
        await server.run(read_stream, write_stream, server.create_initialization_options())


if __name__ == "__main__":
    import asyncio

    asyncio.run(main())
