package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Sentinel errors specific to FieldService.
var (
	// ErrIdempotencyConflict surfaces when an insert hits the
	// UNIQUE(idempotency_key) constraint. The handler maps it to 409;
	// the mobile client treats 409 as "already accepted" and clears
	// the row from its outbox.
	ErrIdempotencyConflict = errors.New("field: idempotency key already used")
)

// MaxFieldNotesLength caps free-text fields the mobile client sends
// (notes, weather_conditions, work_summary, safety_incidents). 4 KiB
// is generous for end-of-day summaries; more than that and the
// content belongs in attachments.
const MaxFieldNotesLength = 4096

// ReportProgressInput is the validated input for /field/progress.
type ReportProgressInput struct {
	TaskID          uuid.UUID
	PercentComplete int
	Notes           *string
	PhotoAssetID    *uuid.UUID
	GPSLat          *float64
	GPSLng          *float64
	IdempotencyKey  uuid.UUID
}

// CheckinInput is the validated input for /field/checkin.
type CheckinInput struct {
	ProjectID      uuid.UUID
	CrewMembers    json.RawMessage
	GPSLat         *float64
	GPSLng         *float64
	Notes          *string
	IdempotencyKey uuid.UUID
}

// DailyLogInput is the validated input for /field/daily-log.
type DailyLogInput struct {
	ProjectID         uuid.UUID
	LogDate           time.Time
	WeatherConditions *string
	WorkSummary       string
	SafetyIncidents   *string
	PhotoAssetIDs     []uuid.UUID
	IdempotencyKey    uuid.UUID
}

// FieldService is the business-logic surface for /api/v1/field/*.
// Sync reads, the three POST paths write idempotently with
// client-supplied UUID keys.
type FieldService struct {
	pool      *pgxpool.Pool
	store     *store.FieldStore
	feedStore *store.FeedCardsStore
	audit     AuditRecorder
}

// NewFieldService creates a service bound to a pool, the FieldStore,
// and a FeedCardsStore (for /field/sync to bundle recent cards).
// audit may be nil; nil falls back to a no-op recorder.
func NewFieldService(pool *pgxpool.Pool, fields *store.FieldStore, feed *store.FeedCardsStore, audit AuditRecorder) *FieldService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &FieldService{pool: pool, store: fields, feedStore: feed, audit: audit}
}

// SyncOptions narrows a /field/sync read.
type SyncOptions struct {
	CallerOrgID       uuid.UUID
	CallerOIDCSubject string
	CallerRole        string
	Since             time.Time // zero = full pull
}

// Sync returns the data the mobile client needs to refresh its local
// state: open tasks assigned to the caller, recent active feed cards
// targeted to the caller (by user_id or role), and a server_time
// stamp the client passes back as `?since=` on the next sync.
//
// We capture server_time BEFORE the reads so a row inserted between
// the read and the response timestamp isn't permanently skipped on
// the next pull. This is a "read-your-writes safe" delta pattern.
func (s *FieldService) Sync(ctx context.Context, opts SyncOptions) (models.FieldSyncResponse, error) {
	if opts.CallerOrgID == uuid.Nil {
		return models.FieldSyncResponse{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if opts.CallerOIDCSubject == "" {
		return models.FieldSyncResponse{}, fmt.Errorf("%w: caller oidc subject is required", ErrInvalidInput)
	}

	serverTime := time.Now().UTC()

	var resp models.FieldSyncResponse
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		userID, err := s.store.LookupUserIDBySubject(ctx, tx, opts.CallerOIDCSubject, opts.CallerOrgID)
		if err != nil {
			return err
		}

		tasks, err := s.store.ListAssignedTasks(ctx, tx, store.ListAssignedTasksParams{
			UserID: userID,
			OrgID:  opts.CallerOrgID,
			Since:  opts.Since,
		})
		if err != nil {
			return err
		}
		resp.Tasks = tasks

		// Feed cards land alongside tasks when the caller has a role
		// (the targeting query needs both subject and role).
		if opts.CallerRole != "" {
			fres, err := s.feedStore.ListFeedCards(ctx, tx, store.ListFeedCardsParams{
				OrgID:             opts.CallerOrgID,
				CallerOIDCSubject: opts.CallerOIDCSubject,
				CallerRole:        opts.CallerRole,
				StatusFilter:      []string{"active"},
				Limit:             50,
			})
			if err != nil {
				return err
			}
			resp.FeedCards = fres.Cards
		} else {
			resp.FeedCards = []models.FeedCard{}
		}

		// Equipment allocated to the caller's active sites (Phase 4a-ii,
		// read-only). FULL-SET — ignores Since (see FieldEquipment doc).
		equip, err := s.store.ListAllocatedEquipment(ctx, tx, store.ListAllocatedEquipmentParams{
			UserID: userID,
			OrgID:  opts.CallerOrgID,
			Today:  serverTime,
		})
		if err != nil {
			return err
		}
		resp.Equipment = equip
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return models.FieldSyncResponse{}, ErrNotFound
		}
		return models.FieldSyncResponse{}, fmt.Errorf("field sync: %w", err)
	}
	resp.ServerTime = serverTime
	return resp, nil
}

// ReportProgress writes a task_progress row. The reported_via channel
// is set to "mobile" — the only path through this service is the
// mobile API. Web-side progress reports come through a different
// service entirely (Sprint 6+ scheduler UI).
func (s *FieldService) ReportProgress(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject string, in ReportProgressInput) (models.TaskProgress, error) {
	if err := requireFieldCaller(callerOrgID, callerOIDCSubject); err != nil {
		return models.TaskProgress{}, err
	}
	if in.TaskID == uuid.Nil {
		return models.TaskProgress{}, fmt.Errorf("%w: task_id is required", ErrInvalidInput)
	}
	if in.IdempotencyKey == uuid.Nil {
		return models.TaskProgress{}, fmt.Errorf("%w: idempotency_key is required", ErrInvalidInput)
	}
	if in.PercentComplete < 0 || in.PercentComplete > 100 {
		return models.TaskProgress{}, fmt.Errorf("%w: percent_complete must be 0-100", ErrInvalidInput)
	}
	if err := validateOptionalNotes(in.Notes); err != nil {
		return models.TaskProgress{}, err
	}
	if err := validateGPS(in.GPSLat, in.GPSLng); err != nil {
		return models.TaskProgress{}, err
	}

	var got models.TaskProgress
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		userID, err := s.store.LookupUserIDBySubject(ctx, tx, callerOIDCSubject, callerOrgID)
		if err != nil {
			return err
		}
		if err := s.store.VerifyTaskInOrg(ctx, tx, in.TaskID, callerOrgID); err != nil {
			return err
		}
		row, err := s.store.ReportProgress(ctx, tx, store.ReportProgressParams{
			TaskID:          in.TaskID,
			ReportedBy:      userID,
			PercentComplete: in.PercentComplete,
			Notes:           in.Notes,
			PhotoAssetID:    in.PhotoAssetID,
			GPSLat:          in.GPSLat,
			GPSLng:          in.GPSLng,
			ReportedVia:     models.ReportedViaMobile,
			IdempotencyKey:  in.IdempotencyKey,
		})
		if err != nil {
			return err
		}
		got = row
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerOIDCSubject,
			Action:       "field.task_progress.reported",
			ResourceType: AuditResourceTaskProgress,
			ResourceID:   row.ID,
			After:        marshalAudit(row),
			Metadata: marshalAudit(map[string]any{
				"task_id":          in.TaskID,
				"percent_complete": in.PercentComplete,
				"reported_via":     models.ReportedViaMobile,
			}),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrIdempotencyConflict):
			return models.TaskProgress{}, ErrIdempotencyConflict
		case errors.Is(err, store.ErrNotFound):
			return models.TaskProgress{}, ErrNotFound
		}
		return models.TaskProgress{}, fmt.Errorf("report progress: %w", err)
	}
	return got, nil
}

// Checkin writes a crew_checkins row scoped to the caller's org.
func (s *FieldService) Checkin(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject string, in CheckinInput) (models.CrewCheckin, error) {
	if err := requireFieldCaller(callerOrgID, callerOIDCSubject); err != nil {
		return models.CrewCheckin{}, err
	}
	if in.ProjectID == uuid.Nil {
		return models.CrewCheckin{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if in.IdempotencyKey == uuid.Nil {
		return models.CrewCheckin{}, fmt.Errorf("%w: idempotency_key is required", ErrInvalidInput)
	}
	if err := validateOptionalNotes(in.Notes); err != nil {
		return models.CrewCheckin{}, err
	}
	if err := validateGPS(in.GPSLat, in.GPSLng); err != nil {
		return models.CrewCheckin{}, err
	}

	var got models.CrewCheckin
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		userID, err := s.store.LookupUserIDBySubject(ctx, tx, callerOIDCSubject, callerOrgID)
		if err != nil {
			return err
		}
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
			return err
		}
		row, err := s.store.Checkin(ctx, tx, store.CheckinParams{
			OrgID:          callerOrgID,
			ProjectID:      in.ProjectID,
			ReportedBy:     userID,
			CrewMembers:    in.CrewMembers,
			GPSLat:         in.GPSLat,
			GPSLng:         in.GPSLng,
			Notes:          in.Notes,
			IdempotencyKey: in.IdempotencyKey,
		})
		if err != nil {
			return err
		}
		got = row
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerOIDCSubject,
			Action:       "field.crew_checkin.recorded",
			ResourceType: AuditResourceCrewCheckin,
			ResourceID:   row.ID,
			After:        marshalAudit(row),
			Metadata: marshalAudit(map[string]any{
				"project_id": in.ProjectID,
			}),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrIdempotencyConflict):
			return models.CrewCheckin{}, ErrIdempotencyConflict
		case errors.Is(err, store.ErrNotFound):
			return models.CrewCheckin{}, ErrNotFound
		}
		return models.CrewCheckin{}, fmt.Errorf("checkin: %w", err)
	}
	return got, nil
}

// DailyLog writes a daily_logs row scoped to the caller's org.
func (s *FieldService) DailyLog(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject string, in DailyLogInput) (models.DailyLog, error) {
	if err := requireFieldCaller(callerOrgID, callerOIDCSubject); err != nil {
		return models.DailyLog{}, err
	}
	if in.ProjectID == uuid.Nil {
		return models.DailyLog{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if in.IdempotencyKey == uuid.Nil {
		return models.DailyLog{}, fmt.Errorf("%w: idempotency_key is required", ErrInvalidInput)
	}
	if in.LogDate.IsZero() {
		return models.DailyLog{}, fmt.Errorf("%w: log_date is required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.WorkSummary) == "" {
		return models.DailyLog{}, fmt.Errorf("%w: work_summary is required", ErrInvalidInput)
	}
	if len(in.WorkSummary) > MaxFieldNotesLength {
		return models.DailyLog{}, fmt.Errorf("%w: work_summary exceeds %d bytes", ErrInvalidInput, MaxFieldNotesLength)
	}
	if err := validateOptionalNotes(in.WeatherConditions); err != nil {
		return models.DailyLog{}, err
	}
	if err := validateOptionalNotes(in.SafetyIncidents); err != nil {
		return models.DailyLog{}, err
	}

	var got models.DailyLog
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		userID, err := s.store.LookupUserIDBySubject(ctx, tx, callerOIDCSubject, callerOrgID)
		if err != nil {
			return err
		}
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, callerOrgID); err != nil {
			return err
		}
		row, err := s.store.DailyLog(ctx, tx, store.DailyLogParams{
			OrgID:             callerOrgID,
			ProjectID:         in.ProjectID,
			ReportedBy:        userID,
			LogDate:           in.LogDate,
			WeatherConditions: in.WeatherConditions,
			WorkSummary:       in.WorkSummary,
			SafetyIncidents:   in.SafetyIncidents,
			PhotoAssetIDs:     in.PhotoAssetIDs,
			IdempotencyKey:    in.IdempotencyKey,
		})
		if err != nil {
			return err
		}
		got = row
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerOIDCSubject,
			Action:       "field.daily_log.recorded",
			ResourceType: AuditResourceDailyLog,
			ResourceID:   row.ID,
			After:        marshalAudit(row),
			Metadata: marshalAudit(map[string]any{
				"project_id": in.ProjectID,
				"log_date":   in.LogDate,
			}),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrIdempotencyConflict):
			return models.DailyLog{}, ErrIdempotencyConflict
		case errors.Is(err, store.ErrNotFound):
			return models.DailyLog{}, ErrNotFound
		}
		return models.DailyLog{}, fmt.Errorf("daily log: %w", err)
	}
	return got, nil
}

// requireFieldCaller is the shared "caller has identity" gate used by
// the three POST paths.
func requireFieldCaller(orgID uuid.UUID, oidcSubject string) error {
	if orgID == uuid.Nil {
		return fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if oidcSubject == "" {
		return fmt.Errorf("%w: caller oidc subject is required", ErrInvalidInput)
	}
	return nil
}

// validateOptionalNotes caps a *string field at MaxFieldNotesLength.
// nil and empty are both fine.
func validateOptionalNotes(s *string) error {
	if s == nil {
		return nil
	}
	if len(*s) > MaxFieldNotesLength {
		return fmt.Errorf("%w: text exceeds %d bytes", ErrInvalidInput, MaxFieldNotesLength)
	}
	return nil
}

// validateGPS rejects out-of-range coordinates if either is supplied.
// Both must be set together — a half-fix is a client bug worth
// surfacing rather than silently dropping.
func validateGPS(lat, lng *float64) error {
	if lat == nil && lng == nil {
		return nil
	}
	if (lat == nil) != (lng == nil) {
		return fmt.Errorf("%w: gps_lat and gps_lng must be supplied together", ErrInvalidInput)
	}
	if *lat < -90 || *lat > 90 {
		return fmt.Errorf("%w: gps_lat out of range", ErrInvalidInput)
	}
	if *lng < -180 || *lng > 180 {
		return fmt.Errorf("%w: gps_lng out of range", ErrInvalidInput)
	}
	return nil
}
