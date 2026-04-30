package middleware

import "testing"

func TestClaimsFromDevHeader(t *testing.T) {
	tests := []struct {
		name    string
		header  string
		want    Claims
		wantErr bool
	}{
		{
			name:   "three fields uses default plan tier",
			header: "alice@buildos.dev,demo-org,owner",
			want:   Claims{Sub: "alice@buildos.dev", OrgID: "demo-org", Role: "owner", PlanTier: "enterprise"},
		},
		{
			name:   "four fields overrides plan tier",
			header: "bob@buildos.dev,demo-org,admin,starter",
			want:   Claims{Sub: "bob@buildos.dev", OrgID: "demo-org", Role: "admin", PlanTier: "starter"},
		},
		{
			name:   "whitespace tolerated around fields",
			header: "  carol@buildos.dev , demo-org , superintendent ",
			want:   Claims{Sub: "carol@buildos.dev", OrgID: "demo-org", Role: "superintendent", PlanTier: "enterprise"},
		},
		{name: "empty header rejected", header: "", wantErr: true},
		{name: "two fields rejected", header: "alice,demo-org", wantErr: true},
		{name: "five fields rejected", header: "a,b,c,d,e", wantErr: true},
		{name: "blank sub rejected", header: ",demo-org,owner", wantErr: true},
		{name: "blank org rejected", header: "alice,,owner", wantErr: true},
		{name: "blank role rejected", header: "alice,demo-org,", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := claimsFromDevHeader(tt.header)
			if (err != nil) != tt.wantErr {
				t.Fatalf("claimsFromDevHeader(%q): err = %v, wantErr = %v", tt.header, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Sub != tt.want.Sub || got.OrgID != tt.want.OrgID ||
				got.Role != tt.want.Role || got.PlanTier != tt.want.PlanTier {
				t.Errorf("claimsFromDevHeader(%q) = %+v, want %+v", tt.header, got, tt.want)
			}
		})
	}
}
