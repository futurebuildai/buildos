# Brand Spec — SHBC Fork (industrial dark)

Visual language for the demo. Aligned with the **FutureBuild industrial
dark theme** used across GableLBM, BuildOS, and LocalBlue surfaces. The
SHBC co-brand surfaces in the logo and the wood-tone bylaw banner; the
rest of the surface is the FutureBuild aesthetic.

## Palette

```
/* Surface stack */
--bg-deep:        #0A0B10   /* page background — deep-space */
--bg-panel:       #12131A   /* secondary panel */
--bg-surface:     #161821   /* card / panel — slate-steel */
--bg-raised:      #1a1c26   /* hover / raised */
--bg-input:       #0f1117   /* form fields */

/* Borders */
--border-default: #1f222e
--border-strong:  #2a2e3d
--border-faint:   #15171f

/* Text */
--text-primary:   #ffffff
--text-secondary: #c8ccd6
--text-muted:     #7a8092
--text-faint:     #4a4e5b

/* Accents */
--accent:         #00FFA3   /* gable-green — FutureBuild signature */
--accent-soft:    #00e593
--shbc-cedar:     #2c6850   /* SHBC secondary */
--shbc-wood:      #d6a778   /* warm cedar accent */

/* Status colors — calibrated against the dark surface */
--status-critical: #ef4444   /* rose-red */
--status-warning:  #f59e0b   /* amber */
--status-ok:       #10b981   /* emerald */
--status-ordered:  #38bdf8   /* sky */
--status-info:     #6366f1   /* indigo */
--status-blocked:  #f97316   /* orange */
```

## Typography

- **All surfaces:** `"Inter", "Söhne", system-ui, sans-serif` — clean
  geometric sans, identical to the FutureBuild website
- **Numeric / tabular:** `"JetBrains Mono", ui-monospace, monospace` —
  for costs, schedule offsets, dates, IDs

(System stacks since we don't bundle fonts; the goal is the FutureBuild
"industrial OS" feel.)

## Voice

- Use "your project," "your crew," "your cohort" — second-person, plural
  where the cohort is involved.
- Plain language: "Permit pending" not "Awaiting jurisdictional approval."
- Numbers always include the currency: `$1,800,000 CAD` not `$1.8M`.
- Dates in plain English: "Wednesday, May 6" not "2026-05-06T00:00:00Z".
- Status pills are SHORT and capitalized: **CRITICAL · WARNING · OK ·
  ORDERED**. The pill colors come from `--status-*` tokens.
- Section headers and metadata labels: small-caps uppercase tracking
  (10px, letter-spacing 0.1em). Body text 14px, line-height 1.5.

## Co-branding rule

The footer reads:
> Powered by **FutureBuild OS** · Configured for **Small Housing BC**

Not "Built by FutureBuild," not "FutureBuild × SHBC partnership." The
word "configured" signals that BuildOS is the platform and SHBC's
content is the layer.

## Logo

A text-based SVG. The roofline mark uses **gable-green** (`#00FFA3`) for
the larger massing and **wood** (`#d6a778`) for the smaller secondary
massing — a subtle nod to the SHBC palette while staying inside the
FutureBuild visual system. Wordmark is white Inter; subtext "CONFIGURED
ON FUTUREBUILD OS" in muted slate-blue (`#7a8092`). See
`assets/shbc-logo.svg`.

## Iconography

- No emoji in any surface.
- Status pills use a single colored dot at the start of the pill, not an
  emoji or icon. Filled background tint at ~12% opacity, border at ~40%.
- Critical / Warning rows in tables and cards rely on the **left-edge
  3px accent border** rather than a full-width fill — keeps the dark
  surface dense without alarm fatigue.

## Surface elevation

Three layers, lowest to highest:
1. **`--bg-deep` (`#0A0B10`)** — page background, behind everything.
2. **`--bg-panel` (`#12131A`)** — header, sidebar, table headers.
3. **`--bg-surface` (`#161821`)** — cards, the main content surfaces.

Hover states bump to **`--bg-raised` (`#1a1c26`)**. Form fields and
"recessed" surfaces drop to **`--bg-input` (`#0f1117`)**.

## Glow accents

- The "today" line on Gantt charts has a subtle 8px green box-shadow —
  the only intentional glow in the design.
- The toast (`#flash`) gets a 12px green glow on its leading dot when
  surfacing action confirmations.
- Avoid neon glow elsewhere; restraint is what makes the accent read as
  intentional rather than decorative.
