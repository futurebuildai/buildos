---
description: Design API contracts from the consumer's perspective. Produces endpoint inventory, request/response schemas, auth model, rate limiting, versioning strategy, error format, and pagination design. Output in OpenAPI or protobuf format where applicable.
invoked_from:
  - workflows/antigravity/07_architecture_spec.md
  - workflows/antigravity/feature_iteration.md
produces:
  - API endpoint inventory with full request/response schemas
  - Authentication and authorization model
  - Rate limiting policy
  - Versioning strategy
  - Error response format
  - Pagination design
  - OpenAPI 3.1 or protobuf definitions (per TECH_STACK.md)
browser_usage: Moderate (research API design best practices, study exemplary public APIs in the domain)
---

# Skill: API Contract Design

Design API contracts consumer-first. Every endpoint must be justified by a user story or system requirement from the PRD. The contract is the binding agreement between frontend and backend — precision here prevents integration pain later.

---

## Prerequisites

Before invoking this skill, ensure the following exist:

- `TECH_STACK.md` — for API style (REST/gRPC/GraphQL/tRPC), spec format, and auth model
- `PRD.md` or `SCOPE_DEFINITION.md` — for user stories that drive endpoint needs
- `DESIGN_SYSTEM.md` / `INFORMATION_ARCHITECTURE.md` — for understanding what data the UI needs

---

## Step 1: Research API Patterns

**Browser: 3-5 searches**

1. Search for "[API style from TECH_STACK.md] API design best practices [current year]"
2. Search for "[product domain] API design patterns"
3. Find 2-3 well-designed public APIs in the same domain (e.g., Stripe for payments, GitHub for developer tools, Twilio for messaging) and study their:
   - URL structure and naming conventions
   - Error response format
   - Pagination approach
   - Rate limiting headers
4. Search for "[API style] versioning strategies pros cons"
5. If gRPC: search for "protobuf style guide" and "gRPC API design guide"

Record findings for reference in subsequent steps.

---

## Step 2: Define API Foundation

### 2a: Versioning Strategy

Choose one and justify:

| Strategy | When to Use |
|----------|-------------|
| URL path (`/api/v1/`) | REST APIs, clear separation, easy routing |
| Header (`Accept: application/vnd.api+v1`) | REST APIs, cleaner URLs, more complex client logic |
| Query param (`?version=1`) | Simple APIs, easy to test |
| Package versioning | gRPC/protobuf, native support |

### 2b: Base URL Structure

```
[protocol]://[host]/api/[version]/[resource]
```

Define:
- Base URL per environment (dev, staging, prod)
- Version prefix format
- Resource naming convention (plural nouns for REST, service-oriented for gRPC)

### 2c: Authentication Model

Reference TECH_STACK.md auth model. Define:

- **Auth mechanism:** JWT / OAuth2 / API key / session — with flow details
- **Token location:** `Authorization: Bearer <token>` header / cookie / query param
- **Token lifecycle:** issuance, refresh, expiration, revocation
- **Scopes/permissions:** map to RBAC/ABAC roles if applicable
- **Public vs. authenticated endpoints:** list which endpoints require no auth

### 2d: Common Response Envelope

Define the standard response wrapper used by all endpoints:

**REST example:**
```json
{
  "data": {},
  "meta": {
    "request_id": "string",
    "pagination": {}
  },
  "errors": [
    {
      "code": "RESOURCE_NOT_FOUND",
      "message": "Human-readable message",
      "field": "optional_field_name",
      "details": {}
    }
  ]
}
```

**gRPC example:**
Define standard metadata fields, status codes, and error detail messages.

---

## Step 3: Endpoint Inventory

For each user story in the PRD, identify the API operations required.

### 3a: Resource Identification

List every resource (noun) the API exposes:

| Resource | Description | CRUD Operations | Custom Operations |
|----------|-------------|-----------------|-------------------|
| | | | |

### 3b: Endpoint Definitions

For **each** endpoint, define:

```markdown
### [METHOD] /[resource-path]

**Purpose:** [Which user story/requirement this serves]
**Auth required:** [Yes/No — scopes if yes]
**Idempotent:** [Yes/No]

**Path parameters:**
| Param | Type | Required | Description |
|-------|------|----------|-------------|

**Query parameters:**
| Param | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|

**Request body:**
```json
{
  "field_name": "type — constraints — description"
}
```

**Response (success):**
| Status | When |
|--------|------|
```json
{
  "data": { ... }
}
```

**Response (error):**
| Status | Code | When |
|--------|------|------|
```

### 3c: Request/Response Schema Definitions

Define reusable schemas for all request and response objects:

```yaml
# OpenAPI 3.1 format (if REST)
components:
  schemas:
    ResourceName:
      type: object
      required: [field1, field2]
      properties:
        id:
          type: string
          format: uuid
          readOnly: true
        field1:
          type: string
          minLength: 1
          maxLength: 255
        created_at:
          type: string
          format: date-time
          readOnly: true
```

For gRPC, define as protobuf messages:

```protobuf
message ResourceName {
  string id = 1;
  string field1 = 2;
  google.protobuf.Timestamp created_at = 3;
}
```

---

## Step 4: Pagination Design

Choose a pagination strategy and define it consistently:

| Strategy | When to Use | Tradeoffs |
|----------|-------------|-----------|
| Cursor-based | Large datasets, real-time data, infinite scroll | No random page access, more complex |
| Offset-based | Small datasets, page-number UI | Performance degrades on large offsets |
| Keyset | Time-ordered data, high-performance | Requires sortable unique key |

Define:
- **Default page size:** (e.g., 20)
- **Maximum page size:** (e.g., 100)
- **Pagination request params:** `cursor` / `page` + `per_page` / `after` + `first`
- **Pagination response shape:**

```json
{
  "meta": {
    "pagination": {
      "total": 150,
      "has_next_page": true,
      "next_cursor": "eyJpZCI6MTIzfQ==",
      "has_previous_page": false,
      "previous_cursor": null
    }
  }
}
```

---

## Step 5: Rate Limiting

Define rate limits per endpoint class:

| Endpoint Pattern | Limit | Window | Scope | Response When Exceeded |
|-----------------|-------|--------|-------|----------------------|
| Auth endpoints (`/auth/*`) | 10 req | 1 min | Per IP | 429 + `Retry-After` header |
| Read endpoints (GET) | 100 req | 1 min | Per user | 429 + `Retry-After` header |
| Write endpoints (POST/PUT/DELETE) | 30 req | 1 min | Per user | 429 + `Retry-After` header |
| Bulk operations | 5 req | 1 min | Per user | 429 + `Retry-After` header |
| Webhooks | 1000 req | 1 hour | Per integration | 429 |

Define rate limit response headers:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1620000000
Retry-After: 30
```

---

## Step 6: Error Format

Define the error taxonomy for the API layer. Cross-reference with `error_taxonomy.md` if already produced.

### Standard Error Codes

| Code | HTTP Status | Category | Description |
|------|------------|----------|-------------|
| `VALIDATION_ERROR` | 400 | Client | Request body/params failed validation |
| `UNAUTHORIZED` | 401 | Client | Missing or invalid auth credentials |
| `FORBIDDEN` | 403 | Client | Valid auth but insufficient permissions |
| `RESOURCE_NOT_FOUND` | 404 | Client | Requested resource does not exist |
| `CONFLICT` | 409 | Client | Resource state conflict (duplicate, stale) |
| `RATE_LIMITED` | 429 | Client | Rate limit exceeded |
| `INTERNAL_ERROR` | 500 | Server | Unexpected server error |
| `SERVICE_UNAVAILABLE` | 503 | Server | Dependency down or maintenance mode |

### Error Response Body

```json
{
  "errors": [
    {
      "code": "VALIDATION_ERROR",
      "message": "Email address is not valid",
      "field": "email",
      "details": {
        "pattern": "^[\\w.+-]+@[\\w-]+\\.[\\w.-]+$"
      }
    }
  ]
}
```

Rules:
- `message` must be safe to display to end users (no stack traces, no internal details)
- `code` is machine-readable and stable across versions
- `field` is present only for validation errors
- `details` is optional and provides machine-actionable context

---

## Step 7: Compile OpenAPI / Protobuf Spec

Produce the full spec in the format specified by TECH_STACK.md:

- **REST:** OpenAPI 3.1 YAML document with all endpoints, schemas, security schemes, and examples
- **gRPC:** `.proto` file(s) with service definitions, message types, and enums
- **GraphQL:** SDL with types, queries, mutations, subscriptions, and input types
- **tRPC:** Router definitions with input/output Zod schemas

Include request/response examples for every endpoint. Examples should use realistic data, not lorem ipsum.

---

## Step 8: Cross-Reference Validation

Before finalizing, verify:

- [ ] Every user story in the PRD maps to at least one endpoint
- [ ] Every endpoint maps to at least one user story (no orphan endpoints)
- [ ] All required fields are marked as required in schemas
- [ ] All response schemas include `id` and timestamps where appropriate
- [ ] Auth requirements are specified for every endpoint
- [ ] Error codes cover all foreseeable failure modes per endpoint
- [ ] Pagination is applied to all list endpoints
- [ ] Rate limits are defined for all endpoint classes
- [ ] Naming is consistent (plural nouns for resources, verb phrases for actions)
- [ ] Field naming convention is consistent (camelCase / snake_case — pick one)

---

## Output

The final output is written to the API contract section of `ARCHITECTURE.md` or as a standalone `API_CONTRACT.md` in the handoff directory, depending on the invoking workflow's instructions.
