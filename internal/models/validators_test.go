package models

import "testing"

func TestIsValidReportedVia(t *testing.T) {
	valid := []string{ReportedViaWeb, ReportedViaMobile}
	for _, s := range valid {
		if !IsValidReportedVia(s) {
			t.Errorf("IsValidReportedVia(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "Web", "desktop", "api"}
	for _, s := range invalid {
		if IsValidReportedVia(s) {
			t.Errorf("IsValidReportedVia(%q) = true, want false", s)
		}
	}
}

func TestIsValidCertificationStatus(t *testing.T) {
	valid := []string{
		CertificationStatusActive,
		CertificationStatusExpired,
		CertificationStatusRevoked,
	}
	for _, s := range valid {
		if !IsValidCertificationStatus(s) {
			t.Errorf("IsValidCertificationStatus(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "Active", "pending", "suspended"}
	for _, s := range invalid {
		if IsValidCertificationStatus(s) {
			t.Errorf("IsValidCertificationStatus(%q) = true, want false", s)
		}
	}
}
