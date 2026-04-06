package agents

import (
	"context"
	"testing"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/google/uuid"
)

// =============================================================================
// Mock Dependencies for Drift Detection Tests
// =============================================================================

// mockFeedWriter records feed card writes for assertion.
type mockFeedWriter struct {
	cards []*models.FeedCard
	err   error
}

func (m *mockFeedWriter) WriteCard(_ context.Context, card *models.FeedCard) error {
	if m.err != nil {
		return m.err
	}
	card.ID = uuid.New()
	m.cards = append(m.cards, card)
	return nil
}

// mockDriftRepository implements DriftRepository for testing.
// It calls fn once per project with the configured tasks.
type mockDriftRepository struct {
	projects map[uuid.UUID][]CompletedTaskRow
}

func newMockDriftRepo() *mockDriftRepository {
	return &mockDriftRepository{
		projects: make(map[uuid.UUID][]CompletedTaskRow),
	}
}

func (m *mockDriftRepository) StreamCompletedTasksByProject(ctx context.Context, fn func(projectID, orgID uuid.UUID, tasks []CompletedTaskRow) error) error {
	for _, tasks := range m.projects {
		if len(tasks) == 0 {
			continue
		}
		projectID := tasks[0].ProjectID
		orgID := tasks[0].OrgID
		if err := fn(projectID, orgID, tasks); err != nil {
			return err
		}
	}
	return nil
}

// =============================================================================
// Test Helper
// =============================================================================

func makeTasks(projectID, orgID uuid.UUID, count int, ratio float64) []CompletedTaskRow {
	tasks := make([]CompletedTaskRow, count)
	for i := 0; i < count; i++ {
		tasks[i] = CompletedTaskRow{
			TaskID:             uuid.New(),
			ProjectID:          projectID,
			OrgID:              orgID,
			PredictedDuration:  10.0,
			ActualDurationDays: 10.0 * ratio,
		}
	}
	return tasks
}

// =============================================================================
// Tests
// =============================================================================

// TestDriftDetection_NoDriftWhenDeviationUnder25Pct verifies that no drift
// card is emitted when the actual/predicted ratio stays within 25% of 1.0.
func TestDriftDetection_NoDriftWhenDeviationUnder25Pct(t *testing.T) {
	projectID := uuid.New()
	orgID := uuid.New()

	repo := newMockDriftRepo()
	// Ratio of 1.10 = 10% slower, well under 25% threshold
	repo.projects[projectID] = makeTasks(projectID, orgID, 10, 1.10)

	fw := &mockFeedWriter{}
	agent := NewDriftDetectionAgent(repo).WithFeedWriter(fw)

	err := agent.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fw.cards) != 0 {
		t.Errorf("expected 0 cards for 10%% deviation, got %d", len(fw.cards))
	}
}

// TestDriftDetection_NoDriftWhenTooFewTasks verifies no drift card when
// fewer than MinCompletedTasks (8) are completed.
func TestDriftDetection_NoDriftWhenTooFewTasks(t *testing.T) {
	projectID := uuid.New()
	orgID := uuid.New()

	repo := newMockDriftRepo()
	// 5 tasks < MinCompletedTasks(8), even with large drift
	repo.projects[projectID] = makeTasks(projectID, orgID, 5, 2.0)

	fw := &mockFeedWriter{}
	agent := NewDriftDetectionAgent(repo).WithFeedWriter(fw)

	err := agent.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fw.cards) != 0 {
		t.Errorf("expected 0 cards for too few tasks, got %d", len(fw.cards))
	}
}

// TestDriftDetection_EmitsDriftCard_SlowerThanPredicted verifies that a
// drift card is emitted when tasks consistently take >25% longer than predicted
// across the sustained window.
func TestDriftDetection_EmitsDriftCard_SlowerThanPredicted(t *testing.T) {
	projectID := uuid.New()
	orgID := uuid.New()

	repo := newMockDriftRepo()
	// Ratio 1.50 = 50% slower than predicted, sustained across 10 tasks
	repo.projects[projectID] = makeTasks(projectID, orgID, 10, 1.50)

	fw := &mockFeedWriter{}
	agent := NewDriftDetectionAgent(repo).WithFeedWriter(fw)

	err := agent.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fw.cards) != 1 {
		t.Fatalf("expected 1 drift card, got %d", len(fw.cards))
	}

	card := fw.cards[0]
	if card.CardType != CardTypeCalibrationDrift {
		t.Errorf("expected card_type %q, got %q", CardTypeCalibrationDrift, card.CardType)
	}
	if card.Priority != models.PriorityLow {
		t.Errorf("expected priority %q, got %q", models.PriorityLow, card.Priority)
	}
	if card.OrgID != orgID {
		t.Errorf("expected org_id %s, got %s", orgID, card.OrgID)
	}
	if card.ProjectID == nil || *card.ProjectID != projectID {
		t.Errorf("expected project_id %s", projectID)
	}
	if card.Status != models.FeedStatusActive {
		t.Errorf("expected status %q, got %q", models.FeedStatusActive, card.Status)
	}
	if card.AgentSource == nil || *card.AgentSource != "drift_detection" {
		t.Error("expected agent_source 'drift_detection'")
	}
	if card.Title == "" {
		t.Error("expected non-empty title")
	}
	if card.Body == "" {
		t.Error("expected non-empty body")
	}
	if card.Consequence == nil || *card.Consequence == "" {
		t.Error("expected non-empty consequence")
	}
}

// TestDriftDetection_EmitsDriftCard_FasterThanPredicted verifies that a
// drift card is emitted when tasks complete >25% faster than predicted.
func TestDriftDetection_EmitsDriftCard_FasterThanPredicted(t *testing.T) {
	projectID := uuid.New()
	orgID := uuid.New()

	repo := newMockDriftRepo()
	// Ratio 0.50 = 50% faster than predicted
	repo.projects[projectID] = makeTasks(projectID, orgID, 10, 0.50)

	fw := &mockFeedWriter{}
	agent := NewDriftDetectionAgent(repo).WithFeedWriter(fw)

	err := agent.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fw.cards) != 1 {
		t.Fatalf("expected 1 drift card for faster-than-predicted, got %d", len(fw.cards))
	}

	card := fw.cards[0]
	if card.CardType != CardTypeCalibrationDrift {
		t.Errorf("expected card_type %q, got %q", CardTypeCalibrationDrift, card.CardType)
	}
}

// TestDriftDetection_MixedRatiosNoDrift verifies no drift card when recent
// tasks have mixed ratios (some faster, some slower).
func TestDriftDetection_MixedRatiosNoDrift(t *testing.T) {
	projectID := uuid.New()
	orgID := uuid.New()

	repo := newMockDriftRepo()

	// Create 10 tasks with alternating fast/slow ratios
	tasks := make([]CompletedTaskRow, 10)
	for i := 0; i < 10; i++ {
		ratio := 0.50 // fast
		if i%2 == 0 {
			ratio = 1.50 // slow
		}
		tasks[i] = CompletedTaskRow{
			TaskID:             uuid.New(),
			ProjectID:          projectID,
			OrgID:              orgID,
			PredictedDuration:  10.0,
			ActualDurationDays: 10.0 * ratio,
		}
	}
	repo.projects[projectID] = tasks

	fw := &mockFeedWriter{}
	agent := NewDriftDetectionAgent(repo).WithFeedWriter(fw)

	err := agent.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fw.cards) != 0 {
		t.Errorf("expected 0 cards for mixed ratios, got %d", len(fw.cards))
	}
}

// TestDriftDetection_MultipleProjects verifies that drift detection works
// independently for each project.
func TestDriftDetection_MultipleProjects(t *testing.T) {
	orgID := uuid.New()
	projectA := uuid.New()
	projectB := uuid.New()

	repo := newMockDriftRepo()
	// Project A: 50% slower (should emit)
	repo.projects[projectA] = makeTasks(projectA, orgID, 10, 1.50)
	// Project B: on track (should NOT emit)
	repo.projects[projectB] = makeTasks(projectB, orgID, 10, 1.05)

	fw := &mockFeedWriter{}
	agent := NewDriftDetectionAgent(repo).WithFeedWriter(fw)

	err := agent.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fw.cards) != 1 {
		t.Errorf("expected 1 card (only for projectA), got %d", len(fw.cards))
	}
}

// TestDriftDetection_ZeroPredictedDuration verifies division-by-zero safety.
// When predicted_duration is 0 or negative, the ratio defaults to 1.0.
func TestDriftDetection_ZeroPredictedDuration(t *testing.T) {
	projectID := uuid.New()
	orgID := uuid.New()

	repo := newMockDriftRepo()
	tasks := make([]CompletedTaskRow, 10)
	for i := 0; i < 10; i++ {
		tasks[i] = CompletedTaskRow{
			TaskID:             uuid.New(),
			ProjectID:          projectID,
			OrgID:              orgID,
			PredictedDuration:  0.0, // zero — should default to 1.0 ratio
			ActualDurationDays: 5.0,
		}
	}
	repo.projects[projectID] = tasks

	fw := &mockFeedWriter{}
	agent := NewDriftDetectionAgent(repo).WithFeedWriter(fw)

	err := agent.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// With ratio=1.0 for all tasks, no drift should be detected
	if len(fw.cards) != 0 {
		t.Errorf("expected 0 cards for zero predicted duration, got %d", len(fw.cards))
	}
}

// TestDriftDetection_NilFeedWriter verifies an error is returned when
// feedWriter is not configured.
func TestDriftDetection_NilFeedWriter(t *testing.T) {
	repo := newMockDriftRepo()
	agent := NewDriftDetectionAgent(repo) // no WithFeedWriter call

	err := agent.Execute(context.Background())
	if err == nil {
		t.Fatal("expected error when feedWriter is nil")
	}
}

// TestDriftDetection_ExactThreshold verifies boundary behavior at exactly 25%.
func TestDriftDetection_ExactThreshold(t *testing.T) {
	projectID := uuid.New()
	orgID := uuid.New()

	repo := newMockDriftRepo()
	// Ratio 1.25 = exactly at the 25% threshold boundary.
	// The condition is: r <= (1.0 + DriftThreshold) for allSlower.
	// At exactly 1.25, r <= 1.25 is true, so allSlower stays true.
	// But also r >= (1.0 - DriftThreshold) = 0.75, so allFaster becomes false.
	// So allSlower=true, allFaster=false -> drift card should be emitted.
	repo.projects[projectID] = makeTasks(projectID, orgID, 10, 1.25)

	fw := &mockFeedWriter{}
	agent := NewDriftDetectionAgent(repo).WithFeedWriter(fw)

	err := agent.Execute(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// At exactly 1.25, the condition r <= (1.0 + 0.25) is true so allSlower
	// stays true, BUT the code checks if r <= (1.0 + DriftThreshold) means
	// allSlower = false. Let's look at the code:
	// if r <= (1.0 + DriftThreshold) { allSlower = false }
	// At r=1.25: 1.25 <= 1.25 is TRUE -> allSlower = false
	// So at exactly the boundary, NO drift is detected.
	if len(fw.cards) != 0 {
		t.Errorf("expected 0 cards at exact threshold boundary, got %d", len(fw.cards))
	}
}

// TestDriftDetection_Constants verifies the drift detection constants
// match the specification.
func TestDriftDetection_Constants(t *testing.T) {
	if MinCompletedTasks != 8 {
		t.Errorf("expected MinCompletedTasks=8, got %d", MinCompletedTasks)
	}
	if SustainedWindow != 5 {
		t.Errorf("expected SustainedWindow=5, got %d", SustainedWindow)
	}
	if DriftThreshold != 0.25 {
		t.Errorf("expected DriftThreshold=0.25, got %f", DriftThreshold)
	}
	if CardTypeCalibrationDrift != "calibration_drift" {
		t.Errorf("expected CardTypeCalibrationDrift='calibration_drift', got %q", CardTypeCalibrationDrift)
	}
}
