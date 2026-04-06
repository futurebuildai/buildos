package service

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewFieldSyncService_NotNil(t *testing.T) {
	svc := NewFieldSyncService(nil)
	if svc == nil {
		t.Fatal("expected non-nil FieldSyncService")
	}
	if svc.store != nil {
		t.Error("expected nil store when created with nil")
	}
}

// ---------------------------------------------------------------------------
// ReportProgress — idempotency key validation
// ---------------------------------------------------------------------------

func TestFieldSync_ReportProgress_MissingKey(t *testing.T) {
	svc := NewFieldSyncService(nil)
	_, err := svc.ReportProgress(nil, &models.FieldProgress{
		IdempotencyKey: "",
		ProjectID:      uuid.New(),
		TaskID:         uuid.New(),
		UserID:         uuid.New(),
		PercentComplete: 50,
	})
	if !errors.Is(err, ErrMissingIdempotencyKey) {
		t.Errorf("expected ErrMissingIdempotencyKey, got: %v", err)
	}
}

func TestFieldSync_ReportProgress_WithKey_ReachesStore(t *testing.T) {
	svc := NewFieldSyncService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.ReportProgress(nil, &models.FieldProgress{
			IdempotencyKey:  "abc-123-def",
			ProjectID:       uuid.New(),
			TaskID:          uuid.New(),
			UserID:          uuid.New(),
			PercentComplete: 75,
		})
	}()
	if !panicked {
		t.Error("expected panic from nil store after passing idempotency validation")
	}
}

func TestFieldSync_ReportProgress_EmptyWhitespaceKey(t *testing.T) {
	// The service only checks for empty string, not whitespace-only.
	// A whitespace key passes validation and reaches the store.
	svc := NewFieldSyncService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.ReportProgress(nil, &models.FieldProgress{
			IdempotencyKey: "   ",
			ProjectID:      uuid.New(),
			TaskID:         uuid.New(),
			UserID:         uuid.New(),
		})
	}()
	if !panicked {
		t.Error("expected panic from nil store for whitespace-only key (passes empty check)")
	}
}

// ---------------------------------------------------------------------------
// Checkin — idempotency key validation
// ---------------------------------------------------------------------------

func TestFieldSync_Checkin_MissingKey(t *testing.T) {
	svc := NewFieldSyncService(nil)
	_, err := svc.Checkin(nil, &models.FieldCheckin{
		IdempotencyKey: "",
		UserID:         uuid.New(),
		ProjectID:      uuid.New(),
		Latitude:       45.0,
		Longitude:      -73.0,
	})
	if !errors.Is(err, ErrMissingIdempotencyKey) {
		t.Errorf("expected ErrMissingIdempotencyKey, got: %v", err)
	}
}

func TestFieldSync_Checkin_WithKey_ReachesStore(t *testing.T) {
	svc := NewFieldSyncService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.Checkin(nil, &models.FieldCheckin{
			IdempotencyKey: "checkin-key-123",
			UserID:         uuid.New(),
			ProjectID:      uuid.New(),
			Latitude:       45.5,
			Longitude:      -73.5,
		})
	}()
	if !panicked {
		t.Error("expected panic from nil store after passing idempotency validation")
	}
}

// ---------------------------------------------------------------------------
// DailyLog — idempotency key validation
// ---------------------------------------------------------------------------

func TestFieldSync_DailyLog_MissingKey(t *testing.T) {
	svc := NewFieldSyncService(nil)
	_, err := svc.DailyLog(nil, &models.DailyLog{
		IdempotencyKey: "",
		UserID:         uuid.New(),
		ProjectID:      uuid.New(),
		LogDate:        time.Now().UTC(),
		Summary:        "Work summary",
		HoursWorked:    8.0,
	})
	if !errors.Is(err, ErrMissingIdempotencyKey) {
		t.Errorf("expected ErrMissingIdempotencyKey, got: %v", err)
	}
}

func TestFieldSync_DailyLog_WithKey_ReachesStore(t *testing.T) {
	svc := NewFieldSyncService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.DailyLog(nil, &models.DailyLog{
			IdempotencyKey: "daily-log-key-456",
			UserID:         uuid.New(),
			ProjectID:      uuid.New(),
			LogDate:        time.Now().UTC(),
			Summary:        "Completed framing",
			HoursWorked:    8.5,
			WeatherNotes:   "Clear",
			SafetyNotes:    "No incidents",
		})
	}()
	if !panicked {
		t.Error("expected panic from nil store after passing idempotency validation")
	}
}

// ---------------------------------------------------------------------------
// Sync — reaches store
// ---------------------------------------------------------------------------

func TestFieldSync_Sync_ReachesStore(t *testing.T) {
	svc := NewFieldSyncService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.Sync(nil, uuid.New(), uuid.New(), "foreman", time.Now().Add(-24*time.Hour))
	}()
	if !panicked {
		t.Error("expected panic from nil store on Sync")
	}
}

// ---------------------------------------------------------------------------
// Sentinel error messages
// ---------------------------------------------------------------------------

func TestFieldSync_SentinelErrors(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{ErrDuplicateIdempotencyKey, "duplicate idempotency key"},
		{ErrMissingIdempotencyKey, "idempotency key is required"},
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
// Error identity — errors.Is
// ---------------------------------------------------------------------------

func TestFieldSync_ErrorIdentity(t *testing.T) {
	if errors.Is(ErrDuplicateIdempotencyKey, ErrMissingIdempotencyKey) {
		t.Error("ErrDuplicateIdempotencyKey should not be ErrMissingIdempotencyKey")
	}
	if !errors.Is(ErrDuplicateIdempotencyKey, ErrDuplicateIdempotencyKey) {
		t.Error("ErrDuplicateIdempotencyKey should be itself")
	}
	if !errors.Is(ErrMissingIdempotencyKey, ErrMissingIdempotencyKey) {
		t.Error("ErrMissingIdempotencyKey should be itself")
	}
}

// ---------------------------------------------------------------------------
// Model struct field validation
// ---------------------------------------------------------------------------

func TestFieldSync_FieldProgressModel(t *testing.T) {
	p := models.FieldProgress{
		ID:              uuid.New(),
		ProjectID:       uuid.New(),
		TaskID:          uuid.New(),
		UserID:          uuid.New(),
		PercentComplete: 100,
		Notes:           "Complete",
		IdempotencyKey:  "key-001",
	}
	if p.PercentComplete != 100 {
		t.Errorf("PercentComplete = %d, want 100", p.PercentComplete)
	}
	if p.IdempotencyKey != "key-001" {
		t.Errorf("IdempotencyKey = %q, want %q", p.IdempotencyKey, "key-001")
	}
}

func TestFieldSync_FieldCheckinModel(t *testing.T) {
	c := models.FieldCheckin{
		ID:             uuid.New(),
		UserID:         uuid.New(),
		ProjectID:      uuid.New(),
		Latitude:       45.508888,
		Longitude:      -73.561668,
		IdempotencyKey: "checkin-002",
	}
	if c.Latitude != 45.508888 {
		t.Errorf("Latitude = %f, want 45.508888", c.Latitude)
	}
	if c.Longitude != -73.561668 {
		t.Errorf("Longitude = %f, want -73.561668", c.Longitude)
	}
}

func TestFieldSync_DailyLogModel(t *testing.T) {
	dl := models.DailyLog{
		ID:             uuid.New(),
		UserID:         uuid.New(),
		ProjectID:      uuid.New(),
		LogDate:        time.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC),
		Summary:        "Framing complete on north wall",
		HoursWorked:    9.5,
		WeatherNotes:   "Overcast, 15C",
		SafetyNotes:    "Harness inspection passed",
		IdempotencyKey: "daily-003",
	}
	if dl.HoursWorked != 9.5 {
		t.Errorf("HoursWorked = %f, want 9.5", dl.HoursWorked)
	}
	if dl.Summary != "Framing complete on north wall" {
		t.Errorf("Summary mismatch")
	}
}

func TestFieldSync_SyncPayloadModel(t *testing.T) {
	sp := models.SyncPayload{
		FeedCards: []models.FeedCard{},
		Tasks:     []models.SyncTask{},
		SyncedAt:  "2026-04-06T12:00:00Z",
	}
	if len(sp.FeedCards) != 0 {
		t.Errorf("expected 0 feed cards, got %d", len(sp.FeedCards))
	}
	if len(sp.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(sp.Tasks))
	}
	if sp.SyncedAt != "2026-04-06T12:00:00Z" {
		t.Errorf("SyncedAt = %q", sp.SyncedAt)
	}
}

func TestFieldSync_SyncTaskModel(t *testing.T) {
	dueDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	st := models.SyncTask{
		ID:              uuid.New(),
		ProjectID:       uuid.New(),
		Name:            "Pour foundation",
		Status:          "in_progress",
		PercentComplete: 60,
		DueDate:         &dueDate,
		UpdatedAt:       time.Now().UTC(),
	}
	if st.PercentComplete != 60 {
		t.Errorf("PercentComplete = %d, want 60", st.PercentComplete)
	}
	if st.Name != "Pour foundation" {
		t.Errorf("Name = %q", st.Name)
	}
	if st.DueDate == nil {
		t.Error("expected non-nil DueDate")
	}
}
