# User Personas

**Date:** 2026-04-02
**Pipeline Stage:** 02 - User Research
**Status:** COMPLETE

---

## Persona 1: Mike "The Super" Martinez -- Site Superintendent

**Demographics:** Male, 42, Central Texas. 18 years in residential construction. Manages 3-5 active jobs. Reports to company owner.

**Tech Profile:** iPhone 15, uses text heavily, occasionally checks email. Comfortable with simple apps but hates complex software. Lost trust in Buildertrend after it crashed during a client walkthrough.

**Daily Workflow:**
- 5:30 AM: Drive to first site, check weather on phone
- 6:00 AM: Crew arrives. Mike assigns tasks verbally, checks if materials arrived
- 7:00-4:00 PM: Bounces between 3 sites. Coordinates subs by phone/text. Takes progress photos
- 4:00 PM: Back at truck, tries to update schedule (often skips this)
- 5:00 PM: Reviews tomorrow's plan mentally, sends texts to confirm sub arrivals

**Pain Points:**
1. Spends 2+ hours/day on phone coordinating subs -- "I am a full-time phone operator who occasionally builds houses"
2. Weather surprises ruin his week -- no proactive weather integration with schedule
3. Material delays cascade silently -- discovers lumber shortage at 6 AM when the crew is standing around
4. Cannot update software in the field -- too many clicks, needs both hands free
5. Boss asks "how are we doing on budget?" and he has no real-time answer

**Jobs to Be Done:**
- Know before 6 AM what today requires (weather, materials, sub confirmations)
- Get alerted when a material order is late BEFORE it impacts the schedule
- Update task progress with minimal effort (voice, photo, one-tap)
- See if he is on track for the target completion date without doing math

**Quote:** "If your app takes more than 30 seconds to do something, I will use a text message instead."

**FutureBuild Value:** DailyFocusAgent morning briefing, SubLiaisonAgent auto-confirmation, Flutter offline-first field app, voice-to-text daily logs.

---

## Persona 2: Sarah Chen -- Back-Office Administrator

**Demographics:** Female, 35, suburban Colorado. Office manager for a 15-person custom home builder. Handles AP/AR, payroll coordination, document filing, insurance certificates. Reports to company owner.

**Tech Profile:** Desktop (Windows), dual monitors. Proficient in QuickBooks, Excel, email. Uses Buildertrend reluctantly. Would prefer one system over four.

**Daily Workflow:**
- 8:00 AM: Process overnight emails -- sub invoices, material receipts, change order requests
- 9:00-11:00 AM: Enter invoices into QuickBooks, match to purchase orders, code to WBS phases
- 11:00 AM: Check insurance certificate expirations for next week's subs
- 1:00-3:00 PM: Prepare draw requests (AIA G702/G703), chase outstanding AR
- 3:00-5:00 PM: Coordinate with Mike on schedule updates, update Buildertrend (manual re-entry)

**Pain Points:**
1. Triple data entry: invoice goes into email -> QuickBooks -> Buildertrend
2. AIA billing preparation takes a full day per project per month
3. No real-time budget visibility -- discovers overruns after invoices are paid
4. Cert tracking is a spreadsheet nightmare -- one lapsed cert can shut down a job site
5. Draw request preparation requires manually pulling completion percentages from Mike (who guesses)

**Jobs to Be Done:**
- See all projects' financial health on one screen without switching tools
- Prepare AIA draw requests in 30 minutes instead of 4 hours
- Get alerted when a sub's insurance or license is expiring
- Auto-match invoices to purchase orders and WBS phases
- Track AR aging without maintaining a separate spreadsheet

**Quote:** "I need a dashboard, not a chat window. Show me the numbers."

**FutureBuild Value:** Corporate Financials dashboard, AIA billing automation, invoice extraction (legacy InvoiceServicer), certification alerts (TypeCertificationAlerts), AR aging (ARAgingSnapshot model).

---

## Persona 3: Tom Nakamura -- Company Owner / Builder

**Demographics:** Male, 55, Pacific Northwest. Owns a custom home building company with 12 employees. Builds 8-12 homes/year, $500K-$2M each. 30 years experience. Decisions are financial-first.

**Tech Profile:** MacBook Pro, iPhone. Uses QuickBooks, Excel, Buildertrend (pays for it, rarely uses it himself). Wants executive-level visibility without learning new tools.

**Weekly Workflow:**
- Monday AM: Review cash position across all projects
- Ongoing: Approves change orders, draw requests, large purchase orders
- Friday PM: Reviews schedule status, plans next week's priorities
- Monthly: Reviews P&L by project, prepares owner reporting

**Pain Points:**
1. No single view of company financial health across 8-12 projects
2. Relies on Sarah's spreadsheets for cash flow projections -- always 2 weeks stale
3. Cannot see if a project is profitable until it is finished
4. Equipment sits idle because no one tracks utilization across projects
5. Hiring/firing decisions based on gut feel, not workload projections

**Jobs to Be Done:**
- See total committed vs actual spend across all projects on one screen
- Know which projects are profitable and which are bleeding money RIGHT NOW
- Approve procurement orders with one click when agents recommend them
- Track equipment utilization and maintenance schedules
- Plan workforce needs based on upcoming project schedules

**Quote:** "I do not care how the sausage is made. Show me if we are making money or losing money."

**FutureBuild Value:** Organization-root Industrial Dark dashboard, CorporateBudget rollup, fleet asset utilization, Tribunal-governed autonomous procurement with owner approval threshold.

---

## Persona 4: Carlos Reyes -- Field Worker / Crew Lead

**Demographics:** Male, 28, South Texas. Framing crew lead. Reports to Mike. Works 6 AM - 3 PM. Limited English (bilingual Spanish/English). Manages 4-person crew.

**Tech Profile:** Android phone (budget model). Uses WhatsApp for personal communication. No experience with construction software. Strong visual/spatial intelligence.

**Daily Workflow:**
- 6:00 AM: Arrives at site, receives verbal instructions from Mike
- 6:00-3:00 PM: Executes framing tasks, manages crew, handles material issues
- Lunch: Checks phone for personal messages
- 3:00 PM: Reports to Mike verbally on what got done

**Pain Points:**
1. Never knows if tomorrow's materials will be on site
2. Cannot report progress digitally -- everything is verbal to Mike, who may forget
3. Safety documentation is paperwork he does not understand
4. Weather changes mid-day with no adjustment plan
5. Language barrier makes complex software impossible

**Jobs to Be Done:**
- See tomorrow's task list with photos/diagrams (not text-heavy)
- Report completion with a photo and one tap
- Get weather alerts in Spanish
- Check in/out for time tracking without paperwork
- Know what safety gear is required for today's tasks

**Quote:** "Show me a picture of what to build. I will build it."

**FutureBuild Value:** Flutter mobile app with bilingual support, photo-based progress reporting, geofenced time tracking, visual task cards with diagrams.

---

## Persona Access Matrix

| Feature Area | Mike (Super) | Sarah (Admin) | Tom (Owner) | Carlos (Field) |
|-------------|-------------|--------------|------------|---------------|
| Morning Briefing | PRIMARY | Views | Views | N/A |
| Schedule Dashboard | Updates | Views | Views | Task list only |
| Financial Dashboard | N/A | PRIMARY | PRIMARY | N/A |
| Procurement Alerts | Receives | Executes orders | Approves | N/A |
| Fleet Management | Requests equipment | Tracks assets | Utilization reports | N/A |
| HR / Time Tracking | Approves crew time | Processes payroll | Cost reports | Clocks in/out |
| Sub Coordination | PRIMARY | Backup | N/A | N/A |
| AIA Billing | Provides completion % | Prepares draws | Approves | N/A |
| Mobile App | Heavy use | Occasional | Light use | PRIMARY |
| Corporate Dashboard | N/A | Daily use | Daily use | N/A |
