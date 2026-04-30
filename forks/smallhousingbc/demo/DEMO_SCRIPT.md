# Demo Script — SHBC × BuildOS

A 12-minute walkthrough script. Read it once before recording or showing
live. Follows the **eight-screen** tour from the proposal, opening on
**Marc Tremblay's Monday morning** (East Van Density Studio).

---

## Open with one sentence

> "What you're about to see is a configured BuildOS instance pre-loaded with
> a fictional but plausible SHBC Multiplex Cohort — four builder firms,
> eleven projects across five BC municipalities, all the data you'd expect
> a small-multiplex builder to wrestle with on a typical Monday."

---

## Screen 1 — Daily Focus Feed (~90 sec)

**URL:** `#/feed`

**What's on screen:** Three cards sorted critical → urgent → normal:
1. 🔴 **Commercial Drive Fourplex** — City of Vancouver heritage character review response due Wednesday
2. 🟠 **Hastings-Sunrise Triplex (Lead)** — New Toolbox lead from Sarah Chen, lot 2231 East Pender, design picked: BC Standardized #BCS-FX-04
3. 🟡 **Buchanan Sixplex (Aldergrove)** — Hydro underground service slipping; you're tagged on the affected MEP coordination

**Talk track:** "This is what Marc sees first thing Monday — same shape as
BuildOS today, but the cards know about the SSMUH world. Heritage character
reviews are a Vancouver-specific bottleneck. The lead in card 2 came from
SHBC's Toolbox via the same A2A webhook receiver that Sprint 4 shipped. The
sixplex card is cross-org — Marc subscribes to MEP coordination on a peer
firm's project because they share an electrician."

**Move on:** click the first card → drills into Commercial Drive Fourplex.

---

## Screen 2 — Project Portfolio (~60 sec)

**URL:** `#/projects`

**What's on screen:** Marc's three projects + a faded sidebar showing the
other cohort firms' portfolios. Filter chips for typology (Fourplex,
Triplex, etc.) and municipality.

**Talk track:** "Eleven projects across the cohort. Filter by typology to
see all the sixplex builds, or by municipality to see who's working in
Squamish this quarter. The cohort visibility is opt-in — Marc's only seeing
what other firms have shared with him under their cohort agreement."

---

## Screen 3 — Project Detail: Commercial Drive Fourplex (~3 min)

**URL:** `#/projects/proj-commercial-drive-fourplex`

Tab strip: **Overview · Schedule · Budget · Procurement · Bylaw · Design · Crew**

### 3a. Schedule (Gantt) — 45 sec

CPM Gantt with weather-adjusted spans. Critical path in red. Foundation tasks
slipped 9 days because the SWIM weather model expanded the rough-grading
buffer for an early March cold snap. "This is BuildOS's deterministic
physics engine — same code as the production system. Just configured for
BC weather and SSMUH typology."

### 3b. Budget — 30 sec

$1.8M CAD across WBS divisions. Note an invoice from BC Hydro for service
upgrade ($42,400 CAD) — a real BC pain point. "Composite Currency
Pattern — every line item is `amount_cents` paired with `currency_code`.
Cross-currency math is forbidden at the SQL layer. Slips become real
numbers, not approximations."

### 3c. Procurement — 30 sec

Sorted critical → warning → ok → ordered. Framing lumber package is
**CRITICAL** (must-order date passed yesterday). Windows package is
**WARNING** (must-order in 4 days). "The ProcurementCheckWorker we shipped
in Phase A3.5 ran overnight — it just flipped the lumber package from
WARNING to CRITICAL based on its `must_order_date`. The agent doesn't
remember to do this — the SQL does."

### 3d. Bylaw & Permit Checklist (NEW — 75 sec) ⭐

This is the SHBC-native surface. For Commercial Drive Fourplex on Vancouver
R1-1 land:

- ☑ Setback compliance (Bill 44 minimum 1.2m)
- ☑ Site coverage ≤ 50%
- ☑ Heritage Character Review submitted (waiting on Wednesday response)
- ☑ Tree retention bylaw compliance — arborist letter on file
- ☑ BC Building Code Part 9 secondary egress
- ☑ Bill 25 housing-target metadata auto-attached
- ☐ Ground Floor Adaptable Suite (Vancouver-specific) — design review needed

"This is **SHBC's IP made executable.** The data model is a
`bylaw_checklist_item` table joined to `municipality_id`; the value is
the curated content, which SHBC has spent a decade producing. No other PM
tool has it because no other PM tool has SHBC's bylaw guide."

### 3e. Design Library (NEW — 45 sec) ⭐

Tabs: SHBC Toolbox · CMHC HDC 2025 · BC Standardized 2024. Filtered by
fourplex. Each design card → "Start a project from this design."

"The integration is a curated data feed — links out to the live Toolbox
content. In production this could be a real ingestion pipeline. Today, it
demonstrates that the homeowner-developer journey doesn't end on the
Toolbox; it starts there and lands here."

---

## Screen 4 — Permit-Schedule Coupling (~30 sec)

Click "Mark heritage review approved" on the Bylaw tab. Watch the schedule
recompute live: framing tasks slide forward by 4 weeks, the daily feed
gets a new card "Schedule recovered 4 weeks after permit clearance."

"This is the deterministic CPM engine doing what it does — but framed
in SHBC terms. A permit decision is a state transition; the schedule
follows."

---

## Screen 5 — Builder Directory (~60 sec)

**URL:** `#/directory`

Four cohort firms with: certifications, training cohort badge, specialty,
**capacity** (% allocated), and a filter "Available for sub-trade work in
the next 60 days."

"This is the **defensible distribution channel**. SHBC owns the network;
the platform surfaces it. Today: four firms. After Cohort 2026.02 ships
in the fall: a dozen. After three years of training cohorts: fifty."

---

## Screen 6 — Crew & Certifications (~30 sec)

**URL:** `#/projects/proj-commercial-drive-fourplex` → Crew tab

Maria Hernandez (lead carpenter): BC Housing license expires in 47 days —
already surfaces as a feed card. Plus OSHA 10, fall protection, asbestos
awareness — the typical small-multiplex compliance load.

"Same `CertificationAlertsArgs` job that Sprint 0 wired up — it just runs
nightly and emits feed cards for everything within 60 days of expiry."

---

## Screen 7 — Pipeline / Toolbox Lead Hand-off (~60 sec)

**URL:** `#/pipeline`

Sarah Chen, lot 2231 East Pender, came in via the Toolbox. She picked
BC Standardized #BCS-FX-04 (a fourplex). The lead is a `Prospect` in
`LeadCaptured` stage. East Van Density Studio is the matched builder
because of typology + municipality alignment.

Click → "Schedule pre-design review" → moves to `PreDesignBooked` stage.
Eighteen months from now this becomes a real project record; today it's
a hand-off.

"The matching logic is simple in the demo — typology × municipality —
but the data model accommodates Smallworks-COI-safe scoring (see the
risk doc). The point is: SHBC owns the inbound; the platform routes it."

---

## Screen 8 — Compliance Export (Bill 25 hint) (~30 sec)

**URL:** `#/compliance`

A button: **Export housing-target report**. Generates a CSV with project
counts by typology, by municipality, by month — exactly what cities need
to file under Bill 25.

"This is the seed of Wedge 3 — the municipal dashboard — without
committing to it. If a city sees this CSV and asks 'can we just consume
this directly?', that's the conversation we want."

---

## Close (~30 sec)

> "Everything you saw is BuildOS, configured. The schedule engine, the
> procurement worker, the feed, the certifications, the A2A receiver — all
> already shipped. The three new things — bylaw checklist, design library,
> builder directory — are content modules SHBC owns and can grow over
> time. The fork doesn't compete with DASH; DASH owns mid-rise wood-frame.
> This owns the lot-scale gentle-density typology the entire province
> just legalized."

---

## Demo URLs cheat-sheet

| Screen | Hash route |
|---|---|
| Daily Focus Feed | `#/feed` |
| Project Portfolio | `#/projects` |
| Project Detail | `#/projects/{id}` |
| Bylaw tab | `#/projects/{id}/bylaw` |
| Design tab | `#/projects/{id}/design` |
| Crew tab | `#/projects/{id}/crew` |
| Builder Directory | `#/directory` |
| Pipeline | `#/pipeline` |
| Compliance | `#/compliance` |

## Speaker tips

- **Don't apologize for mock data.** The story is "this is what your
  cohort's Mondays look like" — make it feel inevitable.
- **Don't promise dates.** The proposal scopes a 4-week production build;
  the disco call is for understanding scope, not committing.
- **Lead with the bylaw checklist.** It's the most differentiated surface
  and the one Daniel will recognize as "things only SHBC could provide."
- **Anticipate the Smallworks question.** "Will Smallworks get
  preferential surfacing?" — answer: "No. The directory is sorted by
  available capacity and rotational ordering. We can write that into the
  governance MOU from day one."
