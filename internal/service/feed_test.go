package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewFeedService(t *testing.T) {
	svc := NewFeedService(nil)
	if svc == nil {
		t.Fatal("expected non-nil FeedService")
	}
	if svc.store != nil {
		t.Error("expected nil store when created with nil")
	}
}

// ---------------------------------------------------------------------------
// CreateCard — validation
// ---------------------------------------------------------------------------

func TestFeed_CreateCard_MissingOrgID(t *testing.T) {
	svc := NewFeedService(nil)
	card := &models.FeedCard{
		OrgID: uuid.Nil,
		Title: "Test Card",
	}
	_, err := svc.CreateCard(nil, card)
	if !errors.Is(err, ErrMissingOrgID) {
		t.Errorf("expected ErrMissingOrgID, got: %v", err)
	}
}

func TestFeed_CreateCard_MissingTitle(t *testing.T) {
	svc := NewFeedService(nil)
	card := &models.FeedCard{
		OrgID: uuid.New(),
		Title: "",
	}
	_, err := svc.CreateCard(nil, card)
	if !errors.Is(err, ErrMissingTitle) {
		t.Errorf("expected ErrMissingTitle, got: %v", err)
	}
}

func TestFeed_CreateCard_BothMissing_OrgIDCheckedFirst(t *testing.T) {
	svc := NewFeedService(nil)
	card := &models.FeedCard{
		OrgID: uuid.Nil,
		Title: "",
	}
	_, err := svc.CreateCard(nil, card)
	// OrgID is checked before title
	if !errors.Is(err, ErrMissingOrgID) {
		t.Errorf("expected ErrMissingOrgID (checked first), got: %v", err)
	}
}

func TestFeed_CreateCard_DefaultStatus(t *testing.T) {
	svc := NewFeedService(nil)
	card := &models.FeedCard{
		OrgID:  uuid.New(),
		Title:  "Test Card",
		Status: "",
	}

	func() {
		defer func() { recover() }()
		_, _ = svc.CreateCard(nil, card)
	}()

	if card.Status != models.FeedStatusActive {
		t.Errorf("expected default status %q, got %q", models.FeedStatusActive, card.Status)
	}
}

func TestFeed_CreateCard_DefaultPriority(t *testing.T) {
	svc := NewFeedService(nil)
	card := &models.FeedCard{
		OrgID:    uuid.New(),
		Title:    "Test Card",
		Priority: "",
	}

	func() {
		defer func() { recover() }()
		_, _ = svc.CreateCard(nil, card)
	}()

	if card.Priority != models.PriorityNormal {
		t.Errorf("expected default priority %q, got %q", models.PriorityNormal, card.Priority)
	}
}

func TestFeed_CreateCard_ExplicitStatusPreserved(t *testing.T) {
	svc := NewFeedService(nil)
	statuses := []string{
		models.FeedStatusActive,
		models.FeedStatusDismissed,
		models.FeedStatusActioned,
		models.FeedStatusExpired,
	}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			card := &models.FeedCard{
				OrgID:  uuid.New(),
				Title:  "Test",
				Status: status,
			}
			func() {
				defer func() { recover() }()
				_, _ = svc.CreateCard(nil, card)
			}()
			if card.Status != status {
				t.Errorf("expected preserved status %q, got %q", status, card.Status)
			}
		})
	}
}

func TestFeed_CreateCard_ExplicitPriorityPreserved(t *testing.T) {
	svc := NewFeedService(nil)
	priorities := []string{
		models.PriorityCritical,
		models.PriorityUrgent,
		models.PriorityNormal,
		models.PriorityLow,
	}
	for _, p := range priorities {
		t.Run(p, func(t *testing.T) {
			card := &models.FeedCard{
				OrgID:    uuid.New(),
				Title:    "Test",
				Priority: p,
			}
			func() {
				defer func() { recover() }()
				_, _ = svc.CreateCard(nil, card)
			}()
			if card.Priority != p {
				t.Errorf("expected preserved priority %q, got %q", p, card.Priority)
			}
		})
	}
}

func TestFeed_CreateCard_ValidCard_ReachesStore(t *testing.T) {
	svc := NewFeedService(nil)
	card := &models.FeedCard{
		OrgID:    uuid.New(),
		Title:    "Test Card",
		CardType: models.CardTypeBriefing,
		Priority: models.PriorityNormal,
		Status:   models.FeedStatusActive,
	}

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.CreateCard(nil, card)
	}()
	if !panicked {
		t.Error("expected panic from nil store after passing validation")
	}
}

// ---------------------------------------------------------------------------
// ListCards — reaches store
// ---------------------------------------------------------------------------

func TestFeed_ListCards_ReachesStore(t *testing.T) {
	svc := NewFeedService(nil)
	orgID := uuid.New()
	userID := uuid.New()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _, _ = svc.ListCards(nil, orgID, &userID, "admin", models.FeedFilter{})
	}()
	if !panicked {
		t.Error("expected panic from nil store on ListCards")
	}
}

func TestFeed_ListCards_NilUserID(t *testing.T) {
	svc := NewFeedService(nil)
	orgID := uuid.New()

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _, _ = svc.ListCards(nil, orgID, nil, "foreman", models.FeedFilter{
			Priority: models.PriorityCritical,
			Status:   models.FeedStatusActive,
			Limit:    10,
			Offset:   0,
		})
	}()
	if !panicked {
		t.Error("expected panic from nil store on ListCards with nil userID")
	}
}

// ---------------------------------------------------------------------------
// DismissCard — reaches store
// ---------------------------------------------------------------------------

func TestFeed_DismissCard_ReachesStore(t *testing.T) {
	svc := NewFeedService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_ = svc.DismissCard(nil, uuid.New())
	}()
	if !panicked {
		t.Error("expected panic from nil store on DismissCard")
	}
}

// ---------------------------------------------------------------------------
// ActionCard — reaches store
// ---------------------------------------------------------------------------

func TestFeed_ActionCard_ReachesStore(t *testing.T) {
	svc := NewFeedService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_ = svc.ActionCard(nil, uuid.New())
	}()
	if !panicked {
		t.Error("expected panic from nil store on ActionCard")
	}
}

// ---------------------------------------------------------------------------
// Sentinel error messages
// ---------------------------------------------------------------------------

func TestFeed_SentinelErrors(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{ErrCardNotFound, "feed card not found or not active"},
		{ErrMissingTitle, "feed card title is required"},
		{ErrMissingOrgID, "org_id is required"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if tc.err.Error() != tc.want {
				t.Errorf("got %q, want %q", tc.err.Error(), tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FeedFilter struct
// ---------------------------------------------------------------------------

func TestFeedFilter_ZeroValues(t *testing.T) {
	f := models.FeedFilter{}
	if f.Priority != "" {
		t.Errorf("expected empty priority, got %q", f.Priority)
	}
	if f.Status != "" {
		t.Errorf("expected empty status, got %q", f.Status)
	}
	if f.Limit != 0 {
		t.Errorf("expected 0 limit, got %d", f.Limit)
	}
	if f.Offset != 0 {
		t.Errorf("expected 0 offset, got %d", f.Offset)
	}
}

func TestFeedFilter_AllFieldsSet(t *testing.T) {
	f := models.FeedFilter{
		Priority: models.PriorityCritical,
		Status:   models.FeedStatusActive,
		Limit:    50,
		Offset:   10,
	}
	if f.Priority != models.PriorityCritical {
		t.Errorf("Priority = %q, want %q", f.Priority, models.PriorityCritical)
	}
	if f.Status != models.FeedStatusActive {
		t.Errorf("Status = %q, want %q", f.Status, models.FeedStatusActive)
	}
	if f.Limit != 50 {
		t.Errorf("Limit = %d, want 50", f.Limit)
	}
	if f.Offset != 10 {
		t.Errorf("Offset = %d, want 10", f.Offset)
	}
}

// ---------------------------------------------------------------------------
// Priority constants
// ---------------------------------------------------------------------------

func TestFeed_PriorityConstants(t *testing.T) {
	expected := map[string]string{
		"critical": models.PriorityCritical,
		"urgent":   models.PriorityUrgent,
		"normal":   models.PriorityNormal,
		"low":      models.PriorityLow,
	}
	for want, got := range expected {
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Status constants
// ---------------------------------------------------------------------------

func TestFeed_StatusConstants(t *testing.T) {
	expected := map[string]string{
		"active":    models.FeedStatusActive,
		"dismissed": models.FeedStatusDismissed,
		"actioned":  models.FeedStatusActioned,
		"expired":   models.FeedStatusExpired,
	}
	for want, got := range expected {
		if got != want {
			t.Errorf("expected %q, got %q", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// CardType constants
// ---------------------------------------------------------------------------

func TestFeed_CardTypeConstants(t *testing.T) {
	if models.CardTypeBriefing != "daily_briefing" {
		t.Errorf("CardTypeBriefing = %q", models.CardTypeBriefing)
	}
	if models.CardTypeProcurement != "procurement_alert" {
		t.Errorf("CardTypeProcurement = %q", models.CardTypeProcurement)
	}
	if models.CardTypeWeatherAlert != "weather_alert" {
		t.Errorf("CardTypeWeatherAlert = %q", models.CardTypeWeatherAlert)
	}
	if models.CardTypeSubConfirmation != "sub_confirmation" {
		t.Errorf("CardTypeSubConfirmation = %q", models.CardTypeSubConfirmation)
	}
	if models.CardTypeProgress != "progress_update" {
		t.Errorf("CardTypeProgress = %q", models.CardTypeProgress)
	}
	if models.CardTypeAgentApproval != "agent_approval" {
		t.Errorf("CardTypeAgentApproval = %q", models.CardTypeAgentApproval)
	}
}
