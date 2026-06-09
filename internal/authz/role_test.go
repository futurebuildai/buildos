package authz

import "testing"

// TestRoleRank pins the exact privilege ladder. Higher rank = more privileged,
// unknown roles return 0.
func TestRoleRank(t *testing.T) {
	tests := []struct {
		role string
		want int
	}{
		{RoleFieldWorker, 1},
		{RoleSuperintendent, 2},
		{RoleAdmin, 3},
		{RoleOwner, 4},
		{"", 0},
		{"superuser", 0},
		{"intern", 0},
	}
	for _, tt := range tests {
		if got := RoleRank(tt.role); got != tt.want {
			t.Errorf("RoleRank(%q) = %d, want %d", tt.role, got, tt.want)
		}
	}
}

// TestRoleRankStrictlyAscending asserts the ladder is strictly monotonic so the
// "at least" semantics are total over the known roles.
func TestRoleRankStrictlyAscending(t *testing.T) {
	ladder := []string{RoleFieldWorker, RoleSuperintendent, RoleAdmin, RoleOwner}
	for i := 1; i < len(ladder); i++ {
		if RoleRank(ladder[i]) <= RoleRank(ladder[i-1]) {
			t.Fatalf("ladder not strictly ascending at %q (%d) vs %q (%d)",
				ladder[i], RoleRank(ladder[i]), ladder[i-1], RoleRank(ladder[i-1]))
		}
	}
}

// TestRoleAtLeast exercises the full role × min matrix plus the fail-closed
// paths for unknown role and unknown min.
func TestRoleAtLeast(t *testing.T) {
	known := []string{RoleFieldWorker, RoleSuperintendent, RoleAdmin, RoleOwner}

	// Full known × known matrix: role meets min iff its rank >= min's rank.
	for _, role := range known {
		for _, min := range known {
			want := RoleRank(role) >= RoleRank(min)
			if got := RoleAtLeast(role, min); got != want {
				t.Errorf("RoleAtLeast(%q, %q) = %v, want %v", role, min, got, want)
			}
		}
	}

	// Fail-closed: unknown role never satisfies any known min.
	for _, min := range known {
		if RoleAtLeast("intern", min) {
			t.Errorf("RoleAtLeast(unknown role, %q) = true, want false (fail closed)", min)
		}
		if RoleAtLeast("", min) {
			t.Errorf("RoleAtLeast(empty role, %q) = true, want false (fail closed)", min)
		}
	}

	// Fail-closed: unknown min is never satisfied, even by the top role.
	for _, role := range known {
		if RoleAtLeast(role, "superuser") {
			t.Errorf("RoleAtLeast(%q, unknown min) = true, want false (fail closed)", role)
		}
		if RoleAtLeast(role, "") {
			t.Errorf("RoleAtLeast(%q, empty min) = true, want false (fail closed)", role)
		}
	}

	// Both unknown: still false.
	if RoleAtLeast("intern", "superuser") {
		t.Error("RoleAtLeast(unknown, unknown) = true, want false (fail closed)")
	}
}

// TestRoleAtLeastSpotChecks documents a few representative directional cases.
func TestRoleAtLeastSpotChecks(t *testing.T) {
	cases := []struct {
		role, min string
		want      bool
	}{
		{RoleOwner, RoleSuperintendent, true},          // higher meets lower
		{RoleSuperintendent, RoleSuperintendent, true}, // exact meets
		{RoleFieldWorker, RoleSuperintendent, false},   // lower fails higher
		{RoleAdmin, RoleOwner, false},                  // admin < owner
		{RoleSuperintendent, RoleFieldWorker, true},    // super meets field_worker
	}
	for _, c := range cases {
		if got := RoleAtLeast(c.role, c.min); got != c.want {
			t.Errorf("RoleAtLeast(%q, %q) = %v, want %v", c.role, c.min, got, c.want)
		}
	}
}
