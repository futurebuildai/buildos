# Member & Operator Workflows

This doc maps the day-to-day workflows of the people SHBC serves — the homeowner-developers and the small-scale multiplex builders. Knowing these workflows is the difference between pitching "a software platform" and pitching "a tool that solves *this specific bottleneck* in *this specific job*."

---

## A. The homeowner-developer journey

The homeowner-developer is the homeowner who decides to add density to their existing lot — a laneway home behind their bungalow, a triplex replacing their duplex, an ADU above a detached garage. Under Bill 44 / Bill 25, this is now legal on virtually every single-family lot in BC.

### Step-by-step (synthesized from SHBC's For Homeowners page and FAQ)

1. **Curiosity / "is this even possible on my lot?"**
   - Today: read SHBC's FAQ, browse the Toolbox, attend a workshop
   - Pain: information is fragmented across SHBC, the municipal website, the BC government's SSMUH manual, builder marketing sites
   - Tool gap: nobody offers a single "enter your address, see what you can build" answer for BC

2. **Zoning + bylaw check**
   - Today: navigate the local municipality's zoning map, read the bylaw PDF, hope you understood it
   - Pain: 162 BC municipalities, all updating their bylaws on different timelines, all with their own quirks despite Bill 44 trying to standardize. SHBC's Bylaw Guide is the closest to a Rosetta Stone.
   - Tool gap: machine-readable bylaw database keyed to lot characteristics

3. **Feasibility analysis**
   - Today: SHBC's "development feasibility analysis" (paid consult) or hire an architect
   - Pain: cost of feasibility study can be $5–15K before you know if the project is viable; many homeowners abandon at this stage
   - Tool gap: parametric pro-forma keyed to lot, zoning, design library, current BC construction costs

4. **Pre-design review**
   - Today: SHBC's "Pre-design home review program"
   - Pain: human-delivered, doesn't scale, requires booking
   - Tool gap: AI-assisted pre-screen on basic massing/setback/site coverage compliance

5. **Design selection / customization**
   - Today: hire a designer/architect from scratch, or use BC's standardized designs (10 templates), or browse the Toolbox's pre-reviewed designs
   - Pain: BC's standardized designs are PDFs, not interactive. Toolbox is photos + plans, not configurable.
   - Tool gap: a configurator that lets a homeowner tweak a pre-approved design and export plans ready for permit

6. **Permitting**
   - Today: each municipality has its own permit portal, document requirements, timelines
   - Pain: **the #1 documented bottleneck post-Bill 44** — permitting delays persist despite zoning reforms
   - Tool gap: cross-municipality permit-readiness checker, document-completeness AI

7. **Builder selection**
   - Today: word of mouth, referral lists, hope
   - Pain: there's no curated builder network; BC Housing licensing tells you who is licensed, not who is competent for multiplexes
   - Tool gap: builder marketplace, ideally vetted by SHBC's training program

8. **Construction**
   - Today: builder runs it on whatever PM tools they use (often spreadsheets)
   - Pain: homeowner has no visibility into project status, costs, schedule
   - Tool gap: this is exactly what BuildOS does today for the builder side. The homeowner-facing layer is the gap.

9. **Tenanting / strata / move-in**
   - Today: owner figures it out
   - Pain: ownership structures (strata, co-ownership, rental), insurance, financing all need new approaches for multi-unit on what used to be a single-family lot
   - Tool gap: legal-template library + checklists

### The cumulative pain

A typical homeowner-developer in BC right now is staring at **18–36 months** between "I'd like to add density" and "I have tenants." Most quit. SHBC's mission is to lower that drop-off rate. **A platform that compresses this journey is exactly what they're trying to build by hand right now via consults and toolkits.**

---

## B. The small-scale multiplex builder workflow

This is the persona SHBC's **Multiplex Construction Training** is targeting. They overlap heavily with FutureBuild's existing customer base.

### Their day-to-day pain points (pulled from SHBC FAQ + BC SSMUH coverage)

- **Permitting bottlenecks** — even with Bill 44, permit times haven't dropped meaningfully. Builders eat carrying costs.
- **Bylaw complexity** — every municipality is interpreting Bill 44 differently. A builder doing a fourplex in Maple Ridge cannot copy/paste the workflow they used in Vancouver.
- **Financing for small-scale projects** — banks underwrite small-multiplex builds awkwardly (too small for commercial real estate desks, too unusual for residential mortgage desks). Vancity is one of the few lenders actively serving this segment.
- **Underground electrical service** — some municipalities require underground utility connections, which can cost 10× overhead and add 10–18 months to schedule.
- **Site-specific constraints** — many lots have physical/regulatory quirks (slope, easements, setbacks, heritage overlays) that aren't visible until you're deep into design.
- **Cost misconceptions** — "small ≠ cheap" is a constant builder education problem with homeowner clients. SSMUH projects are *complex* per square foot.
- **Building code edge cases** — small dwellings with loft bedrooms, ladder access, narrow doorways, no secondary exits routinely fail BC Building Code Part 9.
- **Community engagement** — neighbour pushback, even when the project is by-right under SSMUH

### The builder operating reality

Most small-scale builders in BC operate with:
- Spreadsheets for budgets
- Email + text for communication
- A whiteboard or paper schedule
- A handful of subcontractor relationships built over years
- No formal project management software

This is **precisely the BuildOS user profile** — a residential builder who needs an operating system but won't tolerate enterprise complexity or pricing.

---

## C. The municipal staff workflow (lower-priority for fork conversation)

Municipal planners post-Bill 44 / Bill 25 are scrambling to:
- Update their zoning bylaws by June 30, 2026
- Process the inbound wave of SSMUH applications
- Track which lots in the city are gentle-density-eligible
- Coordinate utility upgrades (sewer, water, electrical) for areas of growth
- Report to the Province on housing-target progress

A municipal-facing dashboard could be a real product, but it competes more with planning consultancies and BC Housing's own DASH platform (acceleratedhousing.ca) than with anything BuildOS does today. Probably an *eventual* wedge, not a *first* wedge.

---

## D. Where the workflows converge

The wedge that connects all three personas is the **project record itself** — one shared object that the homeowner, the builder, and the municipality all interact with.

```
Homeowner (initiates) → Project record → Builder (executes) → Municipality (permits + tracks)
                              │
                              ├── Pulls from Toolbox design library
                              ├── References municipal bylaw rules
                              ├── Generates permit-ready documents
                              ├── Tracks construction milestones
                              └── Reports completion + occupancy
```

A BuildOS fork that owns this object is well-positioned for SHBC because (a) SHBC has the trust of all three personas and (b) nobody else in BC is currently trying to be the system of record.
