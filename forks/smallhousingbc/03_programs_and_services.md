# Programs & Services

SHBC operates a portfolio of programs aimed at three audiences: homeowners, municipalities, and builders. Understanding what they actually deliver day-to-day is the foundation for any conversation about software fit.

---

## 1. For Homeowners — direct enablement

This is the most operationally relevant program for a BuildOS fit conversation.

**Stated services:**
- Small-scale development workshops
- Pre-design home review program
- Development feasibility analysis
- Step-by-step guidance "from concept to construction to move-in"
- Connections to lenders, funding options, financial resources
- Connections to trusted professionals (architects, designers, builders)

**The implied workflow they're trying to support:**
1. Homeowner curiosity → "can I add density to my lot?"
2. Zoning + feasibility check
3. Pre-design review (does this make sense financially and physically?)
4. Design + permitting
5. Builder selection
6. Construction
7. Tenanting / strata setup / move-in

**The gap:** SHBC offers *advice and connections* at each step but doesn't operate a software platform that walks the homeowner through it. Today this is human-delivered (workshops, 1:1 consults). It does not scale.

---

## 2. For Communities — municipal advisory

SHBC's policy/planning shop. Not a software opportunity directly, but it's the function that drives their funder relationships and political access.

**Services:**
- Bylaw review and policy advice
- Industry and public outreach
- Pilot programs (Maple Ridge laneway lookbook, Nelson laneway how-to, Vancouver permanently affordable homeownership feasibility)
- Publishes the **Gentle Density Housing Bylaw Guide (2025)** as a downloadable resource

**Active municipal partners (historic + likely current):**
- City of Vancouver
- City of Maple Ridge
- City of Nelson
- Likely many more post-Bill 44 and Bill 25 (Squamish appears in their training-event roster)

**Why it matters for BuildOS:** if a fork ever serves a municipal customer (a permit-tracker dashboard for a city's gentle-density pipeline, for example), SHBC's existing municipal relationships are the warm-intro channel.

---

## 3. Multiplex Construction Training — builder enablement

A relatively new, hands-on training program for **builders, contractors, and planners** who need to deliver multiplex projects under the new SSMUH legislation.

**What's known:**
- Recent / upcoming sessions: **Vancouver/Burnaby** and **Sea-to-Sky / Squamish**
- Marketed as "real solutions for BC builders and planners facing rising costs, zoning shifts, and pressure to deliver"
- Implies a network of small-scale builders SHBC is actively touching and certifying (informally)

**Why this is the highest-leverage angle for a BuildOS fork:**
- These trainees are the *exact* customer profile for BuildOS today (small residential builders)
- SHBC could plausibly bundle "BuildOS for Multiplex Builders" into the training program — software adoption built into the certification path
- Creates a defensible distribution channel that no competing PM tool has

---

## 4. Gentle Density Toolbox — content + resource library

The closest thing SHBC has to a digital product today. Lives at **toolbox.smallhousing.ca**.

**What's in it:**
- Photo collections of gentle density designs (duplexes, triplexes, multiplexes) from across BC
- Pre-reviewed design library (row-houses, fourplexes, sixplexes, ADUs)
- Sample home designs (Richmond, Mosaic, etc.)
- Standardized "building blocks" designs (mix-and-match, customizable, stack up to 3 storeys)
- The CMHC Housing Design Catalogue (2025) republished
- BC Standardized Designs (2024)
- Bylaw guides and policy resources
- Filterable by country/state/province (Canada, Vancouver, etc.)
- Filterable by audience (local governments, planners, elected officials, advocates)

**Tech read:**
- Lives on a separate subdomain — likely a custom-built CMS / catalog application, not the same WordPress site as the marketing front-end
- Funded in part by the **CMHC Housing Supply Challenge** "Getting Started" grant SHBC won in November 2021
- "Webtool to host resources and the growing knowledge network of practitioners and researchers" was the explicit goal of the CMHC funding

**The strategic gap:** Toolbox is a **content site**, not a workflow tool. A homeowner can browse 50 fourplex designs there but cannot:
- Run a feasibility check on their own lot
- Track their project from feasibility → permit → construction
- Connect to a vetted builder
- Get a checklist of what to do next month

That's the gap a BuildOS fork could fill.

---

## 5. Accelerator Program

A page exists at `smallhousingbc.org/accelerator-program/` but the content didn't render in search snippets (JS-required). Likely refers to a homeowner-developer accelerator (cohort-based program walking participants through a small-housing project). Confirm on the call.

---

## 6. Events

Active events page (`/events/`). Recent topical events tied to the Multiplex Construction Training plus the broader Build Small Live Large Summit speaker circuit.

---

## What SHBC does NOT do (as far as the public web shows)

- Does not directly build housing (Smallworks does that — separate corporate entity)
- Does not operate as a property manager or operator of housing assets
- Does not appear to run a member-fee program (no "join SHBC for $X/year" page surfaced)
- Does not operate any kind of CRM, project tracker, or builder-network platform
- Does not offer financing or escrow services
- Does not certify builders formally (that's BC Housing's licensing role)

---

## Implication for a BuildOS fork

The cleanest product wedge is **the bridge between the Toolbox (content) and the Multiplex Training (builder network)**: a homeowner-developer + small-builder workflow tool that uses the Toolbox's pre-reviewed designs as inputs and the trained builders as the supply side. Detailed in `07_buildos_fit_analysis.md`.
