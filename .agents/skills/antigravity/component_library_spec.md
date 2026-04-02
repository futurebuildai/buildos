---
description: Specify the UI component library. For each component, define variants, props/slots/events, design token mappings, accessibility requirements, composition patterns, and responsive behavior. Grounded in component pattern research.
invoked_from:
  - workflows/antigravity/05_design_system.md
  - workflows/antigravity/06_product_spec.md
  - workflows/antigravity/feature_iteration.md
produces:
  - Component inventory with full API specification (props, slots, events)
  - Variant definitions per component
  - Design token mappings per component
  - Accessibility requirements per component (ARIA, keyboard, focus)
  - Composition patterns (how components combine)
  - Responsive behavior rules per component
browser_usage: Moderate (research component patterns from established design systems, framework-specific component libraries)
---

# Skill: Component Library Specification

Specify every UI component the product needs. Each component is defined precisely enough that a developer can implement it without guessing — variants, props, slots, events, tokens, accessibility, composition, and responsive behavior are all explicit.

---

## Prerequisites

Before invoking this skill, ensure the following exist:

- `TECH_STACK.md` — for frontend framework (determines component model: props/slots vs. props/children, event syntax)
- `DESIGN_SYSTEM.md` — for design tokens (colors, spacing, typography, elevation)
- `INFORMATION_ARCHITECTURE.md` — for understanding which pages need which components
- `accessibility_spec.md` output (if already produced) — for per-component ARIA requirements

---

## Step 1: Research Component Patterns

**Browser: 4-6 searches**

1. Search for "[frontend framework from TECH_STACK.md] component library best practices"
2. Search for "[styling approach from TECH_STACK.md] component patterns" (e.g., "Tailwind component patterns", "CSS Modules component patterns")
3. Study established component libraries for pattern inspiration:
   - Radix UI (headless, accessibility-first)
   - Shadcn/ui (Radix + Tailwind)
   - Material UI / Material Design 3
   - Ant Design
   - Headless UI (Tailwind Labs)
4. Search for "[product domain] UI component patterns" (e.g., "dashboard component patterns", "e-commerce product card patterns")
5. Search for "[framework] compound component pattern" if building complex components

Record findings for reference in subsequent steps.

---

## Step 2: Component Inventory

Categorize all required components using the atomic design hierarchy:

### Atoms (Smallest reusable units)

| Component | Variants | Priority | Used In |
|-----------|----------|----------|---------|
| Button | primary, secondary, ghost, destructive, link | P0 | Everywhere |
| Input | text, email, password, number, search, textarea | P0 | All forms |
| Checkbox | default, indeterminate | P0 | Forms, tables |
| Radio | default | P0 | Forms |
| Toggle/Switch | default | P0 | Settings |
| Select | single, searchable | P0 | Forms |
| Badge | default, outline | P1 | Lists, tables |
| Avatar | image, initials, fallback | P1 | Headers, lists |
| Icon | [icon set name] | P0 | Everywhere |
| Spinner/Loader | default | P0 | Loading states |
| Skeleton | text, circle, rectangle | P1 | Loading states |
| Divider | horizontal, vertical | P2 | Layout |
| Label | default, required | P0 | Forms |
| Link | default, external | P0 | Navigation, content |

### Molecules (Combinations of atoms)

| Component | Contains | Priority | Used In |
|-----------|----------|----------|---------|
| FormField | Label + Input + HelperText + ErrorMessage | P0 | All forms |
| Card | Header + Body + Footer + Actions | P0 | Dashboards, lists |
| ListItem | Avatar/Icon + Content + Metadata + Actions | P0 | Lists, navigation |
| Alert | Icon + Title + Description + Actions | P0 | Notifications |
| Toast | Icon + Message + Action + Dismiss | P0 | Global notifications |
| EmptyState | Illustration + Title + Description + CTA | P1 | Lists, search results |
| SearchInput | Input + Icon + Clear button | P1 | Search functionality |
| Breadcrumb | Link chain + Separator + Current page | P1 | Navigation |
| Pagination | Page numbers + Navigation arrows | P1 | Tables, lists |
| Tag/Chip | Label + Optional icon + Optional remove | P1 | Filters, categories |

### Organisms (Complex, self-contained sections)

| Component | Contains | Priority | Used In |
|-----------|----------|----------|---------|
| Navigation | Logo + NavItems + UserMenu | P0 | App shell |
| DataTable | Header + Rows + Sorting + Filtering + Pagination | P0 | Data views |
| Modal/Dialog | Overlay + Header + Body + Footer | P0 | Confirmations, forms |
| Form | FormFields + Validation + Submit | P0 | Data entry |
| PageHeader | Title + Breadcrumb + Actions | P0 | Every page |
| Sidebar | Navigation + Sections + Collapse toggle | P1 | App layout |
| Dropdown/Menu | Trigger + Menu items + Separator + Keyboard nav | P0 | Actions, navigation |
| CommandPalette | Search + Results + Keyboard nav | P2 | Power users |
| Tabs | TabList + TabPanels | P1 | Content organization |
| Accordion | Items + Expand/Collapse | P1 | FAQs, settings |

---

## Step 3: Component Specification Template

For **each** component, produce a full specification:

```markdown
## [Component Name]

### Overview
**Purpose:** [What problem this component solves]
**Category:** Atom / Molecule / Organism
**Priority:** P0 / P1 / P2

### Variants

| Variant | Visual Difference | When to Use |
|---------|------------------|-------------|
| primary | [description] | [guidance] |
| secondary | [description] | [guidance] |
| ... | ... | ... |

### Sizes

| Size | Height | Padding | Font Size | Icon Size | When to Use |
|------|--------|---------|-----------|-----------|-------------|
| sm | [token] | [token] | [token] | [token] | Compact contexts |
| md (default) | [token] | [token] | [token] | [token] | Standard usage |
| lg | [token] | [token] | [token] | [token] | Touch targets, emphasis |

### Props / Attributes

| Prop | Type | Default | Required | Description |
|------|------|---------|----------|-------------|
| variant | string enum | 'primary' | No | Visual variant |
| size | string enum | 'md' | No | Component size |
| disabled | boolean | false | No | Disabled state |
| loading | boolean | false | No | Loading state |
| ... | ... | ... | ... | ... |

### Slots / Children

| Slot | Accepts | Default | Description |
|------|---------|---------|-------------|
| default | text / elements | Required | Primary content |
| icon | Icon component | None | Leading icon |
| ... | ... | ... | ... |

### Events / Callbacks

| Event | Payload | When Emitted |
|-------|---------|-------------|
| onClick / on:click | MouseEvent | User clicks (not when disabled/loading) |
| onFocus / on:focus | FocusEvent | Component receives focus |
| ... | ... | ... |

### States

| State | Visual Treatment | ARIA Attribute |
|-------|-----------------|----------------|
| Default | [description] | None |
| Hover | [description] | N/A |
| Focus | [focus ring style] | N/A (browser-managed) |
| Active/Pressed | [description] | `aria-pressed` (if toggle) |
| Disabled | [description, opacity] | `aria-disabled="true"` |
| Loading | [description, spinner] | `aria-busy="true"` |
| Error | [description, color] | `aria-invalid="true"` |

### Design Token Mappings

| Property | Token | Variant Override |
|----------|-------|-----------------|
| Background | `color.primary.default` | secondary: `color.neutral.100` |
| Text color | `color.primary.foreground` | secondary: `color.neutral.900` |
| Border radius | `radius.sm` | |
| Padding X | `space.md` | sm: `space.sm`, lg: `space.lg` |
| Padding Y | `space.sm` | sm: `space.xs`, lg: `space.md` |
| Font size | `text.sm` | sm: `text.xs`, lg: `text.md` |
| Font weight | `font.semibold` | |
| Transition | `duration.fast` `easing.ease-out` | |

### Accessibility

| Requirement | Implementation |
|------------|----------------|
| Role | `<button>` native element (not `div` with `role="button"`) |
| Keyboard | `Enter` and `Space` activate; `Tab` to navigate |
| Focus indicator | 2px solid ring, [contrast ratio] against background |
| Screen reader | Announces: label text + variant context if destructive |
| ARIA attributes | `aria-disabled`, `aria-busy`, `aria-pressed` (toggles only) |
| Color independence | Icon + text, not color alone for variant meaning |

### Composition Patterns

**Standalone usage:**
```jsx
<Button variant="primary" size="md">Save Changes</Button>
```

**With icon:**
```jsx
<Button variant="primary">
  <Icon name="save" slot="icon" />
  Save Changes
</Button>
```

**In a button group:**
```jsx
<ButtonGroup>
  <Button variant="secondary">Cancel</Button>
  <Button variant="primary">Save</Button>
</ButtonGroup>
```

**Loading state:**
```jsx
<Button variant="primary" loading>
  Saving...
</Button>
```

### Responsive Behavior

| Breakpoint | Behavior |
|-----------|----------|
| Mobile (< 768px) | Full width in forms, icon-only in toolbars |
| Tablet (768-1023px) | Standard sizing |
| Desktop (>= 1024px) | Standard sizing |

### Do / Don't

| Do | Don't |
|----|-------|
| Use `primary` for the main action on a page | Use multiple `primary` buttons in the same section |
| Use `destructive` for irreversible actions | Use `destructive` for cancel/dismiss |
| Include loading state for async operations | Disable button without visual feedback |
| Use icon + text together | Use icon-only without `aria-label` |
```

---

## Step 4: Complex Component Patterns

### Compound Components

For components with multiple interrelated parts (Tabs, Accordion, DataTable), define the compound pattern:

```markdown
### Compound Component: Tabs

**Container:** `<Tabs>` — manages active state, provides context
**Parts:**
- `<TabList>` — contains tab triggers, handles keyboard navigation
- `<Tab>` — individual tab trigger, receives active state from context
- `<TabPanel>` — content associated with a tab, shown/hidden by context

**State flow:**
- `Tabs` owns the `activeTab` state
- `Tab` reads active state from context, emits `onSelect`
- `TabPanel` reads active state to show/hide

**Keyboard navigation within TabList:**
- `ArrowLeft/Right` moves between tabs
- `Home` moves to first tab
- `End` moves to last tab
- `Enter/Space` activates focused tab

**Composition:**
```jsx
<Tabs defaultValue="tab1" onValueChange={handleChange}>
  <TabList>
    <Tab value="tab1">General</Tab>
    <Tab value="tab2">Security</Tab>
    <Tab value="tab3">Notifications</Tab>
  </TabList>
  <TabPanel value="tab1">General settings content</TabPanel>
  <TabPanel value="tab2">Security settings content</TabPanel>
  <TabPanel value="tab3">Notification settings content</TabPanel>
</Tabs>
```
```

### Controlled vs. Uncontrolled

For stateful components, define both controlled and uncontrolled usage:

| Mode | Props | Use Case |
|------|-------|----------|
| Uncontrolled | `defaultValue`, internal state | Simple forms, quick usage |
| Controlled | `value` + `onChange`, external state | Complex forms, cross-component sync |

---

## Step 5: Responsive Component Rules

### Global Responsive Principles

| Principle | Rule |
|-----------|------|
| Touch targets | Minimum 44x44px on mobile |
| Spacing | Increase padding on touch devices |
| Typography | Minimum 16px body text on mobile (prevents iOS zoom) |
| Stacking | Horizontal layouts stack vertically on mobile |
| Hiding | Use `display: none` at breakpoints, not `visibility: hidden` |
| Overflow | Horizontal scroll for data tables on mobile, never for page layout |

### Per-Component Responsive Rules

For each component, define breakpoint-specific behavior:

```markdown
### [Component]: Responsive Behavior

| Property | Mobile (< 768px) | Tablet (768-1023px) | Desktop (>= 1024px) |
|----------|------------------|--------------------|--------------------|
| Layout | [stack/collapse/hide] | [adapt] | [default] |
| Size | [adjusted] | [default] | [default] |
| Content | [truncated/hidden] | [default] | [default] |
| Interaction | [touch-optimized] | [default] | [default] |
```

---

## Step 6: Design Token Integration

### Token Consumption Rules

1. **Never use raw values** — always reference design tokens
2. **Component tokens derive from global tokens** — e.g., `button.primary.bg` derives from `color.primary.default`
3. **Dark mode is handled at the token level** — components do not have dark mode logic, tokens switch values
4. **Spacing uses the spacing scale** — never arbitrary pixel values

### Component-Level Token Definitions

```css
/* Example: Button component tokens */
--button-primary-bg: var(--color-primary-default);
--button-primary-text: var(--color-primary-foreground);
--button-primary-hover-bg: var(--color-primary-hover);
--button-primary-active-bg: var(--color-primary-active);

--button-secondary-bg: var(--color-neutral-100);
--button-secondary-text: var(--color-neutral-900);
--button-secondary-border: var(--color-neutral-300);

--button-border-radius: var(--radius-sm);
--button-font-weight: var(--font-weight-semibold);
--button-transition: var(--duration-fast) var(--easing-ease-out);

/* Size-specific tokens */
--button-sm-height: 32px;
--button-sm-padding-x: var(--space-sm);
--button-sm-font-size: var(--text-xs);

--button-md-height: 40px;
--button-md-padding-x: var(--space-md);
--button-md-font-size: var(--text-sm);

--button-lg-height: 48px;
--button-lg-padding-x: var(--space-lg);
--button-lg-font-size: var(--text-md);
```

---

## Step 7: Cross-Reference Validation

Before finalizing, verify:

- [ ] Every screen in `INFORMATION_ARCHITECTURE.md` can be built with the defined components
- [ ] Every component has all variants needed by the UI designs
- [ ] Props/slots/events are consistent in naming across all components
- [ ] Design tokens cover every visual property (no hardcoded values)
- [ ] Accessibility requirements are defined for every interactive component
- [ ] Responsive behavior is defined for every component
- [ ] Composition patterns show how components work together
- [ ] Components follow the framework conventions from TECH_STACK.md

---

## Output

The final output feeds into the Component Architecture section of `DESIGN_SYSTEM.md` and serves as the implementation spec for the component library. Each component specification should be detailed enough that a developer can implement it without design ambiguity.
