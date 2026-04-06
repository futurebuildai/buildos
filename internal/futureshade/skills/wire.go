package skills

// WireConfig holds all dependencies needed to wire FutureShade skills
// to real agent and service implementations.
type WireConfig struct {
	// DailyFocus implements DailyFocusExecutor (agents.DailyFocusAgent satisfies this).
	DailyFocus DailyFocusExecutor

	// Procurement implements ProcurementExecutor (agents.ProcurementAgent satisfies this).
	Procurement ProcurementExecutor

	// ScheduleRecalc implements ScheduleRecalcExecutor
	// (tools.ScheduleRecalcAdapter satisfies this).
	ScheduleRecalc ScheduleRecalcExecutor
}

// NewWiredRegistry creates a skill registry with all skills wired to real
// service implementations. Skills with nil executors are skipped (fail-open).
//
// Wiring:
//   - daily_focus_sync -> agents.DailyFocusAgent.GenerateBriefings
//   - procurement_sync -> agents.ProcurementAgent.RunCheck
//   - schedule_recalc  -> tools.ScheduleRecalcAdapter (physics engine CPM)
func NewWiredRegistry(cfg WireConfig) *Registry {
	r := NewRegistry()

	if cfg.DailyFocus != nil {
		r.Register(NewDailyFocusSyncSkill(cfg.DailyFocus))
	}

	if cfg.Procurement != nil {
		r.Register(NewProcurementSyncSkill(cfg.Procurement))
	}

	if cfg.ScheduleRecalc != nil {
		r.Register(NewScheduleRecalcSkill(cfg.ScheduleRecalc))
	}

	return r
}
