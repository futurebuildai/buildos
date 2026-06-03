package main

import "testing"

func TestExtractVersion(t *testing.T) {
	cases := map[string]string{
		"migrations/001_initial_schema.up.sql":  "001",
		"migrations/010_setup_infra.down.sql":   "010",
		"/abs/path/013_standalone_pivot.up.sql": "013",
		"900_create_widget.up.sql":              "900",
		"nounderscore.sql":                      "nounderscore.sql",
	}
	for path, want := range cases {
		if got := extractVersion(path); got != want {
			t.Errorf("extractVersion(%q) = %q, want %q", path, got, want)
		}
	}
}
