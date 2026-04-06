package physics_test

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/futurebuild/futurebuild-os/internal/config"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/physics"
)

// defaultCfg is the standard config for physics tests
var defaultCfg = config.DefaultPhysicsConfig()

// Scenario definitions matching JSON structure
type Scenario struct {
	ScenarioName     string                      `json:"scenario_name"`
	ProjectGSF       float64                     `json:"project_gsf"`
	ProjectStartDate time.Time                   `json:"project_start_date"`
	Context          ContextConfig               `json:"context"`
	Multipliers      []models.DurationMultiplier `json:"multipliers"`
	Tasks            []TaskInput                 `json:"tasks"`
	ExpectedResults  map[string]ExpectedResult   `json:"expected_results"`
}

type ContextConfig struct {
	SupplyChainVolatility  float64 `json:"supply_chain_volatility"`
	RoughInspectionLatency float64 `json:"rough_inspection_latency"`
	FinalInspectionLatency float64 `json:"final_inspection_latency"`
}

type TaskInput struct {
	ID               uuid.UUID          `json:"id"`
	WBSCode          string             `json:"wbs_code"`
	Description      string             `json:"description"`
	BaseDurationDays float64            `json:"base_duration_days"`
	IsInspection     bool               `json:"is_inspection"`
	Predecessors     []PredecessorInput `json:"predecessors"`
	RawPredecessors  string             `json:"-"` // Internal use for CSV parsing
}

type PredecessorInput struct {
	PredecessorID uuid.UUID `json:"predecessor_id"`
	Type          string    `json:"type"` // FS, SS, FF, SF
	Lag           int       `json:"lag"`
}

type ExpectedResult struct {
	CalculatedDuration float64   `json:"calculated_duration"`
	EarlyStart         time.Time `json:"early_start"`
	EarlyFinish        time.Time `json:"early_finish"`
	LateStart          time.Time `json:"late_start"`
	LateFinish         time.Time `json:"late_finish"`
	TotalFloat         float64   `json:"total_float"`
	IsCritical         bool      `json:"is_critical"`
}

// LoadGoldenMasterScenarios reads the CSV file
func LoadGoldenMasterScenarios(t *testing.T) []Scenario {
	path := filepath.Join("..", "..", "test", "data", "golden_master", "physics_scenarios.csv")
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		t.Skip("Golden master CSV not found at ", path, " -- skipping integration test")
	}
	require.NoError(t, err, "Failed to open scenarios file")
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	require.NoError(t, err, "Failed to read scenarios CSV")

	if len(records) < 2 {
		t.Fatal("CSV file is empty or missing header")
	}

	scenarioMap := make(map[string]*Scenario)
	var orderedScenarios []string // Maintain order

	// Skip header (row 0)
	for _, row := range records[1:] {
		// Columns:
		// 0: ScenarioName
		// 1: ProjectGSF
		// 2: ProjectStartDate
		// 3: SupplyChainVolatility
		// 4: RoughInspectionLatency
		// 5: FinalInspectionLatency
		// 6: TaskWBS
		// 7: TaskDescription
		// 8: IsInspection
		// 9: BaseDurationDays
		// 10: Multipliers (e.g. "Material:1.5|Labor:1.0")
		// 11: Predecessors (e.g. "task-1:FS:0|task-2:SS:2")
		// 12: ExpectedDuration
		// 13: ExpectedES
		// 14: ExpectedEF
		// 15: ExpectedLS
		// 16: ExpectedLF
		// 17: ExpectedFloat
		// 18: ExpectedCritical

		name := row[0]
		if _, exists := scenarioMap[name]; !exists {
			gsf, _ := strconv.ParseFloat(row[1], 64)
			startDate, _ := time.Parse(time.RFC3339, row[2])
			volatility, _ := strconv.ParseFloat(row[3], 64)
			roughLat, _ := strconv.ParseFloat(row[4], 64)
			finalLat, _ := strconv.ParseFloat(row[5], 64)

			scenarioMap[name] = &Scenario{
				ScenarioName:     name,
				ProjectGSF:       gsf,
				ProjectStartDate: startDate,
				Context: ContextConfig{
					SupplyChainVolatility:  volatility,
					RoughInspectionLatency: roughLat,
					FinalInspectionLatency: finalLat,
				},
				Tasks:           []TaskInput{},
				ExpectedResults: make(map[string]ExpectedResult),
			}
			orderedScenarios = append(orderedScenarios, name)
		}

		sc := scenarioMap[name]

		// Parse Task
		taskID := uuid.New() // Generate a new ID for the test run
		wbs := row[6]
		desc := row[7]
		isInsp, _ := strconv.ParseBool(row[8])
		baseDur, _ := strconv.ParseFloat(row[9], 64)

		// Parse Multipliers
		var multipliers []models.DurationMultiplier
		if row[10] != "" {
			parts := strings.Split(row[10], "|")
			for _, part := range parts {
				kv := strings.Split(part, ":")
				if len(kv) == 2 {
					val, _ := strconv.ParseFloat(kv[1], 64)
					multipliers = append(multipliers, models.DurationMultiplier{
						WBSTaskCode:       wbs,
						VariableKey:       kv[0],
						Weight:            val,
						MultiplierFormula: "linear",
					})
				}
			}
		}
		sc.Multipliers = append(sc.Multipliers, multipliers...)

		// Parse Expected Results
		expDur, _ := strconv.ParseFloat(row[12], 64)
		expES, _ := time.Parse(time.RFC3339, row[13])
		expEF, _ := time.Parse(time.RFC3339, row[14])
		expLS, _ := time.Parse(time.RFC3339, row[15])
		expLF, _ := time.Parse(time.RFC3339, row[16])
		expFloat, _ := strconv.ParseFloat(row[17], 64)
		expCrit, _ := strconv.ParseBool(row[18])

		sc.ExpectedResults[wbs] = ExpectedResult{
			CalculatedDuration: expDur,
			EarlyStart:         expES,
			EarlyFinish:        expEF,
			LateStart:          expLS,
			LateFinish:         expLF,
			TotalFloat:         expFloat,
			IsCritical:         expCrit,
		}

		// Store Task Input
		sc.Tasks = append(sc.Tasks, TaskInput{
			ID:               taskID,
			WBSCode:          wbs,
			Description:      desc,
			BaseDurationDays: baseDur,
			IsInspection:     isInsp,
			// Store raw string for now, resolve later
			RawPredecessors: row[11],
		})
	}

	var finalScenarios []Scenario
	for _, name := range orderedScenarios {
		sc := scenarioMap[name]
		// Resolve Predecessors
		// 1. Map WBS -> UUID
		wbsToID := make(map[string]uuid.UUID)
		for _, t := range sc.Tasks {
			wbsToID[t.WBSCode] = t.ID
		}

		// 2. Parse RawPredecessors
		for i, t := range sc.Tasks {
			if t.RawPredecessors == "" {
				continue
			}
			preds := strings.Split(t.RawPredecessors, "|")
			for _, p := range preds {
				// Format: WBS:Type:Lag
				parts := strings.Split(p, ":")
				if len(parts) != 3 {
					t.RawPredecessors = fmt.Sprintf("Error parsing predecessors for task %s row %d: invalid format %s", t.WBSCode, i, p) // Debug info
					continue
				}
				predWBS := parts[0]
				depType := parts[1]
				lag, _ := strconv.Atoi(parts[2])

				predID, ok := wbsToID[predWBS]
				if !ok {
					continue // Or handle error
				}

				sc.Tasks[i].Predecessors = append(sc.Tasks[i].Predecessors, PredecessorInput{
					PredecessorID: predID,
					Type:          depType,
					Lag:           lag,
				})
			}
		}
		finalScenarios = append(finalScenarios, *sc)
	}

	return finalScenarios
}

func parseDependencyType(t string) models.DependencyType {
	switch t {
	case "FS":
		return models.DependencyTypeFS
	case "SS":
		return models.DependencyTypeSS
	case "FF":
		return models.DependencyTypeFF
	case "SF":
		return models.DependencyTypeSF
	default:
		return models.DependencyTypeFS
	}
}

// ContinuousCalendar treats all days as working days (7-day week).
// Used ONLY for Golden Master verification against legacy CSV data.
type ContinuousCalendar struct{}

// AddWorkingDays adds the specified number of working days to a date.
// Since this is a continuous calendar, it simply adds 24 hours per day
// without skipping weekends or holidays.
func (c *ContinuousCalendar) AddWorkingDays(date time.Time, days float64) time.Time {
	return date.Add(time.Duration(days*24) * time.Hour).Truncate(time.Minute)
}

// AddWorkDuration adds work duration using integer math (continuous 7-day week).
func (c *ContinuousCalendar) AddWorkDuration(date time.Time, duration time.Duration) time.Time {
	return date.Add(duration).Truncate(time.Minute)
}

func TestGoldenMasterPhysics(t *testing.T) {
	scenarios := LoadGoldenMasterScenarios(t)

	for _, scenario := range scenarios {
		t.Run(scenario.ScenarioName, func(t *testing.T) {
			// 1. Setup Context
			ctx := models.ProjectContext{
				SupplyChainVolatility:  int(scenario.Context.SupplyChainVolatility),
				RoughInspectionLatency: int(scenario.Context.RoughInspectionLatency),
				FinalInspectionLatency: int(scenario.Context.FinalInspectionLatency),
			}

			// 2. Process Tasks (DHSM Calculation)
			var projectTasks []models.ProjectTask
			var deps []models.TaskDependency

			// Helper to map UUID -> TaskInput for ease
			taskInputMap := make(map[uuid.UUID]TaskInput)
			for _, ti := range scenario.Tasks {
				taskInputMap[ti.ID] = ti
			}

			for _, taskIn := range scenario.Tasks {
				// Convert to WBSTask for DHSM
				wbsTask := models.WBSTask{
					Code:             taskIn.WBSCode,
					BaseDurationDays: taskIn.BaseDurationDays,
					IsInspection:     taskIn.IsInspection,
				}

				// Run DHSM
				calcDuration := physics.CalculateTaskDuration(
					wbsTask,
					scenario.ProjectGSF,
					ctx,
					scenario.Multipliers,
					models.Forecast{}, // Assuming no weather for baseline unless specified
					defaultCfg,
				)

				// Verify DHSM Result immediately
				expected, ok := scenario.ExpectedResults[taskIn.WBSCode]
				require.True(t, ok, "No expected result for task %s", taskIn.WBSCode)

				assert.InDelta(t, expected.CalculatedDuration, calcDuration, 0.01,
					"DHSM duration mismatch for task %s", taskIn.WBSCode)

				// Create ProjectTask for CPM
				pTask := models.ProjectTask{
					ID:                 taskIn.ID,
					WBSCode:            taskIn.WBSCode,
					CalculatedDuration: calcDuration,
				}
				projectTasks = append(projectTasks, pTask)

				// Create Dependencies
				for _, pred := range taskIn.Predecessors {
					dep := models.TaskDependency{
						PredecessorID:  pred.PredecessorID,
						SuccessorID:    taskIn.ID,
						DependencyType: parseDependencyType(pred.Type),
						LagDays:        pred.Lag,
					}
					deps = append(deps, dep)
				}
			}

			// 3. Run CPM (Forward & Backward Pass)
			dag := physics.BuildDependencyGraph(projectTasks, deps)

			// Detect Cycles
			err := physics.DetectCycle(dag)
			assert.NoError(t, err, "Cycle detected in scenario %s", scenario.ScenarioName)

			// Forward Pass
			// Use ContinuousCalendar (7-day week) to match legacy CSV golden master data
			cal := &ContinuousCalendar{}
			schedule, err := physics.ForwardPass(dag, scenario.ProjectStartDate, cal, nil)
			require.NoError(t, err, "Forward pass failed")

			// Backward Pass
			_, err = physics.BackwardPass(dag, schedule, cal, nil)
			require.NoError(t, err, "Backward pass failed")

			// 4. Assert Final Results
			for _, taskSched := range schedule {
				expected, ok := scenario.ExpectedResults[taskSched.WBSCode]
				require.True(t, ok)

				// Check Dates (ES, EF, LS, LF)
				// Using 1 minute tolerance for float math issues
				assert.WithinDuration(t, expected.EarlyStart, taskSched.EarlyStart, time.Minute,
					"Early Start mismatch for task %s", taskSched.WBSCode)
				assert.WithinDuration(t, expected.EarlyFinish, taskSched.EarlyFinish, time.Minute,
					"Early Finish mismatch for task %s", taskSched.WBSCode)
				assert.WithinDuration(t, expected.LateStart, taskSched.LateStart, time.Minute,
					"Late Start mismatch for task %s", taskSched.WBSCode)
				assert.WithinDuration(t, expected.LateFinish, taskSched.LateFinish, time.Minute,
					"Late Finish mismatch for task %s", taskSched.WBSCode)

				// Check Float
				assert.InDelta(t, expected.TotalFloat, taskSched.TotalFloat, 0.01,
					"Total Float mismatch for task %s", taskSched.WBSCode)

				// Check Criticality
				assert.Equal(t, expected.IsCritical, taskSched.IsCritical,
					"Criticality mismatch for task %s", taskSched.WBSCode)
			}
		})
	}
}
