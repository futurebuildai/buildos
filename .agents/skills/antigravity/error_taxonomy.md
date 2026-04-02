---
description: Define the error code registry with categories (retriable, fatal, user-facing, internal). For each error, specify code, HTTP status, user message, internal message, logging level, and retry strategy. Define error propagation rules across layers.
invoked_from:
  - workflows/antigravity/07_architecture_spec.md
  - workflows/antigravity/feature_iteration.md
  - workflows/claude_code/scaffold_project.md
produces:
  - Error code registry with unique codes per error
  - Error categorization (retriable, fatal, user-facing, internal)
  - User-facing and internal message definitions
  - Logging level assignments
  - Retry strategy per error type
  - Error propagation rules across system layers
browser_usage: None (relies on architecture and domain inputs)
---

# Skill: Error Taxonomy

Define every error the system can produce. A well-designed error taxonomy makes debugging fast, user messages helpful, and monitoring actionable. Every error must have a unique code, a clear category, and a defined handling strategy.

---

## Prerequisites

Before invoking this skill, ensure the following exist:

- `TECH_STACK.md` — for API style (determines error format), language/framework (determines error patterns)
- `ARCHITECTURE.md` (partial) — for understanding system layers and external dependencies
- `api_contract_design.md` output (if already produced) — for HTTP status code mappings

---

## Step 1: Define Error Categories

### Primary Categories

| Category | Definition | User Impact | System Response |
|----------|-----------|-------------|-----------------|
| **Retriable** | Transient failure that may succeed on retry | Show retry option or auto-retry | Log as warning, auto-retry with backoff |
| **Fatal** | Unrecoverable failure requiring intervention | Show error page or critical message | Log as error, page on-call if systemic |
| **User-Facing** | Error caused by user input or action | Show specific correction guidance | Log as info or warning |
| **Internal** | System error not attributable to user | Show generic "something went wrong" | Log as error with full context |

### Secondary Tags

Errors may also be tagged with:

| Tag | Meaning | Example |
|-----|---------|---------|
| `idempotent_safe` | Retrying will not cause duplicate side effects | GET requests, idempotent PUTs |
| `rate_limited` | Error due to rate limiting, retry after cooldown | 429 responses |
| `dependency_failure` | External service or dependency is unavailable | Third-party API timeout |
| `data_integrity` | Data consistency violation detected | Unique constraint violation |
| `auth_failure` | Authentication or authorization problem | Expired token, insufficient permissions |
| `validation` | Input did not pass validation rules | Missing required field, invalid format |

---

## Step 2: Error Code Registry

### Code Format

Define a consistent error code format:

```
[DOMAIN]_[CATEGORY]_[SPECIFIC]
```

Examples:
- `AUTH_VALIDATION_EMAIL_INVALID`
- `ORDER_INTEGRITY_DUPLICATE`
- `PAYMENT_DEPENDENCY_GATEWAY_TIMEOUT`
- `USER_VALIDATION_PASSWORD_TOO_SHORT`

Rules:
- Error codes are **uppercase snake_case**
- Error codes are **stable** — once published, a code is never reused for a different error
- Error codes are **machine-readable** — clients can switch on error codes
- Error codes map 1:1 to specific error conditions (no catch-all codes)

### Registry

For **each** error in the system:

```markdown
### [ERROR_CODE]

| Property | Value |
|----------|-------|
| **Code** | `[ERROR_CODE]` |
| **HTTP Status** | [status code] |
| **Category** | Retriable / Fatal / User-Facing / Internal |
| **Tags** | [applicable secondary tags] |
| **User Message** | [Human-readable, actionable message safe to display in UI] |
| **Internal Message** | [Detailed technical message for logs and debugging] |
| **Logging Level** | DEBUG / INFO / WARN / ERROR / FATAL |
| **Retry Strategy** | [No retry / Immediate / Exponential backoff / After cooldown] |
| **Retry Max Attempts** | [N or N/A] |
| **Triggered By** | [What condition causes this error] |
| **Resolution** | [How to fix — for user errors: user action; for system errors: ops action] |
```

### Authentication and Authorization Errors

| Code | HTTP | Category | User Message | Retry |
|------|------|----------|-------------|-------|
| `AUTH_MISSING_TOKEN` | 401 | User-Facing | "Please sign in to continue." | No |
| `AUTH_INVALID_TOKEN` | 401 | User-Facing | "Your session has expired. Please sign in again." | No |
| `AUTH_TOKEN_EXPIRED` | 401 | Retriable | "Your session has expired. Please sign in again." | No (redirect to login) |
| `AUTH_INSUFFICIENT_PERMISSIONS` | 403 | User-Facing | "You do not have permission to perform this action." | No |
| `AUTH_ACCOUNT_LOCKED` | 403 | User-Facing | "Your account has been locked. Please contact support." | No |
| `AUTH_INVALID_CREDENTIALS` | 401 | User-Facing | "Invalid email or password." | No |
| `AUTH_MFA_REQUIRED` | 403 | User-Facing | "Multi-factor authentication is required." | No (redirect to MFA) |

### Validation Errors

| Code | HTTP | Category | User Message Pattern | Retry |
|------|------|----------|---------------------|-------|
| `VALIDATION_REQUIRED_FIELD` | 400 | User-Facing | "[Field] is required." | No |
| `VALIDATION_INVALID_FORMAT` | 400 | User-Facing | "[Field] must be a valid [format]." | No |
| `VALIDATION_OUT_OF_RANGE` | 400 | User-Facing | "[Field] must be between [min] and [max]." | No |
| `VALIDATION_STRING_TOO_LONG` | 400 | User-Facing | "[Field] must not exceed [max] characters." | No |
| `VALIDATION_STRING_TOO_SHORT` | 400 | User-Facing | "[Field] must be at least [min] characters." | No |
| `VALIDATION_INVALID_ENUM` | 400 | User-Facing | "[Field] must be one of: [values]." | No |
| `VALIDATION_BODY_PARSE_ERROR` | 400 | User-Facing | "The request body could not be parsed. Please check the format." | No |

### Resource Errors

| Code | HTTP | Category | User Message | Retry |
|------|------|----------|-------------|-------|
| `RESOURCE_NOT_FOUND` | 404 | User-Facing | "The requested [resource] was not found." | No |
| `RESOURCE_ALREADY_EXISTS` | 409 | User-Facing | "A [resource] with this [field] already exists." | No |
| `RESOURCE_CONFLICT` | 409 | User-Facing | "This [resource] has been modified. Please refresh and try again." | Yes (after refresh) |
| `RESOURCE_GONE` | 410 | User-Facing | "This [resource] is no longer available." | No |
| `RESOURCE_LOCKED` | 423 | Retriable | "This [resource] is currently being modified. Please try again." | Yes (backoff) |

### Rate Limiting Errors

| Code | HTTP | Category | User Message | Retry |
|------|------|----------|-------------|-------|
| `RATE_LIMIT_EXCEEDED` | 429 | Retriable | "Too many requests. Please wait a moment and try again." | Yes (after `Retry-After`) |
| `RATE_LIMIT_QUOTA_EXCEEDED` | 429 | User-Facing | "You have exceeded your [plan] quota. Please upgrade." | No |

### Dependency Errors

| Code | HTTP | Category | User Message | Retry |
|------|------|----------|-------------|-------|
| `DEPENDENCY_TIMEOUT` | 504 | Retriable | "The request timed out. Please try again." | Yes (exponential backoff, max 3) |
| `DEPENDENCY_UNAVAILABLE` | 503 | Retriable | "A required service is temporarily unavailable. Please try again later." | Yes (exponential backoff, max 3) |
| `DEPENDENCY_INVALID_RESPONSE` | 502 | Internal | "Something went wrong. Please try again." | Yes (1 retry) |
| `DEPENDENCY_CIRCUIT_OPEN` | 503 | Retriable | "This feature is temporarily unavailable. Please try again later." | Yes (after circuit reset) |

### Internal Errors

| Code | HTTP | Category | User Message | Retry |
|------|------|----------|-------------|-------|
| `INTERNAL_UNEXPECTED` | 500 | Internal | "An unexpected error occurred. Please try again." | Yes (1 retry) |
| `INTERNAL_DATABASE_ERROR` | 500 | Internal | "An unexpected error occurred. Please try again." | Yes (1 retry) |
| `INTERNAL_CONFIGURATION_ERROR` | 500 | Fatal | "The service is misconfigured. Please contact support." | No |
| `INTERNAL_DATA_INTEGRITY` | 500 | Fatal | "An unexpected error occurred. Please contact support." | No |

---

## Step 3: Error Response Format

### REST API Error Response

```json
{
  "errors": [
    {
      "code": "VALIDATION_REQUIRED_FIELD",
      "message": "Email is required.",
      "field": "email",
      "details": {
        "constraint": "required"
      }
    }
  ],
  "meta": {
    "request_id": "req_abc123",
    "timestamp": "2024-01-01T00:00:00Z"
  }
}
```

Rules:
- `errors` is always an array (even for single errors)
- `code` is the machine-readable error code from the registry
- `message` is the user-safe message (never internal details)
- `field` is present only for validation errors
- `details` is optional, machine-readable context for programmatic handling
- `meta.request_id` is always present for traceability

### gRPC Error Response

Map errors to gRPC status codes:

| Error Category | gRPC Status | Details Message |
|---------------|-------------|-----------------|
| Validation | `INVALID_ARGUMENT` | Field-level details |
| Not found | `NOT_FOUND` | Resource type + ID |
| Auth (unauthenticated) | `UNAUTHENTICATED` | N/A |
| Auth (unauthorized) | `PERMISSION_DENIED` | Required permission |
| Rate limit | `RESOURCE_EXHAUSTED` | Retry-after metadata |
| Internal | `INTERNAL` | Request ID only |
| Dependency timeout | `DEADLINE_EXCEEDED` | Service name |
| Dependency unavailable | `UNAVAILABLE` | Service name |

---

## Step 4: Error Propagation Rules

### Layer Responsibilities

```
┌─────────────────────────────────┐
│  API / Transport Layer          │  Catches all errors, formats response
│  (HTTP handlers, gRPC services) │  Maps domain errors → HTTP/gRPC status
├─────────────────────────────────┤
│  Application / Service Layer    │  Catches domain + infra errors
│                                 │  Wraps with context, re-throws as domain errors
├─────────────────────────────────┤
│  Domain Layer                   │  Throws domain-specific errors
│                                 │  Never exposes infrastructure details
├─────────────────────────────────┤
│  Infrastructure / Repository    │  Catches driver/connection errors
│                                 │  Translates to domain-relevant errors
└─────────────────────────────────┘
```

### Propagation Rules

1. **Infrastructure layer** catches raw errors (SQL errors, HTTP client errors, filesystem errors) and translates them into domain-meaningful errors:
   - `unique_violation` SQL error becomes `RESOURCE_ALREADY_EXISTS`
   - `connection_refused` becomes `DEPENDENCY_UNAVAILABLE`
   - `timeout` becomes `DEPENDENCY_TIMEOUT`

2. **Domain layer** throws errors using domain language:
   - `OrderAlreadyShipped` — not `RESOURCE_CONFLICT`
   - `InsufficientInventory` — not `VALIDATION_OUT_OF_RANGE`
   - Domain errors carry domain context (entity ID, business rule violated)

3. **Application/service layer** catches domain errors and wraps them with additional context:
   - Adds request context (user ID, action attempted)
   - Decides on retry strategy
   - May aggregate multiple errors into a single response

4. **API/transport layer** maps domain errors to transport-level status codes:
   - Domain error codes become the `code` field in the response
   - HTTP status is derived from the error category
   - Internal details are stripped from the user-facing message
   - Request ID is attached for traceability

### Error Wrapping Pattern

```
Original error:     "connection refused: postgres:5432"
Infrastructure:   → DatabaseConnectionError("Failed to connect to primary database")
Service layer:    → ServiceError("Failed to create user", cause=DatabaseConnectionError)
API layer:        → HTTP 500 { code: "INTERNAL_DATABASE_ERROR", message: "An unexpected error occurred." }
Log entry:        → ERROR [req_abc123] Failed to create user: Failed to connect to primary database: connection refused: postgres:5432
```

Key rules:
- **Never expose infrastructure details to the client** (no connection strings, table names, stack traces)
- **Always preserve the full error chain in logs** (wrap, do not replace)
- **Always include request_id** in both the response and the log entry

---

## Step 5: Retry Strategy Definitions

### Retry Configurations

| Strategy | Initial Delay | Backoff Factor | Max Delay | Max Attempts | Jitter |
|----------|-------------|---------------|-----------|-------------|--------|
| Immediate | 0ms | N/A | N/A | 1 | No |
| Fast backoff | 100ms | 2x | 1s | 3 | Yes (0-100ms) |
| Standard backoff | 500ms | 2x | 10s | 3 | Yes (0-500ms) |
| Slow backoff | 2s | 2x | 60s | 5 | Yes (0-2s) |
| Rate limit cooldown | `Retry-After` header value | N/A | N/A | 1 | No |

### Client-Side Retry Rules

1. Only retry errors tagged as `retriable`
2. Never retry errors that are not idempotent-safe (non-idempotent POST without idempotency key)
3. Always add jitter to prevent thundering herd
4. Respect `Retry-After` headers when present
5. After max retries exhausted, surface the error to the user
6. Circuit breaker: if a dependency fails N consecutive requests, stop retrying for a cooldown period

### Idempotency

For write operations that may be retried:
- Require an `Idempotency-Key` header (client-generated UUID)
- Server stores the response for the idempotency key for a TTL (e.g., 24 hours)
- On duplicate request with same key, return the stored response without re-executing

---

## Step 6: Logging Standards

### Log Format per Error Level

| Level | When to Use | Includes |
|-------|------------|----------|
| `DEBUG` | Detailed diagnostic info during development | Full request/response, variable state |
| `INFO` | Normal operations worth recording | User action, request_id, result |
| `WARN` | Recoverable issues, potential problems | Error code, request_id, context, retry attempted |
| `ERROR` | Failed operations requiring attention | Full error chain, request_id, stack trace, user context |
| `FATAL` | System cannot continue operating | Everything in ERROR + shutdown context |

### Required Log Fields for Errors

Every error log entry must include:

```json
{
  "level": "ERROR",
  "timestamp": "2024-01-01T00:00:00.000Z",
  "request_id": "req_abc123",
  "error_code": "DEPENDENCY_TIMEOUT",
  "message": "Payment gateway request timed out",
  "user_id": "usr_xyz789",
  "action": "create_order",
  "duration_ms": 30000,
  "stack_trace": "...",
  "cause": "connection timeout after 30s to payments.example.com",
  "retry_attempt": 2,
  "retry_max": 3
}
```

---

## Step 7: Cross-Reference Validation

Before finalizing, verify:

- [ ] Every API endpoint has its possible errors mapped (cross-reference with `api_contract_design.md`)
- [ ] Every external dependency has timeout/unavailable/invalid-response errors defined
- [ ] Every validation rule from the domain model has a corresponding validation error code
- [ ] No two different errors share the same error code
- [ ] User-facing messages are helpful and do not leak internal details
- [ ] Retry strategies are defined for all retriable errors
- [ ] Logging levels are assigned to every error
- [ ] Error propagation rules cover all layer boundaries

---

## Output

The final output feeds into the Error Handling section of `ARCHITECTURE.md` and serves as the reference for implementing error handling code, API error responses, client-side error display, and monitoring/alerting rules.
