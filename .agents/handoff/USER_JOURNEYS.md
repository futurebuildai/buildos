# User Journeys

**Date:** 2026-04-02
**Pipeline Stage:** 02 - User Research
**Status:** COMPLETE

---

## Journey 1: Morning Briefing Consumption (Mike, Superintendent)

**Trigger:** 5:30 AM alarm. Mike picks up phone.

| Step | Action | System | Touchpoint |
|------|--------|--------|-----------|
| 1 | Mike sees push notification: "3 projects briefing ready" | DailyFocusAgent ran at 06:00 UTC (midnight Central) | Flutter app notification |
| 2 | Opens app, sees prioritized card stack | Feed cards sorted by priority (critical > urgent > normal) | Flutter feed view |
| 3 | Card 1: "WEATHER ALERT: Rain >60% chance at Sunrise Villa. Cover lumber, delay exterior paint." | SWIM v2 + Tomorrow.io hourly forecast | Weather alert card |
| 4 | Card 2: "Windows order CRITICAL -- must order by April 5" | ProcurementAgent alert (MustOrderDate passed warning threshold) | Procurement card with "Order Now" action |
| 5 | Mike taps "Order Now" -> Tribunal evaluates -> auto-creates PO draft | Tribunal ConsensusEngine reviews, generates PO if approved | Action card with approval flow |
| 6 | Card 3: "Framing sub unconfirmed for Oak Ridge tomorrow" | SubLiaisonAgent sent SMS 48h ago, no response | Sub confirmation card with "Call Sub" action |
| 7 | Mike taps "Call Sub" -> phone dialer opens with contact | DirectoryService.GetContactForPhase lookup | Native phone integration |
| 8 | Mike dismisses remaining cards. Total time: 4 minutes. | Feed cards expire at end of day | Flutter feed view |

**Success Criteria:** Mike knows his day plan before arriving at the first site. Zero surprises.

---

## Journey 2: Procurement Lifecycle (Sarah + Mike + Tom)

**Trigger:** Project "Maple Estate" is created with permit issued date.

| Step | Action | System | Actor |
|------|--------|--------|-------|
| 1 | Project created, TypeHydrateProject task enqueued | NewHydrateProjectTask(projectID) | System (auto) |
| 2 | Hydration populates procurement_items from WBS template | ProcurementAgent.HydrateProject() matches WBS 6.x items | System (auto) |
| 3 | CPM ForwardPass calculates EarlyStart for all tasks | BuildDependencyGraph -> ForwardPass -> BackwardPass | Physics Engine |
| 4 | ProcurementAgent daily cron (05:00 UTC) analyzes items | analyzeItem() calculates MustOrderDate for each item | ProcurementAgent |
| 5 | Day 1: Windows (12-week lead, EarlyStart = July 15) -> MustOrderDate = April 1. Status: WARNING | Feed card: "Order windows soon (by April 1)" | Sarah sees in dashboard |
| 6 | Day 5: MustOrderDate passed. Status: CRITICAL | Feed card: "ACTION REQUIRED: Order windows to avoid schedule slip" | Mike sees in Flutter app |
| 7 | Sarah clicks "Mark Ordered" -> confirms vendor and PO number | Updates procurement_item status, clears alert | Sarah in dashboard |
| 8 | If no action by Day 7: Tribunal evaluates autonomous order | ConsensusEngine reviews budget, vendor history, decides | Tribunal |
| 9 | If Tribunal approves and amount < Tom's auto-approve threshold | PO auto-generated, notification to Sarah for filing | System (auto) |
| 10 | If amount > threshold: Feed card to Tom: "Approve $8,500 window order?" | Tom taps "Approve" in Flutter or dashboard | Tom |

**Success Criteria:** Zero procurement items reach CRITICAL without human awareness. 80% of routine orders auto-processed.

---

## Journey 3: Corporate Financial Review (Tom, Owner)

**Trigger:** Monday morning. Tom opens dashboard on his MacBook.

| Step | Action | System | Touchpoint |
|------|--------|--------|-----------|
| 1 | Tom logs in (JWT from The Brain), lands on Organization dashboard | Centralized JWT delegation, dark Industrial theme | Web dashboard |
| 2 | Top row: 10 active projects, $4.2M total estimated, $2.8M committed, $1.9M actual | CorporateFinancialsServicer.RollupCorporateBudget() | Glassmorphism summary cards |
| 3 | AR Aging widget: $320K current, $45K 30-day, $12K 60-day, $8K 90+ | CalculateARAging() -> ARAgingSnapshot | Stacked bar chart (JetBrains Mono) |
| 4 | Tom clicks "Maple Estate" row -> drills into project financials | GetBudgetBreakdown by WBS phase | Project detail panel |
| 5 | Sees Foundation phase: $120K estimated, $135K committed (12% over) | Budget vs actual comparison with variance highlighting | Phase-level table |
| 6 | Clicks "View Invoices" -> sees 3 foundation invoices matched to WBS 8.x | InvoiceServicer extraction with WBS code prediction | Invoice detail cards |
| 7 | Tom clicks "Prepare Draw Request" -> G702/G703 auto-populated | Task completion % from CPM + budget actuals | AIA billing form |
| 8 | Total review time: 8 minutes for 10 projects | All data from daily TypeCorporateRollup cron | Dashboard |

**Success Criteria:** Tom sees company health in under 10 minutes without asking Sarah.

---

## Journey 4: Field Progress Reporting (Carlos, Field Worker)

**Trigger:** 2:45 PM. Carlos finished framing second-floor walls at Sunrise Villa.

| Step | Action | System | Touchpoint |
|------|--------|--------|-----------|
| 1 | Carlos opens Flutter app, sees today's assigned task: "WBS 9.2 - Second Floor Framing" | Task list filtered by project + date + crew assignment | Flutter task card |
| 2 | Taps task, sees photo of completed first-floor framing for reference | Historical progress photos from project assets | Photo gallery |
| 3 | Takes 2 photos of completed work from his phone | Camera integration with GPS + timestamp metadata | Flutter camera |
| 4 | Drags progress slider to 100%, taps green checkmark | CreateTaskProgress(percentComplete=100) | One-tap completion |
| 5 | App shows: "Marked complete. Framing inspection next: April 8" | Next dependent task lookup from DependencyGraph | Confirmation with context |
| 6 | Mike receives notification: "Carlos completed 9.2 Second Floor Framing" | Feed card written by system | Push notification |
| 7 | Total interaction time: 45 seconds | Offline-capable, syncs when connectivity available | Flutter app |

**Success Criteria:** Carlos reports progress in under 1 minute. Data flows to CPM automatically.

---

## Journey 5: Sub Coordination (System + Mike)

**Trigger:** SubLiaisonAgent cron detects plumbing rough-in (WBS 10.0) starting in 48 hours.

| Step | Action | System | Actor |
|------|--------|--------|-------|
| 1 | SubLiaisonAgent.ScanAndNotify() finds WBS 10.0 with EarlyStart in 48h | Query project_tasks WHERE early_start BETWEEN now AND now+72h | System |
| 2 | DirectoryService.GetContactForPhase("10") returns "Joe's Plumbing, Joe Smith" | Contact lookup from PROJECT_ASSIGNMENTS | System |
| 3 | Idempotency check: no SENT communication to Joe for this task in 24h | hasRecentCommunication() query | System |
| 4 | Outbox: Insert PENDING record in communication_logs | Transactional outbox pattern | System |
| 5 | SMS sent: "FutureBuild: Please confirm arrival for 'Plumbing Rough-In' scheduled April 8. Reply YES to confirm." | notifier.SendSMS() via Twilio | System -> Joe |
| 6 | Record updated to SENT | updateCommunicationStatus() | System |
| 7 | Feed card: "Awaiting confirmation from Joe Smith for Plumbing Rough-In" | writeSubUnconfirmedCard() | Mike sees in app |
| 8a | Joe replies "yes confirmed for Tuesday" | HandleInboundMessage() detects confirmation keywords | Joe -> System |
| 8b | OR Joe replies "delayed, can not make it until Thursday" | containsDelayIndicator() detects "delayed" | Joe -> System |
| 9a | If confirmed: Feed card "Joe Smith confirmed for Plumbing Rough-In" (priority: low) | writeSubConfirmationCard() | Mike sees |
| 9b | If delayed: Risk flag created, feed card "Delay reported by Joe Smith" (priority: urgent) | createRiskFlag() + writeSubDelayCard() | Mike sees |
| 10 | If no response after 24h: Auto-escalation SMS + feed card to Mike to call | Future enhancement | System -> Mike |

**Success Criteria:** Mike does not make a single phone call for routine sub confirmations.
