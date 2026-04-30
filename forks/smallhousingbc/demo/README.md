# SHBC × BuildOS — Demo Fork

A self-contained, mock-data-driven UI demonstration of what a BuildOS fork tailored
for **Small Housing BC** and their network of multiplex builders could look like.

Built to be shown to Daniel Winer (SHBC Executive Lead) on the disco call as a
12-minute screencast or live walk-through.

---

## What this is

- A **single-page web app** (vanilla HTML/CSS/JS, no build step) that simulates
  one builder's morning inside the SHBC fork.
- All data is **mock JSON** loaded at runtime. Nothing talks to a real backend.
- Reuses ~90% of BuildOS's existing domain surface (Schedule, Budget,
  Procurement, Feed, HR/Certifications, Pipeline) and adds three SHBC-native
  surfaces: **Permit & Bylaw Checklist**, **Design Library**, **Builder
  Directory**.

## What this is NOT

- A production fork. There's no auth, no database, no real Brain integration.
- A homeowner-facing app (that's Wedge 1; deferred).
- A municipal dashboard (that's Wedge 3; deferred).

See `../07_buildos_fit_analysis.md` for the wedge framework.

---

## Run it

### Option 1 — local browser (easiest)

```
cd demo && python3 -m http.server 8765
# then open http://localhost:8765
```

(Direct `open index.html` won't work — Chrome blocks `fetch()` on `file://`.)

### Option 2 — Claude Preview (live in-session URL)

Claude Code's preview tool runs in a sandbox that **can't read files from
the user's home directory** — so the source under `forks/smallhousingbc/demo/`
needs to be mirrored to `/tmp/shbc-demo/` first:

```
./sync-to-tmp.sh
```

Then launch via `mcp__Claude_Preview__preview_start` with the
`shbc-demo` config in `.claude/launch.json` (already provided). The
preview opens at `http://localhost:4319/`.

Re-run `sync-to-tmp.sh` after every source edit.

### Option 3 — pin "today" for a reproducible demo

Append `?today=2026-04-30` to any URL to anchor the demo's clock —
useful when recording the screencast so date-relative copy ("in 4
days", "yesterday") stays consistent across takes.

---

## The demo's mock-data world

**SHBC Multiplex Cohort 2026.01** — a fictional but plausible network of four
SHBC-trained builder firms running 11 SSMUH projects across five BC
municipalities.

| Firm | Principal | HQ | Specialty |
|---|---|---|---|
| Aldergrove Build Co. | Priya Mehta | Burnaby | Sixplex new-builds |
| East Van Density Studio | Marc Tremblay | Vancouver | Fourplex / triplex conversions |
| Sea-to-Sky Workshop | Laila Brennan | Squamish | Laneway homes + ADUs |
| Cottonwood Carpentry | Devon Singh | Maple Ridge / Nelson | Heritage-overlay duplexes |

Each firm has 2–3 active projects with full schedules, budgets, procurement,
certifications, and per-municipality bylaw checklists. The demo defaults to
Marc Tremblay's view inside East Van Density Studio.

## Tour

The default opening is **Marc's Monday morning** — see [`DEMO_SCRIPT.md`](DEMO_SCRIPT.md)
for the eight-screen walkthrough.

## Files

```
demo/
├── README.md                 # this file
├── DEMO_SCRIPT.md            # 12-min walkthrough script
├── BRAND.md                  # color palette, typography, voice
├── index.html                # single-page entry
├── styles.css                # SHBC-themed CSS
├── app.js                    # demo router + view rendering
├── assets/
│   └── shbc-logo.svg         # text-based logo
└── data/
    ├── organizations.json    # SHBC root + 4 builder firms
    ├── users.json            # principals + crew
    ├── projects.json         # 11 SSMUH projects
    ├── schedules.json        # CPM tasks per project
    ├── budgets.json          # WBS line items
    ├── procurement.json      # ~80 items with critical/warning/ok statuses
    ├── invoices.json         # ~30 invoices
    ├── certifications.json   # crew certs (most active, a few expiring)
    ├── municipalities.json   # 5 BC munis with bylaw rules
    ├── bylaw_checklist.json  # per-project checklist instances
    ├── design_library.json   # CMHC + BC Standardized + SHBC Toolbox
    ├── leads.json            # homeowner inquiries (Toolbox hand-offs)
    └── feed_cards.json       # daily focus surface
```

The `../seed/` directory holds a SQL file that mirrors the same dataset for
seeding a real BuildOS instance later.

## SHBC-specific customizations vs. stock BuildOS

| Customization | Where in this demo |
|---|---|
| SHBC brand theme + co-branding | `styles.css`, `assets/shbc-logo.svg` |
| Default currency CAD | `app.js` `formatMoney()` |
| SSMUH project typology enum | `data/projects.json` `typology` field |
| Project templates per typology | `data/schedules.json` (per-typology task graphs) |
| Municipality picker with bylaw rules | `data/municipalities.json` + bylaw module |
| Permit & Bylaw Checklist module | `data/bylaw_checklist.json` |
| Design Library tab (Toolbox + CMHC + BC) | `data/design_library.json` |
| Builder Directory (cohort roster) | rendered from `data/organizations.json` |
| BC cost-data calibration | `data/procurement.json` cost amounts |
| Bill 25 compliance export | `app.js` `exportComplianceCSV()` |
| Toolbox-style homeowner intake hand-off | `data/leads.json` |

## Plans for life beyond the demo

This demo is the first step. The companion roadmap (Wedge 2 → Wedge 1 → Wedge 3)
is documented in `../07_buildos_fit_analysis.md`. The natural sequence:

1. Show this demo on the disco call to Daniel.
2. If positive, scope a 4-week production build (see proposal at top of
   the chat — this demo is the equivalent of "weeks 1-2 of the demo phase").
3. Use the production build as the anchor for a CMHC HSC bid (Wedge 1).
4. Add municipal procurement (Wedge 3) when a city champions it.
