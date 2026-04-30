# BuildOS × Small Housing BC — Fit Analysis

This is the strategic doc — what a "BuildOS fork for SHBC" could actually look like, the three plausible product wedges, and the recommended path.

---

## Recap: what BuildOS already is

From the FutureBuild.ai public site:
- FutureBuild OS — open-source operating system for residential builders. Project + business management, full source code access, self-host or cloud
- Connected by FB-Brain — proprietary AI layer
- AI capabilities: reads blueprints, parses invoices, verifies site photos
- Deterministic Critical Path scheduling (weather-aware SWIM model)
- Daily focus feed for the builder — what's at risk, what's blocked, what needs decisions
- Sister products: GableLBM (open-source ERP for lumber yards / LBM dealers) and LocalBlue (SaaS for trade contractors)

**Core thesis:** BuildOS is purpose-built for residential builders who need real PM software but won't tolerate enterprise complexity or pricing. SHBC's audience — homeowners doing one project of their lives, and small builders doing one to ten multiplexes a year — is the *exact* user profile that doesn't fit Procore or Buildertrend either.

---

## The three plausible product wedges

There are three audiences a fork could serve. Each is a real product. They are not mutually exclusive but they require different first-quarter focus.

### Wedge 1 — Homeowner-Developer App
**"The TurboTax of building a fourplex on your lot."**

- A consumer-grade web app for the BC homeowner who wants to add density
- Walks them from "what can I build?" through feasibility, design selection, permit prep, and builder matchmaking
- Pulls from SHBC's Toolbox content as the design library
- Pulls from machine-readable bylaw data (would have to be built or sourced)
- Light project tracking once construction starts
- **Builder hand-off:** when the homeowner picks a builder, the project record promotes into BuildOS proper for construction execution

**Pros:**
- Strongest brand alignment with SHBC's mission
- Defensible (nobody else is doing this in BC)
- Free / freemium → mass adoption
- Direct funnel into BuildOS proper (homeowner finishes feasibility → invites builder onto BuildOS)

**Cons:**
- Lowest direct revenue (homeowners won't pay much; freemium economics)
- Highest product complexity (consumer UX is hard; bylaw data is hard)
- Slowest path to revenue

**Best if:** SHBC and FutureBuild want to play a 3+ year game and use grant funding to subsidize the build.

---

### Wedge 2 — Multiplex Builder Edition
**"BuildOS, pre-configured for the BC small-scale multiplex builder."**

- A BuildOS fork (or a BuildOS configuration) tailored to the SSMUH project type — duplex, triplex, fourplex, sixplex, ADU
- Pre-loaded templates for the typical SSMUH project workflow
- Pre-loaded municipality-specific bylaw checklists (Vancouver, Burnaby, Squamish, etc.)
- Pre-loaded BC-specific cost data (and integration with GableLBM for materials)
- Distribution channel: bundled into SHBC's Multiplex Construction Training as "the way you'll run your multiplex projects"
- Co-branded "Powered by FutureBuild × Small Housing BC"

**Pros:**
- Fastest path to revenue (these builders already pay for software, just bad software)
- Lowest product risk — it's BuildOS, slightly customized, not a new product
- Distribution channel is real (Multiplex Construction Training has a captive audience)
- Co-brand legitimizes BuildOS in the BC market

**Cons:**
- Doesn't differentiate dramatically from "BuildOS for any small builder"
- Risk of being seen as just a marketing co-brand rather than a real fork
- SHBC's value-add to FutureBuild here is mostly distribution, not product

**Best if:** Both parties want a fast, low-risk first deliverable that proves the relationship works before investing in deeper integration.

---

### Wedge 3 — Municipal Gentle-Density Pipeline Dashboard
**"How a BC city tracks every gentle-density project in its pipeline."**

- A dashboard for municipal planning departments
- Ingests permit applications and tracks them through the SSMUH pipeline
- Reports on housing-target progress (tied to the Province's mandated reporting under Bill 25)
- Surfaces bottlenecks (which permit type stalls longest, which neighbourhoods get the most applications)
- Feeds back into SHBC's policy work — they get aggregated, anonymized data on what's working in each jurisdiction

**Pros:**
- Highest single-deal revenue (municipalities buy software via RFPs at $50K–$250K/year)
- Repeatable across 162 BC municipalities
- Aligned with the Province's Bill 25 reporting requirements
- SHBC's existing municipal relationships (Vancouver, Maple Ridge, Nelson, plus expanding) are a warm-intro channel

**Cons:**
- Long sales cycle (municipal procurement)
- Competes more directly with DASH and incumbent planning-software vendors
- Furthest from BuildOS's current product (BuildOS is a builder tool, not a govtech tool)
- Requires significant new product surface

**Best if:** There's a clear municipal champion willing to be the first paying customer, *and* the partnership is seen as a 5-year strategic play.

---

## Recommended path (opinion)

**Pursue Wedge 2 first as a fast win, with Wedge 1 as the strategic play funded by a CMHC HSC bid. Defer Wedge 3 until you have a flagship municipal customer asking for it.**

Rationale:
1. **Wedge 2 generates revenue inside 6 months.** It's the lowest-risk demonstration that the FutureBuild × SHBC relationship works. The builders SHBC trains pay for SaaS; SHBC gets channel revenue or marketing-equity; FutureBuild gets a vertical-specific co-brand and a credible BC anchor.
2. **Wedge 1 is the right CMHC HSC pitch.** It's open-source, scalable across Canada, addresses a documented housing-supply barrier, and would be co-led by a non-profit (SHBC) — exactly the partnership model CMHC funds. Use the 12-18 month grant timeline to plan and build.
3. **Wedge 3 is real but premature.** Don't lead with municipal sales; let municipal customers come knocking after they've seen Wedges 1 and 2 in market. DASH owns the mid-rise narrative right now; a SHBC-aligned platform owning the lot-scale narrative needs to first be visible at the lot scale before cities will pay.

---

## What "fork" probably means in practice

The word "fork" is doing a lot of work. Three realistic interpretations:

| Interpretation | What it means technically | What it means commercially |
|---|---|---|
| **White-label BuildOS** | Same codebase, custom theme/brand for SHBC | Reseller-style relationship, fastest to ship |
| **BuildOS configuration / module** | Same codebase, SHBC-specific templates, integrations, content packs | Co-development, FutureBuild owns IP, SHBC contributes content |
| **True open-source fork** | A separate repo, downstream from BuildOS | Independent product over time, governance question, slower |

**Recommendation:** Pitch a **configuration/module** for Wedges 2 and 3 (FutureBuild keeps IP and product velocity), and a **true open-source fork** only if Wedge 1's CMHC bid requires public-IP outputs (which CMHC HSC often does). Be ready to discuss all three on the call.

---

## What FutureBuild brings to the table

- An existing, working product (BuildOS, GableLBM, LocalBlue)
- AI document parsing, scheduling, and project intelligence
- Open-source DNA — culturally aligned with SHBC's public-good mission
- Builder-side credibility (Grant Petkau's Kelbrook Construction)
- A credible product team that can ship

## What SHBC brings to the table

- Brand — they are *the* gentle-density voice in BC
- Audience — homeowners and builders who already trust them
- Distribution — Multiplex Construction Training, Toolbox traffic, newsletter, events
- Funder relationships — CMHC, REFBC, Vancity, BC Housing, Province of BC
- Policy expertise — they understand SSMUH, Bill 44, Bill 25 better than anyone
- Municipal relationships — Vancouver, Maple Ridge, Nelson, and growing
- Content — the Toolbox is a real asset, defensible IP

---

## The 18-month roadmap (conceptual, for the call)

**Months 0-3:** Joint scoping, MOU, agree on Wedge 2 first deliverable
**Months 3-9:** Ship Multiplex Builder Edition (Wedge 2). Co-launch at the next Multiplex Construction Training cohort. Begin drafting CMHC HSC bid for Wedge 1.
**Months 6-12:** Submit CMHC HSC bid (timing-dependent on round schedule)
**Months 9-18:** If grant won, build Wedge 1 (Homeowner-Developer App). Use Wedge 2 builder customers as live data for Wedge 1 builder marketplace.
**Month 18+:** Wedge 3 (municipal) opens up if a city champions it.

---

## Red flags / risks to surface on the call

1. **Timeline mismatch with Bill 25 deadline (June 30, 2026)** — if SHBC expects a tool to ship in support of the bylaw deadline, that's already too aggressive. Set expectations honestly.
2. **DASH overlap** — make sure SHBC sees DASH as adjacent, not competitive, to the proposed fork. If they see it as competitive, the positioning conversation is the *first* conversation.
3. **Open-source IP and revenue model** — BuildOS is open source. SHBC needs to understand that the fork code itself can't be a moat; the moat is the integration with their content, brand, and distribution.
4. **Smallworks conflict-of-interest** — Akua Schatz and Jake Fry are both Smallworks principals. If a builder marketplace launches inside the SHBC fork, every other BC builder will scrutinize whether Smallworks gets preferential surfacing. Worth a governance carve-out from day one.
5. **Capacity** — SHBC is small. They cannot dedicate a product manager full-time to this. FutureBuild needs to provide its own product capacity and use SHBC primarily as a content / domain partner.
