---
description: Define information architecture, design system foundations, component patterns, and UX interaction flows grounded in competitive and pattern research.
stage: "05"
inputs: USER_JOURNEYS.md, SCOPE_DEFINITION.md, COMPETITIVE_LANDSCAPE.md, PERSONAS.md
outputs: INFORMATION_ARCHITECTURE.md, DESIGN_SYSTEM.md
browser_usage: Moderate (design pattern research, component library research)
---

# Stage 05: Design System

Build the structural and visual foundation that all UI implementation will follow. This stage produces the design language and information structure — not page-level mockups (those come in Stage 06).

---

## Step 1: Information Architecture

### 1a: Content Inventory
List every type of content/data the user will interact with based on SCOPE_DEFINITION.md:
- Data objects (e.g., projects, tasks, users, messages)
- Actions (e.g., create, edit, delete, share, export)
- Navigation contexts (e.g., dashboard, settings, detail views)
- System messages (e.g., notifications, errors, confirmations)

### 1b: Navigation Model

**Browser: 2-3 searches**
- Search for "[product type] navigation patterns"
- Analyze how competitors (from COMPETITIVE_LANDSCAPE.md) structure their navigation

Define:
- **Primary navigation** — always visible, core product areas
- **Secondary navigation** — contextual, within a primary area
- **Utility navigation** — settings, account, help
- **Pattern choice** — sidebar, top nav, tab bar, breadcrumb (with rationale)

### 1c: Screen Inventory
For each MVP capability, list every unique screen/view:

| Screen | Purpose | Content Displayed | Available Actions | Navigation To/From |
|--------|---------|-------------------|-------------------|-------------------|

### 1d: URL/Route Structure (for web)
```
/                          → Dashboard / Home
/[resource]                → List view
/[resource]/:id            → Detail view
/[resource]/:id/edit       → Edit view
/settings                  → Settings
/settings/[section]        → Settings subsection
```

### 1e: Taxonomy & Naming
Define the vocabulary used in navigation and labels. Use user language (from PERSONAS.md), not engineering jargon.

| Concept | User-Facing Term | Why This Term |
|---------|-----------------|---------------|

Write `.agents/handoff/INFORMATION_ARCHITECTURE.md`.

## Step 2: Design Principles

Define 3-5 principles that guide all design decisions. Ground them in user research:

```markdown
## Design Principles
1. **[Principle]** — [What it means in practice]. Derived from: [research finding].
2. **[Principle]** — [What it means in practice]. Derived from: [research finding].
3. **[Principle]** — [What it means in practice]. Derived from: [research finding].
```

## Step 3: Visual Foundation

**Browser: 5-8 searches**
- Search for "[product domain] design system examples"
- Search for "[brand adjective] color palette design" (using positioning from VISION_BRIEF.md)
- Look at design system references: Material Design 3, Radix, Shadcn/ui, Apple HIG
- Research accessibility requirements: WCAG 2.1 AA color contrast minimums

### Color System
```markdown
## Colors
### Primary Palette
- Primary: [hex] — used for: CTAs, key actions, brand presence
- Primary Hover: [hex]
- Primary Active: [hex]

### Neutral Palette
- Background: [hex]
- Surface: [hex]
- Border: [hex]
- Text Primary: [hex]
- Text Secondary: [hex]
- Text Disabled: [hex]

### Semantic Palette
- Success: [hex] — used for: confirmations, positive states
- Warning: [hex] — used for: caution states
- Error: [hex] — used for: errors, destructive actions
- Info: [hex] — used for: informational states

### Contrast Compliance
| Foreground | Background | Ratio | WCAG AA | WCAG AAA |
|-----------|-----------|-------|---------|----------|
```

### Typography System
```markdown
## Typography
- **Font Family (Headings):** [font] — rationale: [why]
- **Font Family (Body):** [font] — rationale: [why]
- **Font Family (Code):** [monospace font]

| Level | Size | Weight | Line Height | Letter Spacing | Usage |
|-------|------|--------|------------|----------------|-------|
| Display | | | | | Hero sections |
| H1 | | | | | Page titles |
| H2 | | | | | Section titles |
| H3 | | | | | Subsections |
| Body | | | | | Default text |
| Body Small | | | | | Secondary text |
| Caption | | | | | Labels, metadata |
| Code | | | | | Code blocks |
```

### Spacing System
```markdown
## Spacing
- **Base unit:** [4px / 8px]
- **Scale:** [list: 0, 1, 2, 3, 4, 5, 6, 8, 10, 12, 16]

| Token | Value | Usage |
|-------|-------|-------|
| space-xs | Xpx | Tight spacing within components |
| space-sm | Xpx | Default component padding |
| space-md | Xpx | Between components |
| space-lg | Xpx | Section spacing |
| space-xl | Xpx | Page-level spacing |
```

### Elevation & Borders
```markdown
## Elevation
| Level | Shadow | Usage |
|-------|--------|-------|
| 0 | none | Flat elements |
| 1 | [value] | Cards, surfaces |
| 2 | [value] | Dropdowns, popovers |
| 3 | [value] | Modals, dialogs |

## Border Radius
| Token | Value | Usage |
|-------|-------|-------|
| radius-sm | | Buttons, inputs |
| radius-md | | Cards |
| radius-lg | | Modals |
| radius-full | 9999px | Avatars, pills |
```

## Step 4: Component Patterns

**Browser: 3-5 searches**
- Search for "[tech stack frontend framework] component library"
- Research component patterns from established design systems

Define the core component library for the MVP:

### Atoms
For each: variants, states (default, hover, focus, active, disabled, loading), sizes

- **Button** — primary, secondary, ghost, destructive variants
- **Input** — text, number, email, password + error/success states
- **Checkbox / Radio / Toggle**
- **Badge / Tag**
- **Avatar**
- **Icon system** — which icon set, naming convention

### Molecules
- **Form Field** — label + input + helper text + error message
- **Card** — header, body, footer, actions
- **List Item** — icon + content + metadata + actions
- **Alert / Toast** — success, warning, error, info
- **Empty State** — illustration + message + CTA

### Organisms
- **Navigation** — primary nav pattern from IA
- **Data Table** — sorting, filtering, pagination, row actions
- **Modal / Dialog** — confirmation, form, info
- **Form** — multi-field layout, validation patterns, submit states
- **Page Header** — title, breadcrumbs, actions

## Step 5: Interaction Patterns

### State Patterns
Define how every interactive element communicates state:
- **Loading** — skeleton screens vs. spinners vs. progress bars (with guidance on when to use each)
- **Empty** — empty state design with helpful CTAs
- **Error** — inline errors, toast notifications, error pages
- **Success** — confirmation patterns
- **Optimistic updates** — if applicable per TECH_STACK.md

### Motion & Animation
```markdown
## Motion
- **Duration Scale:** instant (0ms), fast (100ms), normal (200ms), slow (300ms), deliberate (500ms)
- **Easing:** ease-out for entrances, ease-in for exits, ease-in-out for state changes
- **Principles:** Purposeful (aids comprehension), subtle (not distracting), consistent

| Pattern | Duration | Easing | Usage |
|---------|----------|--------|-------|
| Hover state | fast | ease-out | Buttons, links, cards |
| Page transition | normal | ease-in-out | Route changes |
| Modal open | normal | ease-out | Dialog entrance |
| Toast notification | fast | ease-out | Alert appearance |
| Skeleton → content | normal | ease-out | Content loading |
```

### Responsive Design
```markdown
## Breakpoints
| Name | Min Width | Columns | Margin | Gutter |
|------|----------|---------|--------|--------|
| mobile | 0px | | | |
| tablet | 768px | | | |
| desktop | 1024px | | | |
| wide | 1440px | | | |
```

## Step 6: Accessibility Requirements

- Minimum color contrast: 4.5:1 for normal text, 3:1 for large text (WCAG AA)
- All interactive elements must be keyboard accessible
- Focus indicators: visible, high-contrast focus ring on all interactive elements
- ARIA landmarks for page structure
- ARIA labels for icon-only buttons
- Skip-to-content link
- Reduced motion: respect `prefers-reduced-motion` media query

Write `.agents/handoff/DESIGN_SYSTEM.md`.

## Step 7: Update Pipeline State

```markdown
## Stage 05 — Design System ✅
- INFORMATION_ARCHITECTURE.md produced ([N] screens inventoried)
- DESIGN_SYSTEM.md produced (colors, type, spacing, [N] component patterns)
## Next Stage: 06 — Product Specification
```

Proceed to Stage 06.
