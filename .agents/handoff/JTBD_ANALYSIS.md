# Jobs to Be Done Analysis

**Date:** 2026-04-02
**Pipeline Stage:** 02 - User Research
**Status:** COMPLETE

---

## Core JTBD Framework

### Job 1: "Help me know what to do today before I get to the job site"
**Persona:** Mike (Superintendent)
**Context:** 5:30 AM, driving to the first of 3 sites. Needs to make decisions about crew allocation, material readiness, and weather impact.
**Current Solution:** Mental checklist, weather app, text messages to subs at 5 AM
**Desired Outcome:** A single briefing that synthesizes weather, schedule, material status, and sub confirmations
**Legacy Mapping:** DailyFocusAgent (reference-vault/futurebuild-os/internal/agents/daily_focus.go) -- already generates morning briefings via AI, but delivery is email-only. Needs push notification via Flutter app.
**Success Metric:** Briefing consumed before 6 AM, zero surprise material shortages

### Job 2: "Help me prevent material delays from killing my schedule"
**Persona:** Mike (Superintendent), Sarah (Admin)
**Context:** A $3,000 window order with 12-week lead time was ordered 2 weeks late, causing a 14-day schedule slip on the critical path.
**Current Solution:** Manual spreadsheet tracking, memory, occasional vendor phone calls
**Desired Outcome:** Automatic monitoring of lead times with proactive alerts before the must-order date
**Legacy Mapping:** ProcurementAgent (reference-vault/futurebuild-os/internal/agents/procurement.go) -- calculates MustOrderDate = NeedByDate - leadTime - weatherBuffer. Already has CRITICAL/WARNING/OK status transitions and notification dampening.
**Success Metric:** Zero critical-path delays caused by late material orders

### Job 3: "Help me see if my company is making money across all projects"
**Persona:** Tom (Owner), Sarah (Admin)
**Context:** Monthly P&L review. Tom has 10 active projects. Currently relies on Sarah's Excel rollup that takes 2 days to prepare and is stale by the time he sees it.
**Current Solution:** QuickBooks reports + manual Excel consolidation across projects
**Desired Outcome:** Real-time dashboard showing estimated vs committed vs actual spend across all projects, with drill-down to WBS phase level
**Legacy Mapping:** CorporateFinancialsServicer (reference-vault/futurebuild-os/internal/service/interfaces.go) -- RollupCorporateBudget, GetCorporateBudget, CalculateARAging already defined. CorporateBudget model uses int64 cents. TypeCorporateRollup cron task exists.
**Success Metric:** Financial health visible in <5 seconds, data no older than 24 hours

### Job 4: "Help me prepare AIA draw requests without losing a full day"
**Persona:** Sarah (Admin)
**Context:** Monthly billing cycle. Sarah must prepare G702/G703 forms for each project, pulling completion percentages from the superintendent (who estimates), matching invoices to budget categories, calculating retention.
**Current Solution:** Excel template + manual data entry + phone call to Mike
**Desired Outcome:** Auto-populated AIA forms from CPM task completion percentages and budget actuals
**Legacy Mapping:** BudgetServicer (reference-vault/futurebuild-os/internal/service/interfaces.go) -- GetBudgetBreakdown returns PROJECT_BUDGETS by WBS phase. ScheduleServicer.GetProjectSchedule provides completion data.
**Success Metric:** AIA preparation time < 30 minutes per project

### Job 5: "Help me coordinate subs without being a full-time phone operator"
**Persona:** Mike (Superintendent)
**Context:** Mike calls 5-8 subs per day to confirm arrival, check status, and resolve scheduling conflicts. Each call is 3-10 minutes. Many go to voicemail.
**Current Solution:** Phone calls, text messages, voicemail
**Desired Outcome:** Automated confirmation requests with intelligent response parsing
**Legacy Mapping:** SubLiaisonAgent (reference-vault/futurebuild-os/internal/agents/sub_liaison.go) -- ScanAndNotify sends SMS confirmation requests for tasks starting within 72h. HandleInboundMessage parses responses for percentage, delay indicators, and image URLs.
**Success Metric:** 80% of sub confirmations handled without superintendent intervention

### Job 6: "Help me track my equipment and avoid scheduling conflicts"
**Persona:** Tom (Owner), Mike (Superintendent)
**Context:** Tom owns 3 excavators, 2 compactors, and a crane. Equipment gets double-booked across projects. Maintenance is tracked on a whiteboard.
**Current Solution:** Whiteboard in office, verbal coordination, maintenance memory
**Desired Outcome:** Digital fleet management with project allocation, availability checking, and maintenance scheduling
**Legacy Mapping:** FleetServicer (reference-vault/futurebuild-os/internal/service/interfaces.go) -- CreateFleetAsset, AllocateEquipment, CheckEquipmentAvailability, LogMaintenance, GetUpcomingMaintenance. EquipmentValidator in physics engine validates equipment constraints for WBS 7.x tasks.
**Success Metric:** Zero equipment double-bookings, maintenance compliance >95%

### Job 7: "Help me keep my certifications and insurance current"
**Persona:** Sarah (Admin)
**Context:** A sub shows up to frame and his contractor license expired 3 days ago. Work stops, the schedule slips, and Sarah gets blamed.
**Current Solution:** Excel spreadsheet reviewed weekly (when she remembers)
**Desired Outcome:** Automated tracking with proactive alerts 30/14/7 days before expiration
**Legacy Mapping:** EmployeeServicer.GetExpiringCertifications (reference-vault/futurebuild-os/internal/service/interfaces.go), TypeCertificationAlerts cron task.
**Success Metric:** Zero work stoppages due to expired certifications

### Job 8: "Help me report progress without typing"
**Persona:** Carlos (Field Worker)
**Context:** End of shift. Carlos completed 60% of second-floor wall framing. His hands are dirty, he is tired, and he does not want to navigate a complex app.
**Current Solution:** Tells Mike verbally; Mike may or may not update the system
**Desired Outcome:** Take a photo, tap "60%", done
**Legacy Mapping:** SubLiaisonAgent.HandleInboundMessage parses percentage from SMS. TaskProgress model tracks reported_by, percent_complete, notes.
**Success Metric:** < 30 seconds per progress update, < 1 day of training

---

## JTBD Priority Matrix

| Job | Frequency | Impact | Effort | Priority |
|-----|-----------|--------|--------|----------|
| J1: Morning briefing | Daily | HIGH -- prevents reactive firefighting | LOW -- DailyFocusAgent exists | P0 |
| J2: Procurement alerts | Continuous | CRITICAL -- prevents schedule slips | LOW -- ProcurementAgent exists | P0 |
| J3: Financial dashboard | Daily/Weekly | HIGH -- business survival visibility | MEDIUM -- models exist, UI needed | P0 |
| J4: AIA billing | Monthly | HIGH -- cash flow impact | MEDIUM -- budget data exists, PDF gen needed | P1 |
| J5: Sub coordination | Daily | HIGH -- time savings for super | LOW -- SubLiaisonAgent exists | P0 |
| J6: Fleet management | Weekly | MEDIUM -- asset utilization | MEDIUM -- FleetServicer exists | P1 |
| J7: Cert tracking | Ongoing | HIGH -- compliance risk | LOW -- cron task exists | P1 |
| J8: Field progress | Daily | MEDIUM -- data quality | HIGH -- Flutter app build required | P1 |
