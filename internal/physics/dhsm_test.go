package physics

import (
	"math"
	"testing"
	"time"

	"github.com/futurebuildai/buildos/internal/config"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/models/types"
)

// almostEqual compares floats within a small tolerance for the SAF power-curve
// cases. Everything else in DHSM is integer-exact and asserted with ==.
func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

func TestCalculateSAF(t *testing.T) {
	def := config.PhysicsConfig{} // exercises WithDefaults (2000 sqft, 0.35)
	const tol = 1e-9

	tests := []struct {
		name string
		gsf  float64
		cfg  config.PhysicsConfig
		want float64
	}{
		{"zero gsf returns unit SAF", 0, def, 1.0},
		{"negative gsf returns unit SAF", -500, def, 1.0},
		{"standard size is exactly unit", 2000, def, 1.0},
		{"double size scales by 2^0.35", 4000, def, math.Pow(2, 0.35)},
		{"half size scales by 0.5^0.35", 1000, def, math.Pow(0.5, 0.35)},
		{"explicit cfg is honored", 3000, config.PhysicsConfig{StandardHouseSizeSF: 3000, SizeAdjustmentExponent: 0.5}, 1.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateSAF(tt.gsf, tt.cfg)
			if !almostEqual(got, tt.want, tol) {
				t.Errorf("CalculateSAF(%v) = %v, want %v", tt.gsf, got, tt.want)
			}
		})
	}
}

func TestCalculateSAF_Monotonic(t *testing.T) {
	def := config.PhysicsConfig{}
	small := CalculateSAF(1500, def)
	mid := CalculateSAF(2000, def)
	large := CalculateSAF(6000, def)
	if !(small < mid && mid < large) {
		t.Errorf("SAF not monotonic in GSF: 1500=%v 2000=%v 6000=%v", small, mid, large)
	}
	if mid != 1.0 {
		t.Errorf("SAF at standard size = %v, want exactly 1.0", mid)
	}
}

func TestCalculateSAFScaled(t *testing.T) {
	def := config.PhysicsConfig{}
	tests := []struct {
		name string
		gsf  float64
		want int64
	}{
		{"standard size scales to 1000", 2000, 1000},
		{"zero gsf scales to 1000", 0, 1000},
		{"double size rounds to 1275", 4000, int64(math.Round(math.Pow(2, 0.35) * SAFScaleFactor))},
		{"half size rounds to 785", 1000, int64(math.Round(math.Pow(0.5, 0.35) * SAFScaleFactor))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateSAFScaled(tt.gsf, def); got != tt.want {
				t.Errorf("CalculateSAFScaled(%v) = %d, want %d", tt.gsf, got, tt.want)
			}
		})
	}
}

func TestGetContextVariable(t *testing.T) {
	ctx := models.ProjectContext{
		SupplyChainVolatility:  3,
		RoughInspectionLatency: 5,
		FinalInspectionLatency: 7,
	}
	tests := []struct {
		key  string
		want float64
	}{
		{"supply_chain_volatility", 3},
		{"rough_inspection_latency", 5},
		{"final_inspection_latency", 7},
		{"unknown_key", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := getContextVariable(ctx, tt.key); got != tt.want {
				t.Errorf("getContextVariable(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestApplyMultiplierFormula(t *testing.T) {
	tests := []struct {
		name    string
		value   float64
		weight  float64
		formula string
		want    float64
	}{
		{"linear", 3, 0.1, "linear", 1.3},
		{"scaled above baseline", 3000, 2, "scaled", 1.2},
		{"scaled at baseline is neutral", 2000, 5, "scaled", 1.0},
		{"step", 3, 0.5, "step", 2.0},
		{"unknown formula falls back to linear", 3, 0.1, "bogus", 1.3},
		{"empty formula falls back to linear", 4, 0.25, "", 2.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyMultiplierFormula(tt.value, tt.weight, tt.formula)
			if got != tt.want {
				t.Errorf("applyMultiplierFormula(%v, %v, %q) = %v, want %v", tt.value, tt.weight, tt.formula, got, tt.want)
			}
		})
	}
}

func TestDurationToDays(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want float64
	}{
		{0, 0},
		{24 * time.Hour, 1.0},
		{12 * time.Hour, 0.5},
		{48 * time.Hour, 2.0},
	}
	for _, tt := range tests {
		if got := DurationToDays(tt.d); got != tt.want {
			t.Errorf("DurationToDays(%v) = %v, want %v", tt.d, got, tt.want)
		}
	}
}

func TestDaysToDuration(t *testing.T) {
	tests := []struct {
		name string
		days float64
		want time.Duration
	}{
		{"whole day is exact", 1.0, 24 * time.Hour},
		{"half day is exact", 0.5, 12 * time.Hour},
		{"14.4 min rounds down to zero", 0.01, 0},
		{"28.8 min rounds up to 30 min", 0.02, 30 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DaysToDuration(tt.days); got != tt.want {
				t.Errorf("DaysToDuration(%v) = %v, want %v", tt.days, got, tt.want)
			}
		})
	}
}

func TestDurationDays_RoundTrip(t *testing.T) {
	// Values already aligned to the 30-min grid survive a round trip exactly.
	for _, days := range []float64{0.5, 1.0, 2.5, 10.0} {
		if got := DurationToDays(DaysToDuration(days)); got != days {
			t.Errorf("round trip %v days = %v", days, got)
		}
	}
}

func TestCalculateTaskDuration(t *testing.T) {
	def := config.PhysicsConfig{}
	benign := types.Forecast{PrecipitationMM: 0, LowTempC: 10, HighTempC: 20}

	t.Run("non-inspection at standard size is unchanged", func(t *testing.T) {
		task := models.WBSTask{Code: "11.0", BaseDurationDays: 2.0}
		got := CalculateTaskDuration(task, 2000, models.ProjectContext{}, nil, benign, def)
		if got != 2.0 {
			t.Errorf("got %v, want 2.0", got)
		}
	})

	t.Run("inspection ignores SAF regardless of GSF", func(t *testing.T) {
		task := models.WBSTask{Code: "20.0", BaseDurationDays: 1.0, IsInspection: true}
		got := CalculateTaskDuration(task, 6000, models.ProjectContext{}, nil, benign, def)
		if got != 1.0 {
			t.Errorf("got %v, want 1.0", got)
		}
	})

	t.Run("result is rounded up to the half day", func(t *testing.T) {
		task := models.WBSTask{Code: "11.0", BaseDurationDays: 2.1}
		got := CalculateTaskDuration(task, 2000, models.ProjectContext{}, nil, benign, def)
		if got != 2.5 {
			t.Errorf("got %v, want 2.5", got)
		}
	})

	t.Run("linear multiplier is applied", func(t *testing.T) {
		task := models.WBSTask{Code: "5.0", BaseDurationDays: 4.0}
		mults := []models.DurationMultiplier{
			{WBSTaskCode: "5.0", VariableKey: "supply_chain_volatility", Weight: 0.1, MultiplierFormula: "linear"},
		}
		ctx := models.ProjectContext{SupplyChainVolatility: 3}
		// 4.0 * 1.0(saf) * 1.3(mult) = 5.2 -> ceil to half day -> 5.5
		got := CalculateTaskDuration(task, 2000, ctx, mults, benign, def)
		if got != 5.5 {
			t.Errorf("got %v, want 5.5", got)
		}
	})

	t.Run("multiplier for a different task code is skipped", func(t *testing.T) {
		task := models.WBSTask{Code: "5.0", BaseDurationDays: 4.0}
		mults := []models.DurationMultiplier{
			{WBSTaskCode: "9.9", VariableKey: "supply_chain_volatility", Weight: 0.1, MultiplierFormula: "linear"},
		}
		ctx := models.ProjectContext{SupplyChainVolatility: 3}
		got := CalculateTaskDuration(task, 2000, ctx, mults, benign, def)
		if got != 4.0 {
			t.Errorf("got %v, want 4.0 (multiplier should not apply)", got)
		}
	})

	t.Run("multiplier with zero context value is skipped", func(t *testing.T) {
		task := models.WBSTask{Code: "5.0", BaseDurationDays: 4.0}
		mults := []models.DurationMultiplier{
			{WBSTaskCode: "*", VariableKey: "supply_chain_volatility", Weight: 0.1, MultiplierFormula: "linear"},
		}
		got := CalculateTaskDuration(task, 2000, models.ProjectContext{}, mults, benign, def)
		if got != 4.0 {
			t.Errorf("got %v, want 4.0 (zero variable should skip)", got)
		}
	})

	t.Run("weather-sensitive WBS applies rain multiplier", func(t *testing.T) {
		task := models.WBSTask{Code: "8.0", BaseDurationDays: 10.0}
		wet := types.Forecast{PrecipitationMM: 20, LowTempC: 10, HighTempC: 20}
		// 10 * 1.15 = 11.5, already on half day
		got := CalculateTaskDuration(task, 2000, models.ProjectContext{}, nil, wet, def)
		if got != 11.5 {
			t.Errorf("got %v, want 11.5", got)
		}
	})
}

func TestCalculateTaskDurationV2(t *testing.T) {
	def := config.PhysicsConfig{}
	benign := types.Forecast{PrecipitationMM: 0, LowTempC: 10, HighTempC: 20}

	t.Run("non-inspection at standard size", func(t *testing.T) {
		task := models.WBSTask{Code: "11.0", BaseDurationDays: 2.0}
		got := CalculateTaskDurationV2(task, 2000, models.ProjectContext{}, nil, benign, def)
		if got != 48*time.Hour {
			t.Errorf("got %v, want 48h", got)
		}
	})

	t.Run("inspection ignores SAF", func(t *testing.T) {
		task := models.WBSTask{Code: "20.0", BaseDurationDays: 1.0, IsInspection: true}
		got := CalculateTaskDurationV2(task, 6000, models.ProjectContext{}, nil, benign, def)
		if got != 24*time.Hour {
			t.Errorf("got %v, want 24h", got)
		}
	})

	t.Run("rounds up to the next half day", func(t *testing.T) {
		// 2.1 days = 50.4h -> next 4h boundary = 52h
		task := models.WBSTask{Code: "11.0", BaseDurationDays: 2.1}
		got := CalculateTaskDurationV2(task, 2000, models.ProjectContext{}, nil, benign, def)
		if got != 52*time.Hour {
			t.Errorf("got %v, want 52h", got)
		}
	})

	t.Run("output is always a multiple of a half day", func(t *testing.T) {
		for _, base := range []float64{0.3, 1.7, 3.33, 9.99} {
			task := models.WBSTask{Code: "11.0", BaseDurationDays: base}
			got := CalculateTaskDurationV2(task, 2000, models.ProjectContext{}, nil, benign, def)
			if got%HalfDay != 0 {
				t.Errorf("base=%v: %v is not a half-day multiple", base, got)
			}
		}
	})
}

func TestCalculateBatchDurations(t *testing.T) {
	def := config.PhysicsConfig{}
	benign := types.Forecast{PrecipitationMM: 0, LowTempC: 10, HighTempC: 20}
	tasks := []models.WBSTask{
		{Code: "11.0", BaseDurationDays: 2.0},
		{Code: "20.0", BaseDurationDays: 1.0, IsInspection: true},
	}

	results := CalculateBatchDurations(tasks, 2000, models.ProjectContext{}, nil, benign, def)
	if len(results) != len(tasks) {
		t.Fatalf("got %d results, want %d", len(results), len(tasks))
	}
	for i, r := range results {
		if r.WBSCode != tasks[i].Code {
			t.Errorf("result[%d].WBSCode = %q, want %q", i, r.WBSCode, tasks[i].Code)
		}
		if r.BaseDuration != tasks[i].BaseDurationDays {
			t.Errorf("result[%d].BaseDuration = %v, want %v", i, r.BaseDuration, tasks[i].BaseDurationDays)
		}
		want := CalculateTaskDuration(tasks[i], 2000, models.ProjectContext{}, nil, benign, def)
		if r.CalculatedDuration != want {
			t.Errorf("result[%d].CalculatedDuration = %v, want %v", i, r.CalculatedDuration, want)
		}
	}
}

func TestCalculateBatchDurationsV2(t *testing.T) {
	def := config.PhysicsConfig{}
	benign := types.Forecast{PrecipitationMM: 0, LowTempC: 10, HighTempC: 20}
	tasks := []models.WBSTask{
		{Code: "11.0", BaseDurationDays: 2.0},
		{Code: "20.0", BaseDurationDays: 1.0, IsInspection: true},
	}

	results := CalculateBatchDurationsV2(tasks, 2000, models.ProjectContext{}, nil, benign, def)
	if len(results) != len(tasks) {
		t.Fatalf("got %d results, want %d", len(results), len(tasks))
	}
	for i, r := range results {
		if r.WBSCode != tasks[i].Code {
			t.Errorf("result[%d].WBSCode = %q, want %q", i, r.WBSCode, tasks[i].Code)
		}
		if r.BaseDuration != DaysToDuration(tasks[i].BaseDurationDays) {
			t.Errorf("result[%d].BaseDuration = %v, want %v", i, r.BaseDuration, DaysToDuration(tasks[i].BaseDurationDays))
		}
		want := CalculateTaskDurationV2(tasks[i], 2000, models.ProjectContext{}, nil, benign, def)
		if r.CalculatedDuration != want {
			t.Errorf("result[%d].CalculatedDuration = %v, want %v", i, r.CalculatedDuration, want)
		}
	}
}
