---
description: Run automated accessibility checks and produce a WCAG compliance report. Covers axe-core scans, keyboard navigation testing, ARIA attribute verification, color contrast ratio checks, focus management validation, and screen reader compatibility assessment.
invoked_from:
  - workflows/claude_code/04_quality_gates.md
  - workflows/claude_code/06_deploy_verify.md
  - After UI component creation or modification
  - Before any production release
produces:
  - WCAG 2.1 AA compliance report with pass/fail per criterion
  - List of accessibility violations with severity, element selectors, and fix instructions
  - Keyboard navigation test results per interactive component
  - Color contrast audit with specific failing color pairs
  - Focus management verification results
---

# Skill: Accessibility Audit

Run automated and semi-automated accessibility checks against the project's accessibility specification. Accessibility failures are bugs, not nice-to-haves. Any WCAG 2.1 Level A or AA violation blocks release unless explicitly deferred by the user with a documented remediation plan.

---

## Prerequisites

Before running this audit, ensure the following exist:

- `accessibility_spec` output (or equivalent accessibility requirements document) — for project-specific WCAG targets and ARIA inventories
- `TECH_STACK.md` — for framework and testing tooling
- A running instance of the application (local or staging)
- Browser or headless browser access for automated scanning

If no accessibility spec exists, audit against WCAG 2.1 Level AA as the default standard.

---

## Step 1: Automated Accessibility Scanning (axe-core)

Run an automated scanner to catch machine-detectable violations.

### Actions

1. **Install axe-core if not already present:**
   ```bash
   # For CLI scanning
   npm install --save-dev @axe-core/cli

   # For integration testing
   npm install --save-dev axe-core
   # Or for Playwright/Cypress
   npm install --save-dev @axe-core/playwright  # Playwright
   npm install --save-dev cypress-axe           # Cypress
   ```

2. **Run axe-core against every page/route:**
   ```bash
   # CLI scan of a single page
   npx axe http://localhost:3000 --exit

   # Scan multiple pages
   npx axe http://localhost:3000 http://localhost:3000/dashboard http://localhost:3000/settings --exit
   ```

3. **For integration test-based scanning, add to existing test suite:**
   ```javascript
   // Playwright example
   const { test, expect } = require('@playwright/test');
   const AxeBuilder = require('@axe-core/playwright').default;

   test('should have no accessibility violations', async ({ page }) => {
     await page.goto('/');
     const results = await new AxeBuilder({ page }).analyze();
     expect(results.violations).toEqual([]);
   });
   ```

4. **Parse and categorize results:**

   | Severity | Count | WCAG Criteria | Description |
   |----------|-------|---------------|-------------|
   | Critical | [n] | [criteria] | [summary] |
   | Serious | [n] | [criteria] | [summary] |
   | Moderate | [n] | [criteria] | [summary] |
   | Minor | [n] | [criteria] | [summary] |

5. **For each violation, record:**
   - The WCAG success criterion violated.
   - The HTML element(s) affected (CSS selector).
   - The specific failure description.
   - The recommended fix from axe-core.

### Alternative Scanners

If axe-core is not suitable for the project's tech stack:
- **Pa11y:** `npx pa11y http://localhost:3000 --standard WCAG2AA`
- **Lighthouse accessibility:** `npx lighthouse http://localhost:3000 --only-categories=accessibility --output=json`
- **WAVE API:** If the project has a WAVE API key

---

## Step 2: Keyboard Navigation Testing

Verify that every interactive element is reachable and operable via keyboard alone.

### Actions

1. **Inventory all interactive elements on each page:**
   - Buttons, links, form inputs, selects, checkboxes, radio buttons.
   - Custom components: dropdowns, modals, tabs, accordions, sliders, date pickers.
   - Drag-and-drop targets (must have keyboard alternative).

2. **For each page, test the following keyboard flows:**

   | Test | Keys | Expected Behavior | Status |
   |------|------|-------------------|--------|
   | Tab through all elements | `Tab` | Focus moves to each interactive element in logical order | PASS/FAIL |
   | Reverse tab | `Shift+Tab` | Focus moves backward through elements | PASS/FAIL |
   | Activate buttons | `Enter`, `Space` | Button action triggers | PASS/FAIL |
   | Follow links | `Enter` | Navigation occurs | PASS/FAIL |
   | Open dropdown | `Enter`/`Space`/`ArrowDown` | Dropdown opens, first item focused | PASS/FAIL |
   | Navigate dropdown | `ArrowUp`/`ArrowDown` | Focus moves between items | PASS/FAIL |
   | Close dropdown | `Escape` | Dropdown closes, focus returns to trigger | PASS/FAIL |
   | Tab into modal | Trigger modal | Focus moves to first element inside modal | PASS/FAIL |
   | Tab within modal | `Tab` at last element | Focus wraps to first element (focus trap) | PASS/FAIL |
   | Close modal | `Escape` | Modal closes, focus returns to trigger | PASS/FAIL |
   | Navigate tabs | `ArrowLeft`/`ArrowRight` | Tab selection changes | PASS/FAIL |
   | Skip navigation | `Tab` (first press) | "Skip to content" link appears and works | PASS/FAIL |

3. **Check for keyboard traps:**
   - Tab into every component and verify you can tab out without using a mouse.
   - The only acceptable focus trap is inside open modals/dialogs, and it must release on `Escape`.

4. **Automated keyboard test (if using Playwright/Cypress):**
   ```javascript
   // Playwright keyboard navigation test
   test('tab order is correct on dashboard', async ({ page }) => {
     await page.goto('/dashboard');
     await page.keyboard.press('Tab'); // Skip link
     await page.keyboard.press('Enter'); // Activate skip link
     const focused = await page.evaluate(() => document.activeElement?.tagName);
     expect(focused).toBe('MAIN'); // or first element in main
   });
   ```

---

## Step 3: ARIA Attribute Verification

Verify that all ARIA attributes are correctly applied per the accessibility spec.

### Actions

1. **Check for required ARIA landmarks:**
   ```bash
   # Using a headless browser or DOM inspection
   # Verify these landmarks exist on every page:
   # - banner (header)
   # - navigation (nav) with aria-label
   # - main
   # - contentinfo (footer)
   ```

2. **For each custom interactive component, verify:**

   | Component | Required ARIA | Present | Correct Value | Status |
   |-----------|--------------|---------|---------------|--------|
   | Modal/Dialog | `role="dialog"`, `aria-modal="true"`, `aria-labelledby` | Yes/No | [value] | PASS/FAIL |
   | Tabs | `role="tablist"`, `role="tab"`, `role="tabpanel"`, `aria-selected` | Yes/No | [value] | PASS/FAIL |
   | Dropdown | `role="menu"` or `role="listbox"`, `aria-expanded` | Yes/No | [value] | PASS/FAIL |
   | Alert/Toast | `role="alert"` or `role="status"`, `aria-live` | Yes/No | [value] | PASS/FAIL |
   | Accordion | `aria-expanded`, `aria-controls` | Yes/No | [value] | PASS/FAIL |
   | Progress | `role="progressbar"`, `aria-valuenow`, `aria-valuemin`, `aria-valuemax` | Yes/No | [value] | PASS/FAIL |

3. **Check for ARIA anti-patterns:**
   ```bash
   # Search for common ARIA mistakes in source code
   # role on elements that already have implicit roles
   grep -rn 'role="button"' --include='*.tsx' --include='*.jsx' src/ | grep '<button'
   # aria-label duplicating visible text
   # aria-hidden on focusable elements
   grep -rn 'aria-hidden="true"' --include='*.tsx' --include='*.jsx' src/
   ```

4. **Verify all images have appropriate alt text:**
   ```bash
   # Find images without alt attributes
   grep -rn '<img' --include='*.tsx' --include='*.jsx' --include='*.html' src/ | grep -v 'alt='
   ```

5. **Verify form inputs have labels:**
   ```bash
   # Find inputs without associated labels
   grep -rn '<input' --include='*.tsx' --include='*.jsx' --include='*.html' src/ | grep -v 'aria-label\|aria-labelledby\|id='
   ```

---

## Step 4: Color Contrast Ratio Checks

Verify that all text and UI elements meet WCAG contrast requirements.

### Actions

1. **Extract the color palette from the design system or CSS:**
   ```bash
   # Find color definitions
   grep -rn 'color:\|background-color:\|--color-\|--bg-' --include='*.css' --include='*.scss' --include='*.ts' src/ | head -50
   ```

2. **For each text color / background color pair, calculate the contrast ratio:**
   - Use an automated tool or manual calculation.
   - WCAG AA requires:
     - Normal text (< 18pt or < 14pt bold): 4.5:1 minimum.
     - Large text (>= 18pt or >= 14pt bold): 3:1 minimum.
     - Non-text UI components and graphical objects: 3:1 minimum.

3. **Automated contrast checking:**
   ```bash
   # axe-core catches most contrast issues
   # Lighthouse also reports contrast failures
   # For manual checking: use contrast-ratio npm package or web-based tools
   ```

4. **Report:**

   | Element | Foreground | Background | Ratio | Required | Status |
   |---------|-----------|-----------|-------|----------|--------|
   | Body text | [color] | [color] | [ratio] | 4.5:1 | PASS/FAIL |
   | Secondary text | [color] | [color] | [ratio] | 4.5:1 | PASS/FAIL |
   | Button label | [color] | [color] | [ratio] | 4.5:1 | PASS/FAIL |
   | Link text | [color] | [color] | [ratio] | 4.5:1 | PASS/FAIL |
   | Error text | [color] | [color] | [ratio] | 4.5:1 | PASS/FAIL |
   | Input border | [color] | [color] | [ratio] | 3:1 | PASS/FAIL |
   | Focus ring | [color] | [color] | [ratio] | 3:1 | PASS/FAIL |
   | Icon (informational) | [color] | [color] | [ratio] | 3:1 | PASS/FAIL |

5. **Check dark mode (if applicable):**
   - Re-run all contrast checks with the dark mode palette.
   - Verify that switching modes does not introduce new contrast failures.

---

## Step 5: Focus Management Verification

Verify that focus is managed correctly for all dynamic content changes.

### Actions

1. **Test each focus management scenario:**

   | Scenario | Action | Expected Focus Target | Actual Focus Target | Status |
   |----------|--------|----------------------|--------------------|----|
   | Modal opens | Click trigger button | First focusable element inside modal | [observed] | PASS/FAIL |
   | Modal closes | Press Escape or click close | Element that triggered the modal | [observed] | PASS/FAIL |
   | Drawer/sidebar opens | Click trigger | First focusable element in drawer | [observed] | PASS/FAIL |
   | Drawer/sidebar closes | Press Escape or click close | Trigger element | [observed] | PASS/FAIL |
   | SPA route change | Click navigation link | Page heading (h1) or main content | [observed] | PASS/FAIL |
   | Form error | Submit invalid form | First field with error | [observed] | PASS/FAIL |
   | Item deleted from list | Delete an item | Nearest remaining item | [observed] | PASS/FAIL |
   | Toast/notification | Triggered by action | No focus change (aria-live used) | [observed] | PASS/FAIL |
   | Accordion expand | Click accordion header | Expanded content or stays on header | [observed] | PASS/FAIL |

2. **Verify focus indicator visibility:**
   - Tab through the entire page and verify the focus indicator is visible on every focused element.
   - Check that focus indicators are visible on both light and dark backgrounds.
   - Verify focus indicator meets the 3:1 contrast requirement.

3. **Verify no focus loss:**
   - After any dynamic content change, `document.activeElement` should not be `<body>` unless intentional.
   ```javascript
   // Automated check in test
   await page.click('#delete-item-button');
   const activeElement = await page.evaluate(() => document.activeElement?.tagName);
   expect(activeElement).not.toBe('BODY');
   ```

---

## Step 6: Screen Reader Compatibility (If Available)

If a screen reader is available in the environment, test critical user flows.

### Actions

1. **Test with at least one screen reader:**
   - macOS: VoiceOver (built-in)
   - Linux: Orca (if available)
   - Headless: Use axe-core and ARIA checks as proxy

2. **For each critical flow, verify:**
   - Page title is announced on navigation.
   - Headings create a navigable document outline.
   - Form labels are announced when inputs receive focus.
   - Error messages are announced (via aria-live regions).
   - Dynamic content updates are announced appropriately.
   - Button purposes are clear from their announced names.

3. **Check live region behavior:**
   - Verify `role="alert"` regions announce immediately (assertive).
   - Verify `role="status"` regions announce at the next pause (polite).
   - Verify no excessive announcements (e.g., a live region that fires on every keystroke).

---

## Output Format

```markdown
## Accessibility Audit Report

**Date:** [timestamp]
**Standard:** WCAG 2.1 Level AA
**Pages tested:** [list of URLs/routes]
**Tools used:** [axe-core version, Lighthouse, manual testing]

### Summary

| Category | Violations | Warnings | Status |
|----------|-----------|----------|--------|
| Automated (axe-core) | [n] | [n] | PASS/FAIL |
| Keyboard Navigation | [n] issues | - | PASS/FAIL |
| ARIA Attributes | [n] issues | [n] | PASS/FAIL |
| Color Contrast | [n] failures | - | PASS/FAIL |
| Focus Management | [n] issues | - | PASS/FAIL |
| Screen Reader | [n] issues | - | PASS/FAIL/SKIPPED |

**Overall: PASS / FAIL**

### Critical Violations (Must Fix Before Release)

| # | WCAG Criterion | Element | Issue | Fix |
|---|---------------|---------|-------|-----|
| 1 | [criterion] | [selector] | [description] | [fix] |

### Serious Violations (Should Fix Before Release)

| # | WCAG Criterion | Element | Issue | Fix |
|---|---------------|---------|-------|-----|
| 1 | [criterion] | [selector] | [description] | [fix] |

### Minor Issues and Warnings

| # | Category | Element | Issue | Fix |
|---|----------|---------|-------|-----|
| 1 | [category] | [selector] | [description] | [fix] |

### Keyboard Navigation Results

[Include the keyboard test table from Step 2]

### Recommendations

1. [Action item with WCAG criterion reference]
2. [Action item with WCAG criterion reference]
```

Every violation must reference the specific WCAG success criterion it violates and include an actionable fix recommendation.
