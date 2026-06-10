package main

import "testing"

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantDir string
		wantDry bool
	}{
		{"no args defaults to up", nil, "up", false},
		{"explicit up", []string{"up"}, "up", false},
		{"explicit down", []string{"down"}, "down", false},
		// The footgun: --dry-run must NOT be taken as the direction (which would
		// have fallen into the DOWN/rollback branch under the old positional parse).
		{"bare --dry-run stays up", []string{"--dry-run"}, "up", true},
		{"short -n", []string{"-n"}, "up", true},
		{"up then flag", []string{"up", "--dry-run"}, "up", true},
		{"flag then up", []string{"--dry-run", "up"}, "up", true},
		{"down dry-run", []string{"down", "--dry-run"}, "down", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, dry := parseArgs(c.args)
			if dir != c.wantDir || dry != c.wantDry {
				t.Errorf("parseArgs(%v) = (%q, %v), want (%q, %v)", c.args, dir, dry, c.wantDir, c.wantDry)
			}
		})
	}
}

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
