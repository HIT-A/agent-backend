# Client-Side API Proposal (SSOT Companion)

**Purpose:** Provide a client-facing proposal that mirrors SSOT constraints so Android and server teams implement the same contract.

**Scope (v0):**
- `campus_id` requirement for public skills
- invoke/jobs semantics
- privacy boundary for request/trace bodies
- cache/TTL behavior expectations

---

## 1) Required Fields (Client MUST send)

- **`campus_id`** on all public skill requests (server-side skills). Default requirement.
  - Only optional for truly global resources (explicitly documented per-skill).

---

## 2) Invoke Semantics (Client MUST follow)

**POST `/v1/skills/{name}:invoke`**
- Always **HTTP 200** for POST success/failed.
- Success:
  - Sync: `{ ok:true, output:{...} }`
  - Async: `{ ok:true, job_id:"..." }`
- Failure: `{ ok:false, error:{ code, message, retryable } }`
- **Skill not found** also returns HTTP 200 with `ok:false` and `error.code=SKILL_NOT_FOUND`.

**Client rule:**
- If `is_async=true`, treat response as async and always fetch result from `/v1/jobs/{id}`.

---

## 3) Jobs Semantics (Client MUST follow)

**GET `/v1/jobs/{id}`**
- Found: HTTP 200, `{ ok:true, job:{...} }`
- Not found: HTTP 404 (body may include `{ ok:false, error:{ code:"JOB_NOT_FOUND" } }`)

**Job fields:**
- `input_json` / `output_json` are **any JSON type** (object/array/string/number/null).
- `output_json` only present for `succeeded` (else `null`).
- `error` only present for `failed` (else `null`).

---

## 4) Privacy & Trace Rules (Client MUST follow)

- No P0 data in any server request body: **no passwords, cookies, tokens, student ID, grades, raw timetable**.
- Trace is metadata-only; **never include request/response bodies** or personal fields.
- If user explicitly enables "optional upload", only send **P1 redacted summaries**.

---

## 5) Caching + TTL (Client MUST follow)

- Cache expiry computed as: `expires_at = cached_at + ttl_seconds`.
- Fetch failures should return stale cached data with `stale=true` and error.
- Suggested polling backoff for async jobs: `0.5s → 1s → 2s → 5s`, cap 5s, max 2 minutes.

---

## 6) Campus-Aware Public Skills (Examples)

**rag.query**
```json
{
  "campus_id": "HITSZ|HITH|HITWH",
  "query": "string",
  "top_k": 10,
  "filters": {}
}
```

**hoa.courses.lookup**
```json
{
  "campus_id": "HITSZ|HITH|HITWH",
  "query": "string"
}
```

**hoa.courses.submit_ops** (async)
```json
{
  "campus_id": "HITSZ|HITH|HITWH",
  "items": [
    { "course_id": "string", "name": "string", "action": "create|update|delete", "payload": {} }
  ]
}
```

---

## 7) Client Responsibilities Summary

- Keep EAS auth/sessions and any private skills **on-device**.
- Use Orchestrator to call local (private) and remote (public) skills.
- Enforce `campus_id` for public calls, use per-campus repos.
- Respect retry/backoff rules to avoid server overload.

---

**Change policy:** This proposal is **v0** and should only change via SSOT updates.
