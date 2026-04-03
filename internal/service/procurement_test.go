package service

import (
	"testing"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

func TestProcurementStatus_Valid(t *testing.T) {
	valid := []models.ProcurementStatus{
		models.ProcurementPending,
		models.ProcurementWarning,
		models.ProcurementCritical,
		models.ProcurementDelivered,
		models.ProcurementCancelled,
	}
	for _, s := range valid {
		if !models.ValidProcurementStatuses[s] {
			t.Errorf("expected %q to be valid", s)
		}
	}
}

func TestProcurementStatus_Invalid(t *testing.T) {
	invalid := []models.ProcurementStatus{
		"UNKNOWN",
		"",
		"delivered", // lowercase
	}
	for _, s := range invalid {
		if models.ValidProcurementStatuses[s] {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestProcurementSupportedCurrencies(t *testing.T) {
	if !SupportedCurrencies["USD"] {
		t.Error("USD should be supported")
	}
	if !SupportedCurrencies["CAD"] {
		t.Error("CAD should be supported")
	}
	if SupportedCurrencies["EUR"] {
		t.Error("EUR should not be supported")
	}
	if SupportedCurrencies["GBP"] {
		t.Error("GBP should not be supported")
	}
}

func TestProcurementCostSummaryFields(t *testing.T) {
	s := models.ProcurementCostSummary{
		CurrencyCode: "USD",
		TotalCents:   150000,
		ItemCount:    3,
	}
	if s.CurrencyCode != "USD" {
		t.Errorf("expected USD, got %s", s.CurrencyCode)
	}
	if s.TotalCents != 150000 {
		t.Errorf("expected 150000, got %d", s.TotalCents)
	}
	if s.ItemCount != 3 {
		t.Errorf("expected 3, got %d", s.ItemCount)
	}
}

func TestProcurementItemModel(t *testing.T) {
	// Verify naming convention: EstimatedCostCents + EstimatedCostCurrencyCode
	item := models.ProcurementItem{
		Description:               "Lumber package",
		EstimatedCostCents:        50000,
		EstimatedCostCurrencyCode: "USD",
		Status:                    models.ProcurementPending,
	}
	if item.EstimatedCostCents != 50000 {
		t.Errorf("expected 50000, got %d", item.EstimatedCostCents)
	}
	if item.EstimatedCostCurrencyCode != "USD" {
		t.Errorf("expected USD, got %s", item.EstimatedCostCurrencyCode)
	}
}

func TestFeedPriorityConstants(t *testing.T) {
	tests := []struct {
		name     string
		priority string
	}{
		{"critical", models.PriorityCritical},
		{"urgent", models.PriorityUrgent},
		{"normal", models.PriorityNormal},
		{"low", models.PriorityLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.priority != tt.name {
				t.Errorf("expected %q, got %q", tt.name, tt.priority)
			}
		})
	}
}

func TestFeedStatusConstants(t *testing.T) {
	if models.FeedStatusActive != "active" {
		t.Error("FeedStatusActive should be 'active'")
	}
	if models.FeedStatusDismissed != "dismissed" {
		t.Error("FeedStatusDismissed should be 'dismissed'")
	}
	if models.FeedStatusActioned != "actioned" {
		t.Error("FeedStatusActioned should be 'actioned'")
	}
}

func TestCardTypeConstants(t *testing.T) {
	if models.CardTypeBriefing != "daily_briefing" {
		t.Errorf("expected daily_briefing, got %s", models.CardTypeBriefing)
	}
	if models.CardTypeProcurement != "procurement_alert" {
		t.Errorf("expected procurement_alert, got %s", models.CardTypeProcurement)
	}
}

func TestAgentTypeConstants(t *testing.T) {
	if models.AgentDailyFocus != "daily_focus" {
		t.Errorf("expected daily_focus, got %s", models.AgentDailyFocus)
	}
	if models.AgentProcurement != "procurement" {
		t.Errorf("expected procurement, got %s", models.AgentProcurement)
	}
	if models.AgentSubLiaison != "sub_liaison" {
		t.Errorf("expected sub_liaison, got %s", models.AgentSubLiaison)
	}
}

func TestStatusChangeStruct(t *testing.T) {
	change := StatusChange{
		OldStatus: models.ProcurementPending,
		NewStatus: models.ProcurementWarning,
		DaysLeft:  5,
	}
	if change.OldStatus != models.ProcurementPending {
		t.Errorf("expected PENDING, got %s", change.OldStatus)
	}
	if change.NewStatus != models.ProcurementWarning {
		t.Errorf("expected WARNING, got %s", change.NewStatus)
	}
	if change.DaysLeft != 5 {
		t.Errorf("expected 5, got %d", change.DaysLeft)
	}
}
