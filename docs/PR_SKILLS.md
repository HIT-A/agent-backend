# PR Skills Integration

## Overview

Three PR (Pull Request) skills have been implemented in `hoa-agent-backend` to integrate with `pr-server`:

- `pr.preview` - Preview course material changes
- `pr.submit` - Submit course material changes via PR
- `pr.lookup` - Query PR status

2026-03-18 alignment:
- `append_course_review` has been removed and is no longer generated/forwarded.
- Generic sub-course review is normalized to `append_course_section_item`.
- Teacher review is normalized to `add_course_teacher_review`.
- For `harbin`/`weihai`, multi-project ops are blocked with `INVALID_OPS`.
- Existing normal ops remain unchanged.

## Architecture

```
端侧 Orchestrator
      ↓
POST /v1/skills/pr.preview:invoke
      ↓
hoa-agent-backend
      ↓
HTTP → pr-server (PR_SERVER_URL)
      ↓
GitHub API → GitHub
```

## Skills

### Multi-Project Operation Routing

Before forwarding to pr-server, `pr.preview` and `pr.submit` apply an ops normalization layer:

- Removed op:
  - `append_course_review`
- Rewrites:
  - generic course review -> `append_course_section_item` (default `section_title` is `课程评价`)
  - teacher-specific review -> `add_course_teacher_review`
- Campus guard:
  - `shenzhen` allows multi-project ops
  - `harbin`/`weihai` reject multi-project ops with `INVALID_OPS`

### 1. pr.preview

Preview course material changes by applying TOML operations and rendering README.md.

**Input:**
```json
{
  "campus": "shenzhen",
  "course_code": "COMP1011",
  "ops": [
    {
      "op": "add_lecturer_review",
      "lecturer_name": "Alice Smith",
      "content": "Great professor!",
      "author": {
        "name": "Student A",
        "link": "https://example.com",
        "date": "2025-01-15"
      }
    }
  ]
}
```

**Output:**
```json
{
  "ok": true,
  "output": {
    "base": {
      "org": "HITSZ-OpenAuto",
      "repo": "COMP1011",
      "ref": "main",
      "toml_path": "readme.toml"
    },
    "result": {
      "readme_toml": "course_name = 'Test Course'...",
      "readme_md": "# Test Course\n..."
    },
    "summary": {
      "changed_files": ["readme.toml", "README.md"],
      "warnings": []
    }
  }
}
```

### 2. pr.submit

Submit course material changes to GitHub by creating a PR.

**Input:**
```json
{
  "campus": "shenzhen",
  "course_code": "COMP1011",
  "ops": [...]
}
```

**Output:**
```json
{
  "ok": true,
  "output": {
    "pr_number": 123,
    "pr_url": "https://github.com/HITSZ-OpenAuto/COMP1011/pull/123",
    "branch": "update-comp1011-abc123"
  }
}
```

### 3. pr.lookup

Query the status of a PR.

**Input:**
```json
{
  "org": "HITSZ-OpenAuto",
  "repo": "COMP1011",
  "pr": 123
}
```

**Output:**
```json
{
  "ok": true,
  "output": {
    "number": 123,
    "state": "open",
    "title": "Update COMP1011 course materials",
    "url": "https://github.com/HITSZ-OpenAuto/COMP1011/pull/123"
  }
}
```

## Configuration

### Environment Variables

- `PR_SERVER_URL` - Base URL of pr-server (default: `http://localhost:8080`)

### Example

```bash
export PR_SERVER_URL=http://pr-server.internal:8080
```

In current deployment, pr-server is typically configured via `/etc/agent-backend.env`.

## Error Handling

PR skills follow SSOT contract semantics:

- **Always HTTP 200** - Success/failure is encoded in the response body
- **Error codes:**
  - `INVALID_INPUT` - Missing or invalid input parameters
  - `INTERNAL` - Internal server error (retryable)
  - `NOT_FOUND` - Resource not found

## Error Mapping

pr-server error codes are mapped to agent-backend error codes:

| pr-server Code | agent-backend Code | Retryable |
|----------------|-------------------|------------|
| TOML_SCHEMA_ERROR | INVALID_INPUT | false |
| RENDER_FAILED | INTERNAL | true |
| REPO_NOT_FOUND | NOT_FOUND | false |
| TOML_NOT_FOUND | NOT_FOUND | false |
| CONFIG_ERROR | INTERNAL | false |
| BRANCH_NOT_FOUND | NOT_FOUND | false |
| INVALID_JSON | INVALID_INPUT | false |
| MISSING_TARGET | INVALID_INPUT | false |
| INVALID_OPS | INVALID_INPUT | false |

## Testing

Run tests:

```bash
# Run all skills tests
go test ./internal/skills -v

# Run only PR skills tests
go test ./internal/skills -v -run "PR"

# Run in short mode (skip integration tests)
go test ./internal/skills -short -v
```

## Implementation Details

### Files

- `internal/pr/client.go` - HTTP client for pr-server
- `internal/skills/pr_skills.go` - PR skill implementations
- `internal/skills/pr_skills_test.go` - Tests for PR skills
- `internal/skills/registry.go` - Updated to register PR skills

### Dependencies

- pr-server (external service)
- Go standard library (net/http, encoding/json, context)

## Future Enhancements

- [ ] PR listing skill
- [ ] PR merge status monitoring
- [ ] Batch operations
