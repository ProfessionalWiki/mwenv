package docker

import "testing"

func TestPHPVersionsAreAccepted(t *testing.T) {
	for _, version := range []string{"8.3", "8.4.1", "8.5-rc"} {
		if err := ValidatePHPVersion(version); err != nil {
			t.Errorf("expected %s to be accepted, got %v", version, err)
		}
	}
}

func TestPHPVersionWithOtherTextIsRejected(t *testing.T) {
	for _, version := range []string{"8.3:latest", "php8.3"} {
		if err := ValidatePHPVersion(version); err == nil {
			t.Errorf("expected %s to be rejected", version)
		}
	}
}
