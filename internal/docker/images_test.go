package docker

import "testing"

func TestMinorAndPatchPHPVersionsAreAccepted(t *testing.T) {
	for _, version := range []string{"8.3", "8.4.1"} {
		if err := ValidatePHPVersion(version); err != nil {
			t.Errorf("expected %s to be accepted, got %v", version, err)
		}
	}
}

func TestPHPVersionWithTrailingTextIsRejected(t *testing.T) {
	if err := ValidatePHPVersion("8.3:latest"); err == nil {
		t.Error("expected an error, got nil")
	}
}
