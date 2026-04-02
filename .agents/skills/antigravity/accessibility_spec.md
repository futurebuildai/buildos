---
description: Define WCAG 2.1 AA/AAA compliance requirements per component. Produce ARIA role inventory, keyboard navigation flows, screen reader announcement strategy, color contrast requirements, focus management rules, and reduced motion support.
invoked_from:
  - workflows/antigravity/05_design_system.md
  - workflows/antigravity/06_product_spec.md
  - workflows/antigravity/08_testing_strategy.md
produces:
  - WCAG 2.1 AA/AAA compliance matrix per component
  - ARIA role and attribute inventory
  - Keyboard navigation flow map
  - Screen reader announcement strategy
  - Color contrast requirements and validation
  - Focus management rules
  - Reduced motion and prefers-contrast support
browser_usage: None (relies on WCAG standards and design system inputs)
---

# Skill: Accessibility Specification

Define accessibility requirements that make the product usable by everyone. Accessibility is not an afterthought bolted onto finished features — it is a structural requirement that shapes component design, interaction patterns, and testing strategy from the start.

Target: **WCAG 2.1 Level AA** compliance as the baseline. AAA targets are specified where achievable without significant tradeoffs.

---

## Prerequisites

Before invoking this skill, ensure the following exist:

- `DESIGN_SYSTEM.md` — for component inventory, color palette, typography
- `INFORMATION_ARCHITECTURE.md` — for navigation structure and screen inventory
- `PRD.md` or `SCOPE_DEFINITION.md` — for understanding user interactions

---

## Step 1: WCAG 2.1 Compliance Matrix

Map every WCAG 2.1 success criterion to its implementation status:

### Perceivable

| Criterion | Level | Requirement | Implementation | Status |
|-----------|-------|------------|----------------|--------|
| 1.1.1 Non-text Content | A | All images have alt text, decorative images have empty alt | `alt` attribute on all `<img>`, `role="presentation"` for decorative | Required |
| 1.2.1 Audio-only and Video-only | A | Provide text alternative for audio/video content | Transcripts for audio, text description for video | If applicable |
| 1.3.1 Info and Relationships | A | Structure conveyed visually is also in markup | Semantic HTML, ARIA landmarks, proper heading hierarchy | Required |
| 1.3.2 Meaningful Sequence | A | Reading order matches visual order | DOM order matches visual flow | Required |
| 1.3.3 Sensory Characteristics | A | Instructions do not rely solely on shape/color/size/location | Add text labels alongside visual indicators | Required |
| 1.3.4 Orientation | AA | Content not restricted to single orientation | No orientation locks | Required |
| 1.3.5 Identify Input Purpose | AA | Input purpose is programmatically determinable | `autocomplete` attributes on form fields | Required |
| 1.4.1 Use of Color | A | Color is not the only means of conveying information | Icons, text, patterns alongside color | Required |
| 1.4.2 Audio Control | A | Auto-playing audio can be paused/stopped | No auto-play audio, or provide controls | If applicable |
| 1.4.3 Contrast (Minimum) | AA | 4.5:1 for normal text, 3:1 for large text | Verified against color palette | Required |
| 1.4.4 Resize Text | AA | Text resizable to 200% without loss of content | Relative units (rem/em), no fixed containers | Required |
| 1.4.5 Images of Text | AA | Real text used instead of images of text | CSS for styling, not text-in-images | Required |
| 1.4.10 Reflow | AA | Content reflows at 320px width without horizontal scrolling | Responsive design, no horizontal scroll at 320px | Required |
| 1.4.11 Non-text Contrast | AA | 3:1 contrast for UI components and graphical objects | Borders, icons, focus indicators meet ratio | Required |
| 1.4.12 Text Spacing | AA | Content adapts to user text spacing preferences | No clipping when line-height, letter-spacing, word-spacing adjusted | Required |
| 1.4.13 Content on Hover or Focus | AA | Hover/focus content is dismissible, hoverable, persistent | Tooltips dismissible via Escape, remain while hovered | Required |
| 1.4.6 Contrast (Enhanced) | AAA | 7:1 for normal text, 4.5:1 for large text | Target where feasible | Stretch |

### Operable

| Criterion | Level | Requirement | Implementation | Status |
|-----------|-------|------------|----------------|--------|
| 2.1.1 Keyboard | A | All functionality available via keyboard | Tab, Enter, Space, Arrow keys, Escape | Required |
| 2.1.2 No Keyboard Trap | A | Focus can be moved away from all components | No focus traps (except modals with intentional trap + Escape exit) | Required |
| 2.1.4 Character Key Shortcuts | A | Single-character shortcuts can be turned off/remapped | No single-key shortcuts, or provide settings | Required |
| 2.4.1 Bypass Blocks | A | Skip-to-content link | First focusable element is "Skip to main content" | Required |
| 2.4.2 Page Titled | A | Descriptive page titles | Dynamic `<title>` per route | Required |
| 2.4.3 Focus Order | A | Focus order is logical and meaningful | DOM order matches visual flow, tabindex managed | Required |
| 2.4.4 Link Purpose (In Context) | A | Link text describes destination | Descriptive link text, no "click here" | Required |
| 2.4.5 Multiple Ways | AA | Multiple ways to find pages | Navigation + search + sitemap | Required |
| 2.4.6 Headings and Labels | AA | Headings and labels are descriptive | Meaningful heading hierarchy per page | Required |
| 2.4.7 Focus Visible | AA | Keyboard focus indicator is visible | Custom focus ring (2px solid, high contrast) | Required |
| 2.5.1 Pointer Gestures | A | No multi-point or path-based gestures required | Alternative single-pointer actions | Required |
| 2.5.2 Pointer Cancellation | A | Down-event does not trigger action | Use click/up events, not mousedown | Required |
| 2.5.3 Label in Name | A | Accessible name contains visible label text | `aria-label` includes visible text | Required |
| 2.5.4 Motion Actuation | A | Motion-triggered actions have alternatives | No shake/tilt-only actions | If applicable |

### Understandable

| Criterion | Level | Requirement | Implementation | Status |
|-----------|-------|------------|----------------|--------|
| 3.1.1 Language of Page | A | Page language is declared | `lang` attribute on `<html>` | Required |
| 3.1.2 Language of Parts | AA | Language changes are marked | `lang` attribute on elements with different language | If applicable |
| 3.2.1 On Focus | A | Focus does not trigger unexpected changes | No context changes on focus alone | Required |
| 3.2.2 On Input | A | Input does not trigger unexpected changes | No auto-submit on input, warn before navigation | Required |
| 3.2.3 Consistent Navigation | AA | Navigation is consistent across pages | Same nav position and order on all pages | Required |
| 3.2.4 Consistent Identification | AA | Same functionality has same labels | Consistent button/link text for same actions | Required |
| 3.3.1 Error Identification | A | Errors are identified and described in text | Inline error messages with field association | Required |
| 3.3.2 Labels or Instructions | A | Form inputs have labels | `<label>` elements, `aria-describedby` for instructions | Required |
| 3.3.3 Error Suggestion | AA | Error messages suggest corrections | "Email must be in format user@domain.com" | Required |
| 3.3.4 Error Prevention (Legal, Financial, Data) | AA | Reversible, checked, or confirmed submissions | Confirmation dialogs for destructive/irreversible actions | Required |

### Robust

| Criterion | Level | Requirement | Implementation | Status |
|-----------|-------|------------|----------------|--------|
| 4.1.1 Parsing | A | Valid HTML | HTML validation in CI | Required |
| 4.1.2 Name, Role, Value | A | Custom components have ARIA roles | ARIA attributes on all custom interactive elements | Required |
| 4.1.3 Status Messages | AA | Status messages announced without focus | `role="status"`, `aria-live="polite"` | Required |

---

## Step 2: ARIA Role Inventory

Define the ARIA roles and attributes for every interactive component:

### Landmark Roles

| Role | Element | Usage |
|------|---------|-------|
| `banner` | `<header>` | Site header (one per page) |
| `navigation` | `<nav>` | Primary and secondary navigation (label each) |
| `main` | `<main>` | Primary content area (one per page) |
| `complementary` | `<aside>` | Sidebar, supplementary content |
| `contentinfo` | `<footer>` | Site footer (one per page) |
| `search` | `<search>` or `role="search"` | Search functionality |
| `form` | `<form>` | Named forms with `aria-label` |

### Widget Roles per Component

For each component in the design system:

```markdown
### [Component Name]

**ARIA role:** [role or native element]
**Required attributes:**
| Attribute | Value | Purpose |
|-----------|-------|---------|
| | | |

**State attributes:**
| Attribute | When | Value |
|-----------|------|-------|
| `aria-expanded` | Toggleable content | true/false |
| `aria-selected` | Selectable items | true/false |
| `aria-disabled` | Disabled state | true |
| `aria-busy` | Loading state | true |
| `aria-invalid` | Validation error | true |

**Relationships:**
| Attribute | Points To | Purpose |
|-----------|----------|---------|
| `aria-labelledby` | Heading/label ID | Names the component |
| `aria-describedby` | Description ID | Provides additional context |
| `aria-controls` | Controlled element ID | Indicates what this controls |
| `aria-owns` | Owned element ID | For DOM-disconnected children |
```

Common components to define:
- **Button:** native `<button>`, `aria-pressed` for toggles
- **Dialog/Modal:** `role="dialog"`, `aria-modal="true"`, `aria-labelledby`
- **Tabs:** `role="tablist"`, `role="tab"`, `role="tabpanel"`, `aria-selected`
- **Dropdown/Menu:** `role="menu"`, `role="menuitem"`, `aria-expanded`
- **Combobox/Autocomplete:** `role="combobox"`, `aria-autocomplete`, `aria-activedescendant`
- **Alert/Toast:** `role="alert"` (assertive) or `role="status"` (polite)
- **Table:** native `<table>` with `<caption>`, `<th scope>`
- **Accordion:** `aria-expanded`, `aria-controls`
- **Progress:** `role="progressbar"`, `aria-valuenow`, `aria-valuemin`, `aria-valuemax`
- **Tooltip:** `role="tooltip"`, `aria-describedby` on trigger

---

## Step 3: Keyboard Navigation Flow

### Global Keyboard Shortcuts

| Key | Action | Context |
|-----|--------|---------|
| `Tab` | Move focus to next interactive element | Global |
| `Shift+Tab` | Move focus to previous interactive element | Global |
| `Escape` | Close modal/popover/dropdown, cancel current action | When overlay is open |
| `Enter` | Activate focused element | Buttons, links |
| `Space` | Activate focused element (buttons), toggle (checkboxes) | Interactive elements |

### Component-Level Keyboard Patterns

For each interactive component, define the keyboard interaction:

```markdown
### [Component Name] Keyboard Interaction

| Key | Action |
|-----|--------|
| `Tab` | Move focus into/out of component |
| `Arrow Up/Down` | Navigate between items |
| `Arrow Left/Right` | Navigate between tabs/segments |
| `Home` | Move to first item |
| `End` | Move to last item |
| `Enter`/`Space` | Select/activate current item |
| `Escape` | Close/dismiss |
| `Type-ahead` | Focus item matching typed characters |
```

### Focus Order Map

For each page/view, define the tab order:

```markdown
### Page: [Name]

Focus order:
1. Skip-to-content link
2. Primary navigation items (left to right)
3. Search input (if present)
4. Page heading (if actionable)
5. Main content interactive elements (top to bottom, left to right)
6. Sidebar interactive elements (if present)
7. Footer links
```

### Focus Trap Behavior (Modals)

When a modal is open:
1. Focus moves to the first focusable element inside the modal (or the modal container if no focusable elements)
2. Tab cycles through focusable elements within the modal only
3. `Escape` closes the modal
4. On close, focus returns to the element that triggered the modal
5. Background content receives `aria-hidden="true"` and `inert`

---

## Step 4: Screen Reader Announcement Strategy

### Live Regions

Define what content is announced dynamically:

| Event | Region Type | Politeness | Implementation |
|-------|-----------|-----------|----------------|
| Form validation error | `aria-live` | `assertive` | Error message container with `role="alert"` |
| Success confirmation | `aria-live` | `polite` | Toast/banner with `role="status"` |
| Loading state change | `aria-live` | `polite` | "Loading..." and "Content loaded" announcements |
| List item count change | `aria-live` | `polite` | "X results found" after filtering |
| Navigation/route change | N/A | N/A | Update `<title>`, move focus to `<h1>` or main content |
| Real-time updates | `aria-live` | `polite` | Announce new messages/notifications |

### Announcement Content Guidelines

- Announcements must be concise (under 100 characters when possible)
- Use consistent phrasing: "X items loaded", "Error: [description]", "Success: [description]"
- Do not announce changes that the user did not initiate
- For lists: announce the count after filtering, not each item individually
- For progress: announce start and completion, not every percentage change

### Hidden Content for Screen Readers

| Pattern | Implementation | When to Use |
|---------|---------------|-------------|
| Visually hidden, announced | `.sr-only` CSS class | Labels for icon-only buttons, additional context |
| Visible, not announced | `aria-hidden="true"` | Decorative icons, redundant text |
| Hidden from everyone | `display: none` or `hidden` | Inactive content |

`.sr-only` CSS pattern:
```css
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}
```

---

## Step 5: Color Contrast Requirements

### Contrast Ratios

Using the color palette from `DESIGN_SYSTEM.md`, verify every color combination:

| Foreground | Background | Contrast Ratio | WCAG AA (4.5:1) | WCAG AAA (7:1) | Usage |
|-----------|-----------|---------------|-----------------|-----------------|-------|
| Text Primary | Background | [ratio] | Pass/Fail | Pass/Fail | Body text |
| Text Secondary | Background | [ratio] | Pass/Fail | Pass/Fail | Secondary text |
| Text Primary | Surface | [ratio] | Pass/Fail | Pass/Fail | Card text |
| Error text | Background | [ratio] | Pass/Fail | Pass/Fail | Error messages |
| Link color | Background | [ratio] | Pass/Fail | Pass/Fail | Hyperlinks |
| Button text | Button bg | [ratio] | Pass/Fail | Pass/Fail | Button labels |
| Placeholder text | Input bg | [ratio] | Pass/Fail | Pass/Fail | Placeholder (3:1 min for AA) |
| Focus ring | Background | [ratio] | 3:1 min | N/A | Focus indicators |
| Icon (informational) | Background | [ratio] | 3:1 min | N/A | UI icons |

### Non-Text Contrast (1.4.11)

UI components and graphical objects that convey meaning must have a 3:1 contrast ratio:

| Element | Adjacent Color | Contrast Ratio | Pass/Fail |
|---------|---------------|---------------|-----------|
| Input border | Background | [ratio] | |
| Button border | Background | [ratio] | |
| Focus indicator | Background | [ratio] | |
| Chart data series | Adjacent series | [ratio] | |
| Toggle (active/inactive) | Background | [ratio] | |

### Dark Mode Considerations (if applicable)

- All contrast ratios must be re-verified for dark mode palette
- Avoid pure white text on pure black backgrounds (use off-white/off-black for reduced eye strain)
- Ensure images and icons remain visible in dark mode

---

## Step 6: Focus Management Rules

### Focus Indicator Style

```css
/* Default focus indicator */
:focus-visible {
  outline: 2px solid [primary-color];
  outline-offset: 2px;
  border-radius: [radius-sm];
}

/* High-contrast mode override */
@media (forced-colors: active) {
  :focus-visible {
    outline: 2px solid LinkText;
  }
}
```

Requirements:
- Focus indicator must have a minimum 3:1 contrast ratio against the background
- Focus indicator must be visible on all backgrounds (light and dark)
- Focus indicator must enclose or be adjacent to the focused element (not just a color change)
- Never use `outline: none` without a visible replacement

### Focus Management for Dynamic Content

| Scenario | Focus Action |
|----------|-------------|
| Modal opens | Move focus to first focusable element inside modal |
| Modal closes | Return focus to trigger element |
| Inline content expands (accordion) | Move focus to expanded content heading |
| Item deleted from list | Move focus to nearest remaining item or parent container |
| Navigation (SPA route change) | Move focus to `<h1>` of new page, update `<title>` |
| Error on form submit | Move focus to first field with an error |
| Toast/notification appears | Do not move focus (use `aria-live` instead) |
| Infinite scroll loads more | Do not move focus (keep current position) |
| Drawer/sidebar opens | Move focus to first focusable element in drawer |
| Drawer/sidebar closes | Return focus to trigger element |

---

## Step 7: Reduced Motion and User Preferences

### `prefers-reduced-motion` Support

```css
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
    scroll-behavior: auto !important;
  }
}
```

For each animation in the design system, define the reduced-motion alternative:

| Animation | Default Behavior | Reduced Motion Alternative |
|-----------|-----------------|---------------------------|
| Page transitions | Slide/fade (200ms) | Instant swap |
| Loading spinners | Rotating animation | Static "Loading..." text |
| Hover effects | Scale/transform (100ms) | Opacity change or no effect |
| Toast entrance | Slide in (200ms) | Appear immediately |
| Skeleton loaders | Shimmer animation | Static gray blocks |
| Scroll-linked animations | Parallax/reveal effects | No effect |

### `prefers-contrast` Support

```css
@media (prefers-contrast: more) {
  /* Increase border widths, text weight, and contrast ratios */
}

@media (prefers-contrast: less) {
  /* Soften contrast for users who find high contrast uncomfortable */
}
```

### `prefers-color-scheme` Support

If the product supports dark mode, respect the user's system preference:
```css
@media (prefers-color-scheme: dark) {
  /* Apply dark mode tokens */
}
```

---

## Step 8: Testing Requirements

### Automated Testing

| Tool | Tests | When to Run |
|------|-------|-------------|
| axe-core | WCAG violations | Every component test, CI/CD |
| Lighthouse accessibility audit | Overall score | Every PR |
| HTML validator | Markup validity | CI/CD |
| Color contrast checker | Contrast ratios | Design system changes |

### Manual Testing Checklist

For each component and page, verify:

- [ ] Fully operable with keyboard only (no mouse)
- [ ] Focus order is logical and matches visual order
- [ ] Focus indicator is visible on all backgrounds
- [ ] Screen reader announces component purpose, state, and changes
- [ ] Error messages are announced and associated with fields
- [ ] Works with 200% zoom without loss of content or functionality
- [ ] Works with user-defined text spacing (1.5x line-height, 0.12em letter-spacing)
- [ ] Reduced motion preference is respected
- [ ] All images have appropriate alt text
- [ ] Color is not the only means of conveying information

### Screen Reader Testing Matrix

| Screen Reader | Browser | OS | Priority |
|-------------|---------|-----|----------|
| VoiceOver | Safari | macOS | High |
| NVDA | Firefox | Windows | High |
| JAWS | Chrome | Windows | Medium |
| TalkBack | Chrome | Android | Medium |
| VoiceOver | Safari | iOS | Medium |

---

## Step 9: Cross-Reference Validation

Before finalizing, verify:

- [ ] Every component in the design system has ARIA roles and keyboard interaction defined
- [ ] All color combinations pass WCAG AA contrast minimums
- [ ] Every page has a defined focus order and heading hierarchy
- [ ] Live region strategy covers all dynamic content updates
- [ ] Focus management handles all modal/drawer/navigation scenarios
- [ ] Reduced motion alternatives exist for all animations
- [ ] Testing plan covers automated and manual verification
- [ ] Accessibility requirements are included in the component acceptance criteria

---

## Output

The final output feeds into the accessibility sections of `DESIGN_SYSTEM.md`, `ARCHITECTURE.md`, and the testing strategy. Every component spec must reference its accessibility requirements from this document.
