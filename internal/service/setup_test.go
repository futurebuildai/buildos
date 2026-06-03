package service

import "testing"

// TestIsShortCode covers the trade-category code validator directly (it's
// pure, so no pool/container is needed). The DB-backed CreateTrade tests
// only ever reach the disallowed-character leg (e.g. "elec/plumbing"); the
// length-bound rejection — len(s) < min or > max — has no caller that can
// exercise both ends, so it's asserted here alongside the accept path.
func TestIsShortCode(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		min, max int
		want     bool
	}{
		{"accepts alphanumeric + underscore + hyphen", "ELEC_01-A", 1, 16, true},
		{"too short", "", 1, 16, false},
		{"too long", "ABCDEFGHIJKLMNOPQ", 1, 16, false}, // 17 > 16
		{"disallowed slash", "ELEC/PLUMB", 1, 16, false},
		{"disallowed lowercase", "elec", 1, 16, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isShortCode(c.in, c.min, c.max); got != c.want {
				t.Errorf("isShortCode(%q, %d, %d) = %v, want %v", c.in, c.min, c.max, got, c.want)
			}
		})
	}
}

// TestLooksLikeRegion covers the relaxed ISO-3166-2 region shape check. The
// jurisdiction wizard step reaches the length-bound rejection, but the
// per-character rejection (a char outside [-A-Za-z0-9]) has no DB-backed
// caller, so it's asserted here.
func TestLooksLikeRegion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"accepts US-CA", "US-CA", true},
		{"accepts lowercase + digit", "ca-on1", true},
		{"too short", "U", false},
		{"too long", "ABCDEFGHI", false}, // 9 > 8
		{"disallowed symbol", "US@CA", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikeRegion(c.in); got != c.want {
				t.Errorf("looksLikeRegion(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestLooksLikeCSICode covers the CSI MasterFormat shape check. The cost-code
// wizard step reaches the segment-count and segment-length rejections, but
// the non-digit-character rejection inside a well-sized segment has no
// DB-backed caller, so it's asserted here alongside the accept paths.
func TestLooksLikeCSICode(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"accepts two segments", "03-30", true},
		{"accepts three segments", "03-30-00", true},
		{"too few segments", "03", false},
		{"too many segments", "03-30-00-10", false},
		{"wrong segment length", "3-30", false},
		{"non-digit in segment", "03-3X", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikeCSICode(c.in); got != c.want {
				t.Errorf("looksLikeCSICode(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
