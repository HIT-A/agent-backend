#!/usr/bin/env python3
import sys
import json
import subprocess


def send_message(msg):
    data = json.dumps(msg)
    sys.stdout.write(f"Content-Length: {len(data)}\r\n\r\n{data}")
    sys.stdout.flush()


def read_message():
    try:
        headers = {}
        while True:
            line = sys.stdin.readline()
            if not line:
                return None
            if line in ("\r\n", "\n", ""):
                break
            if ":" in line:
                k, v = line.split(":", 1)
                headers[k.strip().lower()] = v.strip()

        if "content-length" in headers:
            length = int(headers["content-length"])
            body = sys.stdin.read(length)
            if not body:
                return None
            return json.loads(body)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
    return None


def crawl_docker(url):
    try:
        script = f"""
from crawl4ai import AsyncWebCrawler
import asyncio

async def crawl():
    async with AsyncWebCrawler() as crawler:
        result = await crawler.arun(url='{url}')
        print(result.markdown[:5000])

asyncio.run(crawl())
"""
        cmd = [
            "docker",
            "run",
            "--rm",
            "-i",
            "unclecode/crawl4ai:latest",
            "python",
            "-c",
            script,
        ]
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            return {"content": result.stdout, "isError": False}
        return {"content": result.stderr, "isError": True}
    except subprocess.TimeoutExpired:
        return {"content": "Timeout", "isError": True}
    except Exception as e:
        return {"content": str(e), "isError": True}


def main():
    while True:
        msg = read_message()
        if msg is None:
            break

        method = msg.get("method", "")
        msg_id = msg.get("id")

        if method == "initialize":
            send_message(
                {
                    "jsonrpc": "2.0",
                    "id": msg_id,
                    "result": {
                        "protocolVersion": "2024-11-05",
                        "capabilities": {},
                        "serverInfo": {"name": "crawl4ai-mcp", "version": "0.1.0"},
                    },
                }
            )
        elif method == "tools/list":
            send_message(
                {
                    "jsonrpc": "2.0",
                    "id": msg_id,
                    "result": {
                        "tools": [
                            {
                                "name": "crawl_page",
                                "description": "Crawl a web page",
                                "inputSchema": {
                                    "type": "object",
                                    "properties": {"url": {"type": "string"}},
                                    "required": ["url"],
                                },
                            }
                        ]
                    },
                }
            )
        elif method == "tools/call":
            params = msg.get("params", {})
            if params.get("name") == "crawl_page":
                url = params.get("arguments", {}).get("url", "")
                result = crawl_docker(url)
                send_message(
                    {
                        "jsonrpc": "2.0",
                        "id": msg_id,
                        "result": {
                            "content": [{"type": "text", "text": result["content"]}],
                            "isError": result["isError"],
                        },
                    }
                )
            else:
                send_message(
                    {
                        "jsonrpc": "2.0",
                        "id": msg_id,
                        "error": {"code": -32601, "message": "Unknown tool"},
                    }
                )
        elif method == "notifications/initialized":
            pass
        elif msg_id:
            send_message(
                {
                    "jsonrpc": "2.0",
                    "id": msg_id,
                    "error": {"code": -32601, "message": f"Unknown: {method}"},
                }
            )


if __name__ == "__main__":
    main()
