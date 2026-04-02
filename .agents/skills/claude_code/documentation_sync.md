---
description: Ensure code documentation matches the actual implementation. Verifies doc comments (JSDoc/TSDoc/Go doc/docstrings) match exported function signatures, updates README if project structure changed, verifies API documentation matches actual endpoints, and checks for stale TODO/FIXME comments.
invoked_from:
  - workflows/claude_code/04_quality_gates.md
  - workflows/claude_code/05_code_review.md
  - After significant implementation changes
  - Before any production release
produces:
  - Documentation sync report with pass/fail per category
  - List of mismatched doc comments with corrected versions
  - Updated README sections (if project structure changed)
  - API documentation drift report
  - Stale TODO/FIXME inventory with age and ownership
---

# Skill: Documentation Sync

Verify that all code documentation accurately reflects the current implementation. Stale documentation is worse than no documentation — it actively misleads. Every exported function, API endpoint, and structural change must have matching documentation.

---

## Prerequisites

Before running this skill, ensure the following exist:

- `TECH_STACK.md` — for language and documentation tooling
- `ARCHITECTURE.md` — for structural reference
- Access to the full codebase
- Knowledge of what changed since the last documentation sync (use `git diff` or the current task scope)

---

## Step 1: Verify Doc Comments Match Function Signatures

Check that every exported function's doc comment matches its actual signature.

### Actions

1. **Identify all exported functions/methods/classes:**

   **TypeScript / JavaScript:**
   ```bash
   # Find all exported functions
   grep -rn 'export function\|export const\|export class\|export interface\|export type\|export enum\|export default' \
     --include='*.ts' --include='*.tsx' --include='*.js' --include='*.jsx' src/
   ```

   **Python:**
   ```bash
   # Find all public functions and classes (not starting with _)
   grep -rn 'def [a-zA-Z]\|class [A-Z]' --include='*.py' src/ | grep -v 'def _'
   ```

   **Go:**
   ```bash
   # Find all exported functions (start with uppercase)
   grep -rn 'func [A-Z]\|type [A-Z]' --include='*.go' . | grep -v '_test.go\|vendor/'
   ```

2. **For each exported function, verify its doc comment:**

   **TypeScript (JSDoc/TSDoc):**
   - Every exported function must have a JSDoc/TSDoc comment.
   - `@param` tags must list every parameter with the correct name and type.
   - `@returns` tag must describe the return value and match the return type.
   - `@throws` tag must document any thrown exceptions.
   - `@example` tag is recommended for non-trivial functions.
   - `@deprecated` tag must be present if the function is deprecated.

   Check for mismatches:
   ```bash
   # Find exported functions without preceding doc comments
   # This is a heuristic — look for export lines not preceded by */ on the previous line
   grep -rn -B1 'export function\|export const.*=.*(' --include='*.ts' src/ | \
     grep -v '\*/' | grep 'export'
   ```

   **Python (docstrings):**
   - Every public function and class must have a docstring.
   - Parameters must be documented (Google style, NumPy style, or Sphinx style — match project convention).
   - Return values must be documented.
   - Raised exceptions must be documented.

   Check for missing docstrings:
   ```bash
   # Find public functions without docstrings
   # Heuristic: function definition not followed by a triple-quote on the next line
   grep -rn -A1 'def [a-zA-Z]' --include='*.py' src/ | grep -v 'def _' | \
     grep -B1 -v '"""' | grep 'def '
   ```

   **Go (doc comments):**
   - Every exported function, type, and package must have a doc comment.
   - Doc comment must start with the function/type name.
   - Parameters are described in the comment prose (Go does not use @param tags).

   Check for missing doc comments:
   ```bash
   # Find exported items without doc comments
   grep -rn -B1 'func [A-Z]\|type [A-Z]' --include='*.go' . | grep -v '_test.go' | \
     grep -B1 'func\|type' | grep -v '//'
   ```

3. **For each mismatch found, document:**

   | File | Function | Issue | Current Doc | Correct Doc |
   |------|----------|-------|-------------|-------------|
   | [path] | [name] | Missing param / Wrong type / Missing doc | [current] | [corrected] |

4. **Auto-fix where possible:**
   - If a parameter was added to a function but not to its doc comment, add the `@param` tag.
   - If a parameter was renamed, update the doc comment.
   - If a parameter was removed, remove the `@param` tag.
   - If the return type changed, update the `@returns` tag.

---

## Step 2: README Verification

Check that the README reflects the current project structure and setup.

### Actions

1. **Read the current README:**
   ```bash
   cat README.md
   ```

2. **Verify project structure section (if present):**
   - Compare the directory tree in the README against the actual directory structure.
   ```bash
   # Generate current directory tree
   find src/ -type f | head -50 | sort
   # Or
   tree src/ -L 3 --dirsfirst 2>/dev/null || find src/ -type d | sort
   ```
   - Flag any directories or files mentioned in the README that no longer exist.
   - Flag any new directories or files not mentioned in the README.

3. **Verify setup instructions:**
   - Are the listed dependencies still correct?
   - Do the install commands still work?
   - Are the environment variable names still correct?
   - Do the example commands still work?

4. **Verify build and run commands:**
   ```bash
   # Check if the commands listed in README exist in package.json
   cat package.json | grep -A 30 '"scripts"' 2>/dev/null
   # Or check Makefile targets
   cat Makefile 2>/dev/null | grep '^[a-zA-Z].*:'
   ```

5. **Check for outdated version references:**
   - Node.js version in README vs `.nvmrc` or `engines` in `package.json`.
   - Python version in README vs `pyproject.toml` or `.python-version`.
   - Any tool versions mentioned that may have changed.

6. **Report:**

   | README Section | Status | Issue |
   |---------------|--------|-------|
   | Project description | Current/Stale | [details] |
   | Setup instructions | Current/Stale | [details] |
   | Directory structure | Current/Stale | [details] |
   | Build commands | Current/Stale | [details] |
   | Environment variables | Current/Stale | [details] |
   | Version references | Current/Stale | [details] |

---

## Step 3: API Documentation Verification

Verify that API documentation matches the actual implemented endpoints.

### Actions

1. **Identify the API documentation source:**
   ```bash
   # OpenAPI / Swagger
   find . -name 'openapi.*' -o -name 'swagger.*' -o -name '*.openapi.*' | grep -v node_modules

   # Postman collections
   find . -name '*.postman_collection.json' | grep -v node_modules

   # API route definitions in code
   grep -rn 'router\.\(get\|post\|put\|patch\|delete\)\|@app\.\(get\|post\|put\|patch\|delete\)\|@Get\|@Post\|@Put\|@Patch\|@Delete' \
     --include='*.ts' --include='*.js' --include='*.py' src/
   ```

2. **Build a list of actual endpoints from the code:**

   | Method | Path | Handler | Auth Required | Request Body | Response |
   |--------|------|---------|--------------|-------------|----------|
   | [GET/POST/etc] | [path] | [function] | [yes/no] | [schema] | [schema] |

3. **Compare against API documentation:**

   | Endpoint | In Code | In Docs | Match | Issue |
   |----------|---------|---------|-------|-------|
   | GET /api/users | Yes | Yes | Yes/No | [mismatch details] |
   | POST /api/users | Yes | No | N/A | Missing from docs |
   | DELETE /api/legacy | No | Yes | N/A | Documented but removed from code |

4. **For each endpoint present in both code and docs, verify:**
   - Request parameters match (path params, query params, body schema).
   - Response schema matches actual response shape.
   - Error responses are documented.
   - Authentication requirements are documented correctly.
   - Rate limiting is documented if applicable.

5. **Check for auto-generation opportunities:**
   - If using TypeScript with decorators (NestJS, tsoa), can the OpenAPI spec be auto-generated?
   - If using Python with FastAPI, is the auto-generated spec being served?
   - If using Go with swag or similar, is the spec up to date?

---

## Step 4: Stale TODO/FIXME Inventory

Find and catalog all TODO, FIXME, HACK, and XXX comments.

### Actions

1. **Search for all TODO-style comments:**
   ```bash
   grep -rn 'TODO\|FIXME\|HACK\|XXX\|WORKAROUND\|TEMP\|DEPRECATED' \
     --include='*.ts' --include='*.tsx' --include='*.js' --include='*.jsx' \
     --include='*.py' --include='*.go' --include='*.rs' \
     --include='*.css' --include='*.scss' \
     src/
   ```

2. **For each comment, determine:**
   - **Age:** When was this line last modified?
     ```bash
     # Get the last commit date for a specific line
     git log -1 --format="%ai" -L <line>,<line>:<file>
     # Or
     git blame -L <line>,<line> <file>
     ```
   - **Author:** Who wrote this comment?
   - **Context:** Is this TODO still relevant, or has it been addressed?
   - **Staleness:** Is the TODO older than 30 days? 90 days? 180 days?

3. **Categorize and report:**

   | # | Type | File:Line | Comment | Age | Author | Status |
   |---|------|-----------|---------|-----|--------|--------|
   | 1 | TODO | [path:line] | [comment text] | [days] | [author] | Active/Stale/Resolved |
   | 2 | FIXME | [path:line] | [comment text] | [days] | [author] | Active/Stale/Resolved |
   | 3 | HACK | [path:line] | [comment text] | [days] | [author] | Active/Stale/Resolved |

4. **Classification rules:**
   - **Active:** Less than 30 days old and describes work that is clearly still needed.
   - **Stale:** More than 90 days old without progress. Should be converted to an issue or removed.
   - **Resolved:** The TODO describes work that has already been completed. The comment should be removed.

5. **For stale TODOs, recommend action:**
   - Convert to a tracked issue (GitHub issue, Jira ticket, etc.).
   - Remove if the work is no longer relevant.
   - Fix if the work is small enough to do now.

6. **Check for TODOs that reference removed code:**
   ```bash
   # TODOs that reference function names or variables that no longer exist
   # This requires manual review of each TODO in context
   ```

---

## Step 5: Inline Comment Quality Check

Verify that inline comments add value and are not misleading.

### Actions

1. **Check for comments that repeat the code:**
   - Comments like `// increment counter` above `counter++` add no value.
   - Comments should explain **why**, not **what**.

2. **Check for commented-out code:**
   ```bash
   # Find multi-line comment blocks that look like code
   grep -rn '//.*function\|//.*const\|//.*let\|//.*var\|//.*return\|#.*def \|#.*class ' \
     --include='*.ts' --include='*.js' --include='*.py' src/ | head -30
   ```
   - Commented-out code should be removed. Version control preserves history.

3. **Check for misleading comments:**
   - Read each comment adjacent to recently modified code.
   - If the code was changed but the comment was not updated, flag it.

---

## Output Format

```markdown
## Documentation Sync Report

**Date:** [timestamp]
**Scope:** [full sync / targeted — specify files]

### Summary

| Category | Issues Found | Auto-Fixed | Manual Fix Needed | Status |
|----------|-------------|------------|-------------------|--------|
| Doc Comments | [n] | [n] | [n] | PASS/FAIL |
| README | [n] | [n] | [n] | PASS/FAIL |
| API Documentation | [n] | [n] | [n] | PASS/FAIL |
| Stale TODOs | [n] | - | [n] | PASS/FAIL |
| Inline Comments | [n] | - | [n] | INFO |

**Overall: PASS / FAIL**

### Doc Comment Mismatches

| # | File | Function | Issue | Action |
|---|------|----------|-------|--------|
| 1 | [path] | [name] | [mismatch description] | [fix or flag] |

### README Issues

| # | Section | Issue | Action |
|---|---------|-------|--------|
| 1 | [section] | [description] | [update needed] |

### API Documentation Drift

| # | Endpoint | Issue | Action |
|---|----------|-------|--------|
| 1 | [method path] | [missing/wrong/extra] | [fix] |

### Stale TODO Inventory

| # | Type | File:Line | Age | Comment | Recommended Action |
|---|------|-----------|-----|---------|-------------------|
| 1 | [type] | [location] | [days] | [text] | Remove/Convert to issue/Fix now |

### Actions Taken

- [List of auto-fixes applied: doc comments updated, commented-out code removed, etc.]

### Actions Required (Manual)

- [ ] [Specific action with file and location]
```

Documentation is only useful when it is accurate. Every mismatch found must either be fixed or explicitly flagged for manual review.
