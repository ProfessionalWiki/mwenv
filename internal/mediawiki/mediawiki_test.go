package mediawiki_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/ProfessionalWiki/mwenv/internal/mediawiki"
	"github.com/ProfessionalWiki/mwenv/internal/project"
)

func TestMajorVersionMeansReleaseBranch(t *testing.T) {
	v := parseVersion(t, "1.45")

	assertSame(t, "REL1_45", v.CoreRef)
	assertSame(t, "REL1_45", v.VectorRef)
}

func TestOldestSupportedVersionIsAccepted(t *testing.T) {
	v := parseVersion(t, "1.43")

	assertSame(t, "REL1_43", v.CoreRef)
}

func TestReleaseVersionMeansTagWithVectorFromReleaseBranch(t *testing.T) {
	v := parseVersion(t, "1.45.2")

	assertSame(t, "1.45.2", v.CoreRef)
	assertSame(t, "REL1_45", v.VectorRef)
}

func TestReleaseCandidateMeansTagWithVectorFromReleaseBranch(t *testing.T) {
	v := parseVersion(t, "1.45.0-rc.0")

	assertSame(t, "1.45.0-rc.0", v.CoreRef)
	assertSame(t, "REL1_45", v.VectorRef)
}

func TestReleaseBranchNameIsAccepted(t *testing.T) {
	v := parseVersion(t, "REL1_44")

	assertSame(t, "REL1_44", v.CoreRef)
	assertSame(t, "REL1_44", v.VectorRef)
}

func TestOtherRefIsUsedAsIsWithVectorMaster(t *testing.T) {
	v := parseVersion(t, "wmf/1.46.0-wmf.3")

	assertSame(t, "wmf/1.46.0-wmf.3", v.CoreRef)
	assertSame(t, "master", v.VectorRef)
}

func TestTagPartReplacesCharactersNotAllowedInDockerTags(t *testing.T) {
	assertSame(t, "wmf-1.46.0-wmf.3", parseVersion(t, "wmf/1.46.0-wmf.3").TagPart())
}

func TestVersionsBefore143AreRejected(t *testing.T) {
	for _, version := range []string{"1.42", "1.39.1", "1.42.0-rc.0", "REL1_40"} {
		t.Run(version, func(t *testing.T) {
			if _, err := mediawiki.ParseVersion(version); err == nil {
				t.Errorf("expected an error for %s", version)
			}
		})
	}
}

func TestComposerLocalMergesWorktreeComposerJson(t *testing.T) {
	var composerLocal struct {
		Extra struct {
			MergePlugin struct {
				Include []string `json:"include"`
			} `json:"merge-plugin"`
		} `json:"extra"`
	}

	if err := json.Unmarshal([]byte(mediawiki.ComposerLocal(minerva())), &composerLocal); err != nil {
		t.Fatal(err)
	}

	assertSame(t, "skins/MinervaNeue/composer.json", strings.Join(composerLocal.Extra.MergePlugin.Include, ","))
}

func TestSettingsLoadExtension(t *testing.T) {
	settings := mediawiki.Settings(pageApprovals(), "pageapprovals-d29ed706")

	assertContains(t, settings, "wfLoadExtension( 'PageApprovals' );")
	assertContains(t, settings, "$wgCookiePrefix = 'pageapprovals-d29ed706';")
	assertNotContains(t, settings, "$wgDefaultSkin")
}

func TestSettingsLoadSkinByDirectoryAndMakeItsKeyTheDefault(t *testing.T) {
	settings := mediawiki.Settings(minerva(), "minervaneue-7432de7d")

	assertContains(t, settings, "wfLoadSkin( 'MinervaNeue' );")
	assertContains(t, settings, "$wgDefaultSkin = 'minerva';")
}

func TestSettingsEscapePhpStrings(t *testing.T) {
	settings := mediawiki.Settings(project.Project{Type: project.Extension, Name: `It's\`}, "id")

	assertContains(t, settings, `wfLoadExtension( 'It\'s\\' );`)
}

func TestInstallArgumentsForMariaDb(t *testing.T) {
	args := mediawiki.InstallArgs(pageApprovals(), mediawiki.MariaDB, 25094)

	assertHas(t, args, "--dbtype=mysql")
	assertHas(t, args, "--dbserver=db")
	assertHas(t, args, "--server=http://localhost:25094")
	assertHas(t, args, "--skins=Vector")
}

func TestInstallArgumentsForSqlite(t *testing.T) {
	args := mediawiki.InstallArgs(pageApprovals(), mediawiki.SQLite, 25094)

	assertHas(t, args, "--dbtype=sqlite")
	assertHas(t, args, "--dbpath="+mediawiki.SQLiteDir)
	assertNotHas(t, args, "--dbserver=db")
}

func TestInstallArgumentsEndWithSiteNameAndAdminUser(t *testing.T) {
	args := mediawiki.InstallArgs(pageApprovals(), mediawiki.SQLite, 25094)

	assertSame(t, "PageApprovals Admin", strings.Join(args[len(args)-2:], " "))
}

func TestCompletePageHasNoProblem(t *testing.T) {
	problem, _ := mediawiki.PageProblem(200, "<!DOCTYPE html>\n<html><body>Main Page</body></html>\n")

	assertSame(t, "", problem)
}

func TestErrorStatusIsAProblemWorthRetrying(t *testing.T) {
	problem, retry := mediawiki.PageProblem(500, "<html><body><p>MediaWiki internal error.</p></body></html>")

	assertContains(t, problem, "HTTP 500")
	assertContains(t, problem, "MediaWiki internal error.")
	assertSame(t, true, retry)
}

func TestPageCutShortByFatalErrorIsAProblemNotWorthRetrying(t *testing.T) {
	problem, retry := mediawiki.PageProblem(200, "<html><body><br />\n<b>Fatal error</b>:  Allowed memory size exhausted<br />")

	assertContains(t, problem, "Allowed memory size exhausted")
	assertSame(t, false, retry)
}

func pageApprovals() project.Project {
	return project.Project{Type: project.Extension, Name: "PageApprovals"}
}

func minerva() project.Project {
	return project.Project{Type: project.Skin, Name: "MinervaNeue", DefaultSkinKey: "minerva"}
}

func parseVersion(t *testing.T, version string) mediawiki.Version {
	t.Helper()
	v, err := mediawiki.ParseVersion(version)
	if err != nil {
		t.Fatalf("ParseVersion(%q): %v", version, err)
	}
	return v
}

func assertSame[T comparable](t *testing.T, expected T, actual T) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected to find %q in:\n%s", needle, haystack)
	}
}

func assertNotContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if strings.Contains(haystack, needle) {
		t.Errorf("did not expect %q in:\n%s", needle, haystack)
	}
}

func assertNotHas(t *testing.T, args []string, arg string) {
	t.Helper()
	if slices.Contains(args, arg) {
		t.Errorf("did not expect %q in %v", arg, args)
	}
}

func assertHas(t *testing.T, args []string, arg string) {
	t.Helper()
	if !slices.Contains(args, arg) {
		t.Errorf("expected %q in %v", arg, args)
	}
}
