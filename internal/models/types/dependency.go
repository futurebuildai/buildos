package types

// DependencyType represents the four CPM dependency relationships.
// See DATA_SPINE_SPEC.md Section 3.4
type DependencyType string

const (
	DependencyTypeFS DependencyType = "FS" // Finish-to-Start
	DependencyTypeSS DependencyType = "SS" // Start-to-Start
	DependencyTypeFF DependencyType = "FF" // Finish-to-Finish
	DependencyTypeSF DependencyType = "SF" // Start-to-Finish
)

// Forecast holds weather data for SWIM model calculations.
// See CPM_RES_MODEL_SPEC.md Section 19.2
type Forecast struct {
	PrecipitationMM float64 `json:"precipitation_mm"`
	LowTempC        float64 `json:"low_temp_c"`
	HighTempC       float64 `json:"high_temp_c"`
}
