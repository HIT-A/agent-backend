#!/usr/bin/env python3
import argparse
import json
import sys
import time
import urllib.error
import urllib.request
from typing import Any, Dict, Tuple


def http_json(method: str, url: str, body: Dict[str, Any] | None, timeout: float) -> Tuple[int, Dict[str, Any]]:
    data = None
    headers = {"Content-Type": "application/json"}
    if body is not None:
        data = json.dumps(body, ensure_ascii=True).encode("utf-8")
    req = urllib.request.Request(url=url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
            try:
                return resp.status, json.loads(raw) if raw else {}
            except json.JSONDecodeError:
                return resp.status, {"_raw": raw}
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", errors="replace")
        try:
            payload = json.loads(raw) if raw else {}
        except json.JSONDecodeError:
            payload = {"_raw": raw}
        return e.code, payload


def ok(cond: bool, message: str) -> bool:
    if cond:
        print(f"[PASS] {message}")
        return True
    print(f"[FAIL] {message}")
    return False


def main() -> int:
    parser = argparse.ArgumentParser(description="agent-backend E2E smoke test")
    parser.add_argument("--base-url", default="http://127.0.0.1:8080", help="agent-backend base URL")
    parser.add_argument("--poll-interval", type=float, default=0.3, help="job polling interval seconds")
    parser.add_argument("--poll-timeout", type=float, default=20.0, help="job polling timeout seconds")
    parser.add_argument("--timeout", type=float, default=8.0, help="single request timeout seconds")
    args = parser.parse_args()

    base = args.base_url.rstrip("/")
    passed = 0
    total = 0

    # 1) health
    total += 1
    code, payload = http_json("GET", f"{base}/health", None, args.timeout)
    if ok(code == 200 and payload.get("status") == "ok", f"GET /health -> 200 and status=ok (got code={code})"):
        passed += 1

    # 2) skills list
    total += 1
    code, payload = http_json("GET", f"{base}/v1/skills", None, args.timeout)
    skill_names = set()
    if isinstance(payload.get("skills"), list):
        for s in payload["skills"]:
            if isinstance(s, dict) and isinstance(s.get("name"), str):
                skill_names.add(s["name"])
    required = {"echo", "sleep_echo", "pr.lookup"}
    if ok(code == 200 and required.issubset(skill_names), f"GET /v1/skills includes {sorted(required)}"):
        passed += 1

    # 3) sync invoke: echo
    total += 1
    echo_req = {
        "input": {"msg": "hello-e2e", "n": 1},
        "trace": {"case": "sync-echo"},
    }
    code, payload = http_json("POST", f"{base}/v1/skills/echo:invoke", echo_req, args.timeout)
    cond_echo = (
        code == 200
        and payload.get("ok") is True
        and isinstance(payload.get("output"), dict)
        and isinstance(payload["output"].get("input"), dict)
        and payload["output"]["input"].get("msg") == "hello-e2e"
    )
    if ok(cond_echo, "POST /v1/skills/echo:invoke returns expected output"):
        passed += 1

    # 4) async invoke + poll job: sleep_echo
    total += 1
    sleep_req = {
        "input": {"sleep_ms": 80, "msg": "async-e2e"},
        "trace": {"case": "async-sleep-echo"},
    }
    code, payload = http_json("POST", f"{base}/v1/skills/sleep_echo:invoke", sleep_req, args.timeout)
    job_id = payload.get("job_id") if isinstance(payload, dict) else None
    cond_enqueue = code == 200 and payload.get("ok") is True and isinstance(job_id, str) and len(job_id) > 0

    if not ok(cond_enqueue, "POST /v1/skills/sleep_echo:invoke enqueues async job"):
        print("[INFO] cannot continue async polling because job_id is missing")
    else:
        end_at = time.time() + args.poll_timeout
        final_job = None
        while time.time() < end_at:
            j_code, j_payload = http_json("GET", f"{base}/v1/jobs/{job_id}", None, args.timeout)
            if j_code == 200 and isinstance(j_payload.get("job"), dict):
                job = j_payload["job"]
                status = job.get("status")
                if status in {"succeeded", "failed"}:
                    final_job = job
                    break
            time.sleep(args.poll_interval)

        if final_job is None:
            ok(False, f"GET /v1/jobs/{{id}} reaches terminal status within {args.poll_timeout}s")
        else:
            status = final_job.get("status")
            output_json = final_job.get("output_json")
            cond_async = status == "succeeded" and isinstance(output_json, dict)
            if cond_async:
                cond_async = output_json.get("input", {}).get("msg") == "async-e2e"
            if ok(cond_async, f"async job completed with succeeded status (status={status})"):
                passed += 1

    # 5) pr-server chain through pr.lookup
    total += 1
    pr_req = {
        "input": {"org": "HIT-A", "repo": "all_api", "pr": 1},
        "trace": {"case": "pr-lookup-chain"},
    }
    code, payload = http_json("POST", f"{base}/v1/skills/pr.lookup:invoke", pr_req, args.timeout)

    cond_pr = False
    if code == 200 and isinstance(payload, dict) and "ok" in payload:
        if payload.get("ok") is True:
            cond_pr = True
        else:
            err = payload.get("error") if isinstance(payload.get("error"), dict) else {}
            msg = str(err.get("message", ""))
            # Allow business errors (e.g. PR_NOT_FOUND), but fail if transport/config errors.
            transport_markers = ["pr-server error", "connect", "timeout", "refused", "no such host", "dial tcp"]
            cond_pr = not any(m in msg.lower() for m in transport_markers)

    if ok(cond_pr, "POST /v1/skills/pr.lookup:invoke reaches pr-server without transport/config errors"):
        passed += 1

    print("\n=== Summary ===")
    print(f"Passed: {passed}/{total}")
    if passed != total:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
