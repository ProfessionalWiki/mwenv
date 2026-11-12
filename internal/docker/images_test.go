package docker

import (
	"slices"
	"strings"
	"testing"

	"github.com/ProfessionalWiki/mwenv/internal/mediawiki"
)

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

func TestOtherBuildsAreTheImagesWithOtherHashes(t *testing.T) {
	hashes := map[string]string{"mwenv-core": "aaaaaaaa", "mwenv-php": "bbbbbbbb"}
	listing := strings.Join([]string{
		"mwenv-php:8.3-bbbbbbbb",
		"mwenv-php:8.3-cccccccc",
		"mwenv-core:1.43-php8.3-aaaaaaaa",
		"mwenv-core:REL1_47-php8.5-rc-dddddddd",
		"localhost/mwenv-core:master-php8.4-eeeeeeee",
		"localhost/mwenv-php:8.4-bbbbbbbb",
		"<none>:<none>",
		"mwenv-core:mine",
		"example.org:5000/mwenv-core:1.43-php8.3-ffffffff",
		"other:8.3-cccccccc",
		"",
	}, "\n")

	got := otherBuilds(listing, hashes)

	want := []string{
		"mwenv-php:8.3-cccccccc",
		"mwenv-core:REL1_47-php8.5-rc-dddddddd",
		"localhost/mwenv-core:master-php8.4-eeeeeeee",
	}
	if !slices.Equal(got, want) {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestTheImagesThisVersionBuildsAreNotOtherBuilds(t *testing.T) {
	version, err := mediawiki.ParseVersion("1.43")
	if err != nil {
		t.Fatal(err)
	}
	listing := PHPImage("8.3") + "\n" + CoreImage(version, "8.3") + "\n"

	if got := otherBuilds(listing, currentHashes()); len(got) != 0 {
		t.Errorf("expected none, got %q", got)
	}
}
