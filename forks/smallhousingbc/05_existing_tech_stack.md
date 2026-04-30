# Existing Tech Stack & Digital Footprint

What SHBC actually has today, and where the gaps are. This frames "fork vs. integrate vs. replace" decisions.

---

## Public web properties

| Property | URL | What it is | Notes |
|---|---|---|---|
| Marketing site | smallhousingbc.org | WordPress site, public-facing brand | Primary org presence |
| Gentle Density Toolbox | toolbox.smallhousing.ca | Resource library / design catalogue | Funded by CMHC HSC grant |
| Sister/staging domain | smallhousing.ca | Resolves; appears to be a parallel/rebrand effort | Worth asking about |
| YouTube | youtube.com/@SmallHousingBC | Talks, panels, recorded webinars | Not heavily used |
| LinkedIn | linkedin.com/company/small-housing | Org page | Active |
| Facebook | facebook.com/SmallHousingBC | Lower activity | Legacy presence |

---

## Tech stack (as detected publicly)

The marketing site (smallhousingbc.org) runs on:

- **WordPress.org** (self-hosted)
- **PHP**
- **reCAPTCHA** (Google) for forms
- **Site5** as the hosting provider

This is a basic, low-budget CMS setup — appropriate for a small non-profit with a content-publishing workflow, not appropriate as a foundation for any kind of multi-user transactional product.

The Toolbox subdomain (toolbox.smallhousing.ca) appears to be a separate application (likely a custom WordPress build with a custom post-type / taxonomy structure for the resource library, or possibly a static-generated catalogue). Probably built or co-built with a contractor under the CMHC grant. Worth asking who built it.

---

## What they DO have digitally

- Content + resource library (Toolbox)
- Email list / newsletter (implied but not confirmed)
- Event registration (Eventbrite-style flows for the Multiplex Construction Training)
- Standard WordPress contact forms
- Brand assets, photo library, design catalogues

---

## What they DO NOT have (as far as the public web shows)

- A homeowner account / login system
- A project tracker for individual homeowner-developers
- A builder directory / marketplace
- A CRM (likely uses Mailchimp/Constant Contact + spreadsheets)
- A permit-tracking integration with any municipality
- A feasibility / pro-forma calculator
- A design configurator
- An API or data feed of their content
- Any kind of native mobile app
- Any AI/automation in the user-facing experience

---

## Adjacent platforms in the BC ecosystem (competitive / complementary landscape)

These are the platforms a SHBC fork would either compete with, complement, or have to interoperate with.

### BC Housing's DASH (acceleratedhousing.ca)
- Launched early 2025
- Funded by **$4M from CMHC's Housing Supply Challenge** (much larger than SHBC's grant from the same program)
- Aimed at standardizing and accelerating **3- to 6-storey wood-frame buildings** — the tier *above* SSMUH gentle density
- Combines standardized building components, digital coordination, prefabrication
- Public and free
- **Implication for SHBC fork:** DASH owns the mid-rise market. SHBC's natural lane is *below* DASH — the 1-to-6-unit gentle density typology that DASH explicitly doesn't address. Clear positioning gap.

### BC Builds (bcbuildshomes.ca)
- Provincial program for affordable middle-income rental housing
- More of a delivery program than a software platform
- **Implication:** Different scope; not a direct competitor. Could be a downstream user of SHBC's data.

### CMHC Housing Design Catalogue
- A federal catalogue of pre-reviewed designs
- Already republished in SHBC's Toolbox
- **Implication:** SHBC has a content-licensing relationship with CMHC; a fork could surface this catalogue as a configurator input.

### Standardized Housing Design (BC, 2024)
- Provincial program publishing 10 standardized designs as PDFs
- Static
- **Implication:** Same as CMHC — content input, not a competitor.

### Builder PM tools active in BC
- Buildertrend, CoConstruct, JobTread, Procore (commercial)
- These are the tools BuildOS is already up against
- **Implication:** A SHBC fork wouldn't displace these for general builders; it would be the platform of choice **specifically for the homeowner-developer + small-scale-multiplex segment** that SHBC owns the relationship with.

---

## Read on technical maturity

SHBC is a **content + advocacy org with light digital tooling**. They've successfully shipped one custom web product (the Toolbox) with grant funding. That's good — it means they understand product timelines, can manage a contractor build, and have a reference for what "shipping software" means. They are *not* a product organization with engineers on staff. Any fork conversation will require:

1. A clear ownership model — who maintains the fork over 3+ years?
2. Funding model that covers product engineering, not just initial build
3. A reasonable expectation that SHBC contributes domain content, brand, distribution, and customer relationships — but not engineering capacity

---

## What to verify on the call

- Who built the Toolbox? In-house contractor or agency?
- What's their analytics view on Toolbox usage? Daily users, return rate, popular resources?
- Do they have any view on Toolbox-to-action conversion? (Did anyone who used the Toolbox actually build something?)
- Is the Toolbox content under a creative-commons license, or restrictively licensed? (Matters for what a fork can ingest.)
- Do they have any partnership history with construction-tech vendors before this?
