---
description: Structured self-review checklist that Claude Code runs on its own output before presenting it. Catches hallucinated imports, architecture drift, unhandled errors, type safety violations, dead code, naming inconsistencies, hardcoded secrets, SOLID violations, and circular dependencies.
invoked_from:
  - After generating or modifying any code file
  - Before committing changes
  - When requested as a standalone review pass
produces:
  - Self-review report with pass/fail per checklist item
  - List of issues found with file paths and line numbers
  - Suggested fixes for each issue
---

# Skill: Code Review Self-Check

Run this checklist on every piece of code you produce or modify. This is not optional. Every item must be explicitly verified before presenting code to the user or committing changes.

---

## Prerequisites

Before running this checklist, ensure you have access to:

- `ARCHITECTURE.md` — for interface contracts, module boundaries, and dependency rules
- `TECH_STACK.md` — for language, framework, and tooling choices
- The code you just wrote or modified

---

## Check 1: Architecture Match

Verify that the code conforms to the architecture specification.

### Actions

1. Read `ARCHITECTURE.md` (or equivalent architecture document in the repository).
2. For every exported function, class, or module you created or modified:
   - Verify the interface signature matches what `ARCHITECTURE.md` defines.
   - Verify the module lives in the correct directory per the architecture layer diagram.
   - Verify data flows in the correct direction (e.g., controllers call services, services call repositories — never the reverse).
3. For any new module or file:
   - Verify it belongs to an existing architectural layer or component.
   - If it introduces a new component, flag it for architecture review.

### Failure Criteria

- An exported function signature differs from the architecture spec.
- A module is placed in the wrong directory or layer.
- A dependency arrow points in the wrong direction.

---

## Check 2: Import Verification

Verify that every import resolves to a real package or module.

### Actions

1. For every `import` / `require` / `from` statement in the code:
   - Check if the package exists in `package.json`, `go.mod`, `requirements.txt`, `Cargo.toml`, or the equivalent dependency manifest.
   - For local imports, verify the target file actually exists on disk using `ls` or glob.
   - For standard library imports, verify the module name is a real standard library module for the language version in `TECH_STACK.md`.
2. Run the language-appropriate import check:
   - **Node.js/TypeScript:** `npx tsc --noEmit` or attempt a dry build.
   - **Python:** `python -c "import <module>"` for each non-standard import.
   - **Go:** `go build ./...` to verify all imports resolve.
   - **Rust:** `cargo check` to verify all imports resolve.

### Failure Criteria

- Any import references a package not in the dependency manifest.
- Any import references a local file that does not exist.
- Any import uses a subpath that does not exist in the target package.
- Any standard library import uses a module name that does not exist in the language version.

---

## Check 3: Error Path Handling

Verify that every error path is explicitly handled.

### Actions

1. For every function call that can fail (returns an error, throws an exception, returns null/undefined, returns a Result/Option type):
   - Verify there is an explicit error handler (try/catch, if err != nil, .catch(), match on Err/None).
   - Verify the error handler does something meaningful (log, return error to caller, show user message) — not just swallow the error silently.
2. For every async operation:
   - Verify promise rejections are handled.
   - Verify there is no unhandled promise (no floating promises without await or .catch()).
3. For every HTTP endpoint or API handler:
   - Verify all error responses use appropriate status codes.
   - Verify error responses include a structured error body (not raw stack traces in production).
4. Grep for patterns that indicate missing error handling:
   - `catch {}` or `catch (e) {}` with empty bodies.
   - `// TODO: handle error` comments.
   - `_ = someFunction()` ignoring return values in Go.
   - Bare `except:` or `except Exception:` in Python without re-raise.

### Failure Criteria

- Any error path leads to a silent failure.
- Any async call lacks rejection handling.
- Any API endpoint can leak stack traces.
- Any catch block has an empty body.

---

## Check 4: Type Explicitness

Verify that all types are explicit — no `any`, no untyped parameters, no implicit returns.

### Actions

1. **TypeScript:** Search for `any` type annotations. Each instance must be justified or replaced.
   ```bash
   grep -rn ': any' --include='*.ts' --include='*.tsx' src/
   grep -rn 'as any' --include='*.ts' --include='*.tsx' src/
   ```
2. **Python:** Verify type hints on all function signatures (parameters and return types).
   ```bash
   # Check for functions missing type hints
   grep -rn 'def ' --include='*.py' src/ | grep -v '->'
   ```
3. **Go:** This is enforced by the compiler, but verify no `interface{}` or `any` is used without justification.
4. For all languages:
   - Verify function return types are explicit.
   - Verify callback and closure parameter types are explicit.
   - Verify generic type parameters are constrained (no unbounded generics).

### Failure Criteria

- Any `any` type without a documented justification comment.
- Any function missing explicit parameter or return types.
- Any unbounded generic without a constraint.

---

## Check 5: Function Signature Spec Match

Verify that function signatures match the specification or interface contract.

### Actions

1. For every function that implements an interface or contract defined in the spec:
   - Compare parameter names, types, and order.
   - Compare return type.
   - Compare error/exception types.
2. For every API endpoint:
   - Compare request schema (path params, query params, body) against API spec.
   - Compare response schema against API spec.
   - Compare error responses against error taxonomy.
3. For every event handler or callback:
   - Verify the signature matches what the caller expects.

### Failure Criteria

- Parameter type mismatch between implementation and spec.
- Missing or extra parameters.
- Return type mismatch.
- API response shape differs from spec.

---

## Check 6: Dead Code Detection

Verify that no dead code is present.

### Actions

1. Search for:
   - Functions that are defined but never called (not exported, not referenced).
   - Variables that are assigned but never read.
   - Imports that are not used.
   - Commented-out code blocks (more than 2 lines).
   - Unreachable code after `return`, `throw`, `break`, or `continue`.
2. Run the language-appropriate dead code check:
   - **TypeScript:** `npx tsc --noUnusedLocals --noUnusedParameters --noEmit`
   - **Python:** `vulture` or manual review.
   - **Go:** `go vet ./...` catches some; also check for `_` assignments.
   - **Rust:** Compiler warns on unused code by default.
3. Remove dead code. Do not leave it in "for later." If the code might be needed in the future, it lives in version control history, not in the current codebase.

### Failure Criteria

- Any unused import.
- Any unused variable or function.
- Any commented-out code block longer than 2 lines.
- Any unreachable code.

---

## Check 7: Naming Convention Consistency

Verify that naming conventions are consistent with the codebase.

### Actions

1. Check the existing codebase for naming patterns:
   - **Files:** kebab-case, camelCase, PascalCase, or snake_case?
   - **Functions/methods:** camelCase, PascalCase, or snake_case?
   - **Variables:** camelCase or snake_case?
   - **Constants:** UPPER_SNAKE_CASE?
   - **Types/interfaces:** PascalCase? Prefixed with I for interfaces?
   - **CSS classes:** BEM, kebab-case, or module-scoped?
2. Verify every name you introduced follows the established pattern.
3. Verify names are descriptive:
   - No single-letter variables outside of loop counters (`i`, `j`, `k`) or well-known conventions (`e` for event, `x`/`y` for coordinates).
   - No abbreviations unless they are universally understood (`id`, `url`, `http`).
   - Boolean variables/functions start with `is`, `has`, `should`, `can`, or similar.

### Failure Criteria

- Any name that breaks the established naming convention.
- Any non-descriptive variable name outside of loop counters.
- Any inconsistency between similar names (e.g., `getUserData` in one place and `fetch_user_data` in another within the same codebase).

---

## Check 8: No Hardcoded Secrets, URLs, or Magic Numbers

Verify that no sensitive or environment-specific values are hardcoded.

### Actions

1. Search for hardcoded secrets:
   ```bash
   grep -rn 'password\|secret\|api_key\|apikey\|token\|credential' --include='*.ts' --include='*.py' --include='*.go' --include='*.rs' src/
   ```
2. Search for hardcoded URLs:
   ```bash
   grep -rn 'http://\|https://' --include='*.ts' --include='*.py' --include='*.go' src/ | grep -v 'node_modules\|test\|spec\|\.md'
   ```
   Exceptions: URLs in test fixtures, documentation, and URL-parsing logic.
3. Search for magic numbers:
   - Any numeric literal that is not 0, 1, -1, or a commonly understood constant (like HTTP status codes in a handler).
   - These should be extracted to named constants with documentation.
4. Verify environment-specific values come from:
   - Environment variables.
   - Configuration files not committed to git.
   - Secret management systems.

### Failure Criteria

- Any hardcoded credential, API key, or token.
- Any hardcoded URL that should be configurable (database connection strings, API base URLs, webhook URLs).
- Any unexplained magic number.

---

## Check 9: SOLID Principles

Verify adherence to SOLID principles.

### Actions

1. **Single Responsibility:** Does each module/class/function do one thing? If a function is longer than ~40 lines, it likely does too much. If a file has more than ~300 lines, it likely contains multiple responsibilities.
2. **Open/Closed:** Are modules open for extension but closed for modification? Check for:
   - Switch statements on type discriminators that would need modification to add new types (use polymorphism or strategy pattern instead).
   - Functions with many boolean parameters that toggle behavior.
3. **Liskov Substitution:** Can derived types be used wherever base types are expected? Check for:
   - Subtypes that throw exceptions for methods the base type supports.
   - Subtypes that ignore or override base behavior in incompatible ways.
4. **Interface Segregation:** Are interfaces small and focused? Check for:
   - Interfaces with more than 5-7 methods (consider splitting).
   - Implementations that stub out interface methods with no-ops.
5. **Dependency Inversion:** Do high-level modules depend on abstractions, not concretions? Check for:
   - Direct instantiation of dependencies inside business logic (use injection).
   - Import of concrete implementations where an interface would suffice.

### Failure Criteria

- A function exceeding 40 lines without justification.
- A file exceeding 300 lines without justification.
- A switch/case on type discriminators that should use polymorphism.
- An interface with stubbed-out methods in implementations.
- A high-level module directly instantiating its dependencies.

---

## Check 10: No Circular Dependencies

Verify that no circular dependency chains exist.

### Actions

1. Trace the import graph for every file you created or modified:
   - List all imports in the file.
   - For each imported module, list its imports.
   - Verify no cycle exists (A imports B, B imports C, C imports A).
2. Run the language-appropriate cycle detection:
   - **TypeScript:** `npx madge --circular src/`
   - **Python:** `pydeps` or manual review of import chains.
   - **Go:** `go vet ./...` detects import cycles at compile time.
3. If a circular dependency is detected:
   - Extract the shared dependency into a separate module.
   - Use dependency inversion (depend on interfaces defined in a shared layer).
   - Use events or callbacks to break the cycle.

### Failure Criteria

- Any circular import chain.
- Any module that both imports from and is imported by the same module.

---

## Output Format

After running all checks, produce a report:

```markdown
## Self-Review Report

**Files reviewed:** [list of files]
**Date:** [timestamp]

| # | Check | Status | Issues |
|---|-------|--------|--------|
| 1 | Architecture Match | PASS/FAIL | [details] |
| 2 | Import Verification | PASS/FAIL | [details] |
| 3 | Error Path Handling | PASS/FAIL | [details] |
| 4 | Type Explicitness | PASS/FAIL | [details] |
| 5 | Function Signature Match | PASS/FAIL | [details] |
| 6 | Dead Code Detection | PASS/FAIL | [details] |
| 7 | Naming Conventions | PASS/FAIL | [details] |
| 8 | No Hardcoded Secrets/URLs/Magic Numbers | PASS/FAIL | [details] |
| 9 | SOLID Principles | PASS/FAIL | [details] |
| 10 | Circular Dependencies | PASS/FAIL | [details] |

### Issues Requiring Attention

- [ ] [File:line] [Description of issue] [Suggested fix]
- [ ] ...
```

If any check fails, fix the issue before proceeding. If a fix is not possible (e.g., an architectural question that needs user input), flag it explicitly and do not silently skip it.
