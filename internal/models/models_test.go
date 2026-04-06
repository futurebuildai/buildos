package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// =============================================================================
// Procurement Status Validation
// =============================================================================

func TestValidProcurementStatuses_AllDefined(t *testing.T) {
	expected := []ProcurementStatus{
		ProcurementPending,
		ProcurementWarning,
		ProcurementCritical,
		ProcurementDelivered,
		ProcurementCancelled,
	}

	if len(ValidProcurementStatuses) != len(expected) {
		t.Errorf("ValidProcurementStatuses has %d entries, want %d",
			len(ValidProcurementStatuses), len(expected))
	}

	for _, s := range expected {
		if !ValidProcurementStatuses[s] {
			t.Errorf("expected %q to be in ValidProcurementStatuses", s)
		}
	}
}

func TestValidProcurementStatuses_RejectsInvalid(t *testing.T) {
	invalid := []ProcurementStatus{
		"",
		"UNKNOWN",
		"pending",   // lowercase
		"Pending",   // mixed case
		"delivered", // lowercase
		"ORDERED",
		"IN_TRANSIT",
	}

	for _, s := range invalid {
		if ValidProcurementStatuses[s] {
			t.Errorf("expected %q to NOT be in ValidProcurementStatuses", s)
		}
	}
}

func TestProcurementStatusConstants_Values(t *testing.T) {
	tests := []struct {
		name     string
		constant ProcurementStatus
		want     string
	}{
		{"Pending", ProcurementPending, "PENDING"},
		{"Warning", ProcurementWarning, "WARNING"},
		{"Critical", ProcurementCritical, "CRITICAL"},
		{"Delivered", ProcurementDelivered, "DELIVERED"},
		{"Cancelled", ProcurementCancelled, "CANCELLED"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.constant) != tc.want {
				t.Errorf("got %q, want %q", tc.constant, tc.want)
			}
		})
	}
}

func TestProcurementStatusTransitions(t *testing.T) {
	// Verify that known forward transitions use valid statuses on both sides
	transitions := []struct {
		from ProcurementStatus
		to   ProcurementStatus
	}{
		{ProcurementPending, ProcurementWarning},
		{ProcurementWarning, ProcurementCritical},
		{ProcurementPending, ProcurementDelivered},
		{ProcurementWarning, ProcurementDelivered},
		{ProcurementCritical, ProcurementDelivered},
		{ProcurementPending, ProcurementCancelled},
		{ProcurementWarning, ProcurementCancelled},
		{ProcurementCritical, ProcurementCancelled},
	}

	for _, tr := range transitions {
		if !ValidProcurementStatuses[tr.from] {
			t.Errorf("transition from %q: 'from' status not valid", tr.from)
		}
		if !ValidProcurementStatuses[tr.to] {
			t.Errorf("transition to %q: 'to' status not valid", tr.to)
		}
	}
}

// =============================================================================
// Financial Model Validation — BIGINT Cents + CurrencyCode
// =============================================================================

func TestProjectBudget_VarianceCents(t *testing.T) {
	tests := []struct {
		name      string
		estimated int64
		actual    int64
		want      int64
	}{
		{"under budget", 100000, 85000, 15000},
		{"over budget", 100000, 120000, -20000},
		{"exact budget", 100000, 100000, 0},
		{"zero budget", 0, 0, 0},
		{"large values", 999999999999, 500000000000, 499999999999},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := &ProjectBudget{
				EstimatedCostCents: tc.estimated,
				ActualCostCents:    tc.actual,
			}
			if got := b.VarianceCents(); got != tc.want {
				t.Errorf("VarianceCents() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestProjectBudget_CurrencyCodeDefaults(t *testing.T) {
	// Verify a fresh budget struct has zero-value currency codes (empty strings)
	b := ProjectBudget{}
	if b.EstimatedCostCurrencyCode != "" {
		t.Error("expected empty EstimatedCostCurrencyCode for zero-value struct")
	}
	if b.CommittedCostCurrencyCode != "" {
		t.Error("expected empty CommittedCostCurrencyCode for zero-value struct")
	}
	if b.ActualCostCurrencyCode != "" {
		t.Error("expected empty ActualCostCurrencyCode for zero-value struct")
	}
}

func TestProjectBudget_JSONRoundtrip(t *testing.T) {
	b := ProjectBudget{
		ID:                        uuid.New(),
		ProjectID:                 uuid.New(),
		WBSCode:                   "1.0",
		PhaseName:                 "Foundation",
		EstimatedCostCents:        50000,
		EstimatedCostCurrencyCode: "USD",
		CommittedCostCents:        30000,
		CommittedCostCurrencyCode: "USD",
		ActualCostCents:           25000,
		ActualCostCurrencyCode:    "USD",
		CreatedAt:                 time.Now().UTC().Truncate(time.Millisecond),
		UpdatedAt:                 time.Now().UTC().Truncate(time.Millisecond),
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got ProjectBudget
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if got.EstimatedCostCents != b.EstimatedCostCents {
		t.Errorf("EstimatedCostCents = %d, want %d", got.EstimatedCostCents, b.EstimatedCostCents)
	}
	if got.EstimatedCostCurrencyCode != b.EstimatedCostCurrencyCode {
		t.Errorf("EstimatedCostCurrencyCode = %q, want %q",
			got.EstimatedCostCurrencyCode, b.EstimatedCostCurrencyCode)
	}
}

func TestInvoice_StatusConstants(t *testing.T) {
	statuses := []struct {
		name  string
		value string
	}{
		{"pending", InvoiceStatusPending},
		{"approved", InvoiceStatusApproved},
		{"rejected", InvoiceStatusRejected},
		{"paid", InvoiceStatusPaid},
	}

	for _, s := range statuses {
		t.Run(s.name, func(t *testing.T) {
			if s.value != s.name {
				t.Errorf("InvoiceStatus %q = %q, want %q", s.name, s.value, s.name)
			}
		})
	}
}

func TestInvoice_MonetaryFieldsAreInt64(t *testing.T) {
	inv := Invoice{
		AmountCents:  9999999999999,
		CurrencyCode: "USD",
	}
	if inv.AmountCents != 9999999999999 {
		t.Error("AmountCents should support large int64 values")
	}
}

func TestCorporateBudget_Fields(t *testing.T) {
	cb := CorporateBudget{
		ID:                  uuid.New(),
		OrgID:               uuid.New(),
		FiscalYear:          2026,
		Quarter:             2,
		CurrencyCode:        "CAD",
		TotalEstimatedCents: 50000000,
		TotalCommittedCents: 30000000,
		TotalActualCents:    20000000,
		ProjectCount:        5,
	}
	if cb.FiscalYear != 2026 {
		t.Errorf("FiscalYear = %d, want 2026", cb.FiscalYear)
	}
	if cb.Quarter != 2 {
		t.Errorf("Quarter = %d, want 2", cb.Quarter)
	}
	if cb.CurrencyCode != "CAD" {
		t.Errorf("CurrencyCode = %q, want CAD", cb.CurrencyCode)
	}
}

func TestARAgingSnapshot_TotalReceivable(t *testing.T) {
	snap := ARAgingSnapshot{
		CurrencyCode:         "USD",
		CurrentCents:         10000,
		Days30Cents:          5000,
		Days60Cents:          3000,
		Days90PlusCents:      2000,
		TotalReceivableCents: 20000,
	}
	sum := snap.CurrentCents + snap.Days30Cents + snap.Days60Cents + snap.Days90PlusCents
	if sum != snap.TotalReceivableCents {
		t.Errorf("bucket sum %d != TotalReceivableCents %d", sum, snap.TotalReceivableCents)
	}
}

func TestProjectFinancialSummary_Fields(t *testing.T) {
	s := ProjectFinancialSummary{
		ProjectID:           uuid.New(),
		ProjectName:         "Test Project",
		CurrencyCode:        "USD",
		TotalEstimatedCents: 100000,
		TotalCommittedCents: 80000,
		TotalActualCents:    60000,
		VarianceCents:       40000,
		InvoiceCount:        10,
		PendingInvoiceCents: 15000,
	}
	if s.VarianceCents != s.TotalEstimatedCents-s.TotalActualCents {
		t.Errorf("VarianceCents %d != estimated-actual (%d)", s.VarianceCents,
			s.TotalEstimatedCents-s.TotalActualCents)
	}
}

// =============================================================================
// Feed Card Model Validation
// =============================================================================

func TestFeedPriority_Constants(t *testing.T) {
	priorities := map[string]string{
		"critical": PriorityCritical,
		"urgent":   PriorityUrgent,
		"normal":   PriorityNormal,
		"low":      PriorityLow,
	}
	for expected, got := range priorities {
		if got != expected {
			t.Errorf("Priority constant %q = %q, want %q", expected, got, expected)
		}
	}
}

func TestFeedStatus_Constants(t *testing.T) {
	statuses := map[string]string{
		"active":    FeedStatusActive,
		"dismissed": FeedStatusDismissed,
		"actioned":  FeedStatusActioned,
		"expired":   FeedStatusExpired,
	}
	for expected, got := range statuses {
		if got != expected {
			t.Errorf("FeedStatus %q = %q, want %q", expected, got, expected)
		}
	}
}

func TestCardType_Constants(t *testing.T) {
	types := map[string]string{
		"daily_briefing":    CardTypeBriefing,
		"procurement_alert": CardTypeProcurement,
		"weather_alert":     CardTypeWeatherAlert,
		"sub_confirmation":  CardTypeSubConfirmation,
		"progress_update":   CardTypeProgress,
		"agent_approval":    CardTypeAgentApproval,
	}
	for expected, got := range types {
		if got != expected {
			t.Errorf("CardType %q = %q, want %q", expected, got, expected)
		}
	}
}

func TestAgentType_Constants(t *testing.T) {
	tests := []struct {
		agent AgentType
		want  string
	}{
		{AgentDailyFocus, "daily_focus"},
		{AgentProcurement, "procurement"},
		{AgentSubLiaison, "sub_liaison"},
	}
	for _, tc := range tests {
		if string(tc.agent) != tc.want {
			t.Errorf("AgentType = %q, want %q", tc.agent, tc.want)
		}
	}
}

func TestFeedCard_JSONRoundtrip(t *testing.T) {
	projectID := uuid.New()
	userID := uuid.New()
	taskID := uuid.New()
	headline := "Test Headline"
	consequence := "Schedule delay"
	horizon := "7d"
	agentSource := "daily_focus"
	now := time.Now().UTC().Truncate(time.Millisecond)

	card := FeedCard{
		ID:          uuid.New(),
		OrgID:       uuid.New(),
		ProjectID:   &projectID,
		CardType:    CardTypeBriefing,
		Title:       "Morning Briefing",
		Body:        "Summary of today's work",
		Priority:    PriorityCritical,
		TargetUserID: &userID,
		TargetRole:  "superintendent",
		Actions:     json.RawMessage(`[{"label":"Review","action":"open"}]`),
		Status:      FeedStatusActive,
		CreatedAt:   now,
		Headline:    &headline,
		Consequence: &consequence,
		Horizon:     &horizon,
		AgentSource: &agentSource,
		TaskID:      &taskID,
		EngineData:  json.RawMessage(`{"score":0.95}`),
	}

	data, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var got FeedCard
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if got.CardType != card.CardType {
		t.Errorf("CardType = %q, want %q", got.CardType, card.CardType)
	}
	if got.Priority != card.Priority {
		t.Errorf("Priority = %q, want %q", got.Priority, card.Priority)
	}
	if got.Title != card.Title {
		t.Errorf("Title = %q, want %q", got.Title, card.Title)
	}
	if got.ProjectID == nil || *got.ProjectID != projectID {
		t.Error("ProjectID mismatch after roundtrip")
	}
	if got.Headline == nil || *got.Headline != headline {
		t.Error("Headline mismatch after roundtrip")
	}
	if got.TaskID == nil || *got.TaskID != taskID {
		t.Error("TaskID mismatch after roundtrip")
	}
}

func TestFeedFilter_Defaults(t *testing.T) {
	f := FeedFilter{}
	if f.Priority != "" {
		t.Error("expected empty default Priority")
	}
	if f.Status != "" {
		t.Error("expected empty default Status")
	}
	if f.Limit != 0 {
		t.Error("expected zero default Limit")
	}
	if f.Offset != 0 {
		t.Error("expected zero default Offset")
	}
}

func TestPendingAction_StatusConstants(t *testing.T) {
	statuses := map[string]string{
		"pending":  PendingActionPending,
		"approved": PendingActionApproved,
		"rejected": PendingActionRejected,
		"expired":  PendingActionExpired,
	}
	for expected, got := range statuses {
		if got != expected {
			t.Errorf("PendingActionStatus %q = %q, want %q", expected, got, expected)
		}
	}
}

func TestCommunicationLog_Fields(t *testing.T) {
	idempKey := uuid.New()
	log := CommunicationLog{
		ID:             uuid.New(),
		ProjectID:      uuid.New(),
		TaskID:         uuid.New(),
		ContactName:    "John Smith",
		ContactPhone:   "555-1234",
		MessageType:    "sms",
		MessageBody:    "Are you available?",
		Status:         "sent",
		IdempotencyKey: &idempKey,
	}
	if log.ContactName != "John Smith" {
		t.Errorf("ContactName = %q, want %q", log.ContactName, "John Smith")
	}
	if log.IdempotencyKey == nil || *log.IdempotencyKey != idempKey {
		t.Error("IdempotencyKey mismatch")
	}
}

// =============================================================================
// Pipeline Model Validation
// =============================================================================

func TestPipelineStage_Order(t *testing.T) {
	expected := []PipelineStage{
		StageLead,
		StageQualified,
		StageEstimateSent,
		StageVerbalCommitment,
		StagePermitApplied,
		StagePermitIssued,
	}
	if len(StageOrder) != len(expected) {
		t.Fatalf("StageOrder length = %d, want %d", len(StageOrder), len(expected))
	}
	for i, s := range expected {
		if StageOrder[i] != s {
			t.Errorf("StageOrder[%d] = %q, want %q", i, StageOrder[i], s)
		}
	}
}

func TestStageProbability_ValidRange(t *testing.T) {
	for stage, prob := range StageProbability {
		if prob < 0 || prob > 100 {
			t.Errorf("StageProbability[%q] = %d, want 0..100", stage, prob)
		}
	}
}

func TestStageProbability_LostIsZero(t *testing.T) {
	if StageProbability[StageLost] != 0 {
		t.Errorf("StageProbability[LOST] = %d, want 0", StageProbability[StageLost])
	}
}

func TestStageProbability_PermitIssuedIs100(t *testing.T) {
	if StageProbability[StagePermitIssued] != 100 {
		t.Errorf("StageProbability[PERMIT_ISSUED] = %d, want 100", StageProbability[StagePermitIssued])
	}
}

func TestStageProbability_MonotonicallyIncreasing(t *testing.T) {
	// Probability should increase (or stay same) along StageOrder
	for i := 1; i < len(StageOrder); i++ {
		prev := StageProbability[StageOrder[i-1]]
		curr := StageProbability[StageOrder[i]]
		if curr < prev {
			t.Errorf("StageProbability should be non-decreasing: %q (%d) > %q (%d)",
				StageOrder[i-1], prev, StageOrder[i], curr)
		}
	}
}

func TestPipelineEstimate_MonetaryFields(t *testing.T) {
	est := PipelineEstimate{
		TotalEstimatedCents: 50000000,
		CurrencyCode:        "USD",
		MarginPct:           15,
	}
	if est.TotalEstimatedCents != 50000000 {
		t.Errorf("TotalEstimatedCents = %d, want 50000000", est.TotalEstimatedCents)
	}
	if est.CurrencyCode != "USD" {
		t.Errorf("CurrencyCode = %q, want USD", est.CurrencyCode)
	}
}

func TestPermit_MonetaryFields(t *testing.T) {
	permit := Permit{
		FeeCents:        75000,
		FeeCurrencyCode: "CAD",
		Status:          PermitStatusSubmitted,
	}
	if permit.FeeCents != 75000 {
		t.Errorf("FeeCents = %d, want 75000", permit.FeeCents)
	}
	if permit.FeeCurrencyCode != "CAD" {
		t.Errorf("FeeCurrencyCode = %q, want CAD", permit.FeeCurrencyCode)
	}
}

func TestPermitStatus_Constants(t *testing.T) {
	statuses := []struct {
		name  string
		value string
	}{
		{"not_submitted", PermitStatusNotSubmitted},
		{"submitted", PermitStatusSubmitted},
		{"under_review", PermitStatusUnderReview},
		{"revisions_requested", PermitStatusRevisionsRequested},
		{"approved", PermitStatusApproved},
		{"denied", PermitStatusDenied},
	}
	for _, s := range statuses {
		if s.value != s.name {
			t.Errorf("PermitStatus %q = %q", s.name, s.value)
		}
	}
}

func TestEstimateStatus_Constants(t *testing.T) {
	statuses := []struct {
		name  string
		value string
	}{
		{"draft", EstimateStatusDraft},
		{"sent", EstimateStatusSent},
		{"revised", EstimateStatusRevised},
		{"accepted", EstimateStatusAccepted},
	}
	for _, s := range statuses {
		if s.value != s.name {
			t.Errorf("EstimateStatus %q = %q", s.name, s.value)
		}
	}
}

// =============================================================================
// Physics Model Validation
// =============================================================================

func TestDependencyType_Constants(t *testing.T) {
	types := []struct {
		dt   DependencyType
		want string
	}{
		{DependencyTypeFS, "FS"},
		{DependencyTypeSS, "SS"},
		{DependencyTypeFF, "FF"},
		{DependencyTypeSF, "SF"},
	}
	for _, tc := range types {
		if string(tc.dt) != tc.want {
			t.Errorf("DependencyType = %q, want %q", tc.dt, tc.want)
		}
	}
}

func TestTaskStatus_Constants(t *testing.T) {
	statuses := []struct {
		ts   TaskStatus
		want string
	}{
		{TaskStatusPending, "Pending"},
		{TaskStatusReady, "Ready"},
		{TaskStatusInProgress, "In_Progress"},
		{TaskStatusInspectionPending, "Inspection_Pending"},
		{TaskStatusCompleted, "Completed"},
		{TaskStatusBlocked, "Blocked"},
		{TaskStatusDelayed, "Delayed"},
	}
	for _, tc := range statuses {
		if string(tc.ts) != tc.want {
			t.Errorf("TaskStatus = %q, want %q", tc.ts, tc.want)
		}
	}
}

func TestMultiplierSource_Constants(t *testing.T) {
	sources := []struct {
		ms   MultiplierSource
		want string
	}{
		{MultiplierSourceDefault, "default"},
		{MultiplierSourceOrgTrained, "org_trained"},
		{MultiplierSourceGlobalTrained, "global_trained"},
	}
	for _, tc := range sources {
		if string(tc.ms) != tc.want {
			t.Errorf("MultiplierSource = %q, want %q", tc.ms, tc.want)
		}
	}
}

// =============================================================================
// Fleet / HR / Field Model Validation
// =============================================================================

func TestFleetAssetStatus_Constants(t *testing.T) {
	statuses := []struct {
		name  string
		value string
	}{
		{"available", AssetStatusAvailable},
		{"allocated", AssetStatusAllocated},
		{"maintenance", AssetStatusMaintenance},
		{"retired", AssetStatusRetired},
	}
	for _, s := range statuses {
		if s.value != s.name {
			t.Errorf("AssetStatus %q = %q", s.name, s.value)
		}
	}
}

func TestCertStatus_Constants(t *testing.T) {
	statuses := []struct {
		name  string
		value string
	}{
		{"active", CertStatusActive},
		{"expired", CertStatusExpired},
		{"revoked", CertStatusRevoked},
	}
	for _, s := range statuses {
		if s.value != s.name {
			t.Errorf("CertStatus %q = %q", s.name, s.value)
		}
	}
}

func TestFieldProgress_PercentComplete(t *testing.T) {
	fp := FieldProgress{
		PercentComplete: 100,
		IdempotencyKey:  uuid.New().String(),
	}
	if fp.PercentComplete != 100 {
		t.Errorf("PercentComplete = %d, want 100", fp.PercentComplete)
	}
}

func TestSyncPayload_Structure(t *testing.T) {
	payload := SyncPayload{
		FeedCards: []FeedCard{{Title: "Test Card"}},
		Tasks:     []SyncTask{{Name: "Task 1"}},
		SyncedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if len(payload.FeedCards) != 1 {
		t.Errorf("FeedCards length = %d, want 1", len(payload.FeedCards))
	}
	if len(payload.Tasks) != 1 {
		t.Errorf("Tasks length = %d, want 1", len(payload.Tasks))
	}
}

// =============================================================================
// Meta-test: Verify Composite Currency Pattern via reflect
//
// Every struct field ending in "Cents" (int64) must have a companion
// "CurrencyCode" (string) field on the SAME struct — enforcing the
// BIGINT cents + currency_code contract.
// =============================================================================

func TestCompositeCurrencyPattern_AllCentsFieldsHaveCurrencyCode(t *testing.T) {
	// All model types with monetary fields
	monetaryTypes := []struct {
		name string
		typ  reflect.Type
	}{
		{"ProjectBudget", reflect.TypeOf(ProjectBudget{})},
		{"CorporateBudget", reflect.TypeOf(CorporateBudget{})},
		{"ARAgingSnapshot", reflect.TypeOf(ARAgingSnapshot{})},
		{"Invoice", reflect.TypeOf(Invoice{})},
		{"ProjectFinancialSummary", reflect.TypeOf(ProjectFinancialSummary{})},
		{"ProcurementItem", reflect.TypeOf(ProcurementItem{})},
		{"ProcurementCostSummary", reflect.TypeOf(ProcurementCostSummary{})},
		{"PipelineEstimate", reflect.TypeOf(PipelineEstimate{})},
		{"Permit", reflect.TypeOf(Permit{})},
		{"PipelineAnalytics", reflect.TypeOf(PipelineAnalytics{})},
		// StageAnalytics is excluded: it is a child struct within
		// PipelineAnalytics.ByStage and inherits CurrencyCode from the parent.
	}

	for _, mt := range monetaryTypes {
		t.Run(mt.name, func(t *testing.T) {
			checkCentsFieldsHaveCurrencyCode(t, mt.typ)
		})
	}
}

// checkCentsFieldsHaveCurrencyCode verifies that for each *Cents field,
// a companion CurrencyCode field exists on the same struct.
//
// The companion naming convention:
//   - "FooCents" must have "FooCurrencyCode" (e.g., EstimatedCostCents -> EstimatedCostCurrencyCode)
//   - OR the struct must have a standalone "CurrencyCode" field
//     (for structs where all Cents fields share a single currency, e.g., Invoice.AmountCents + Invoice.CurrencyCode)
func checkCentsFieldsHaveCurrencyCode(t *testing.T, typ reflect.Type) {
	t.Helper()

	// Build a set of field names
	fieldNames := make(map[string]bool)
	for i := 0; i < typ.NumField(); i++ {
		fieldNames[typ.Field(i).Name] = true
	}

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !strings.HasSuffix(f.Name, "Cents") {
			continue
		}

		// Verify it's int64
		if f.Type.Kind() != reflect.Int64 {
			t.Errorf("%s.%s ends in 'Cents' but is %s, want int64",
				typ.Name(), f.Name, f.Type.Kind())
		}

		// Determine expected companion name:
		// "FooCents" -> "FooCurrencyCode"
		prefix := strings.TrimSuffix(f.Name, "Cents")
		specificCompanion := prefix + "CurrencyCode"

		// Either the specific companion exists, or a generic "CurrencyCode" exists
		if !fieldNames[specificCompanion] && !fieldNames["CurrencyCode"] {
			t.Errorf("%s.%s has no companion CurrencyCode field (tried %q and %q)",
				typ.Name(), f.Name, specificCompanion, "CurrencyCode")
		}
	}
}

// TestNonMonetaryStructsHaveNoCentsFields verifies that structs not in the
// monetary list do not accidentally introduce Cents fields without the pattern.
func TestFieldModels_NoCentsWithoutCurrencyCode(t *testing.T) {
	nonMonetaryTypes := []struct {
		name string
		typ  reflect.Type
	}{
		{"FieldProgress", reflect.TypeOf(FieldProgress{})},
		{"FieldCheckin", reflect.TypeOf(FieldCheckin{})},
		{"DailyLog", reflect.TypeOf(DailyLog{})},
		{"FleetAsset", reflect.TypeOf(FleetAsset{})},
		{"EquipmentAllocation", reflect.TypeOf(EquipmentAllocation{})},
		{"Employee", reflect.TypeOf(Employee{})},
		{"Certification", reflect.TypeOf(Certification{})},
	}

	for _, mt := range nonMonetaryTypes {
		t.Run(mt.name, func(t *testing.T) {
			for i := 0; i < mt.typ.NumField(); i++ {
				f := mt.typ.Field(i)
				if strings.HasSuffix(f.Name, "Cents") {
					t.Errorf("%s.%s has Cents suffix — must follow Composite Currency Pattern",
						mt.name, f.Name)
				}
			}
		})
	}
}
