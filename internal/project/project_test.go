package project_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ProfessionalWiki/mwenv/internal/project"
)

func TestDetectsExtensionFromExtensionJson(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "extension.json", `{ "name": "PageApprovals" }`)

	p := detect(t, root)

	assertSame(t, project.Extension, p.Type)
	assertSame(t, "PageApprovals", p.Name)
	assertSame(t, "extensions/PageApprovals", p.Dir())
}

func TestDirectoryNameIsNameWithoutSpaces(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "extension.json", `{ "name": "Page Approvals" }`)

	p := detect(t, root)

	assertSame(t, "PageApprovals", p.Name)
}

func TestDetectsSkinWithItsFirstSkinKeyAsDefaultSkin(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "skin.json", `{ "name": "Citizen", "ValidSkinNames": { "zeta": {}, "citizen": {}, "alpha": {} } }`)

	p := detect(t, root)

	assertSame(t, project.Skin, p.Type)
	assertSame(t, "Citizen", p.Name)
	assertSame(t, "zeta", p.DefaultSkinKey)
	assertSame(t, "skins/Citizen", p.Dir())
}

func TestSkinWithoutValidSkinNamesUsesLowercaseNameAsDefaultSkin(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "skin.json", `{ "name": "Chameleon", "callback": "Skins\\Chameleon\\Chameleon::init" }`)

	p := detect(t, root)

	assertSame(t, "chameleon", p.DefaultSkinKey)
}

func TestDetectsRepositoryRootFromSubdirectory(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "extension.json", `{ "name": "PageApprovals" }`)
	subdirectory := filepath.Join(root, "src", "Domain")
	mkdir(t, subdirectory)

	p := detect(t, subdirectory)

	assertSame(t, root, p.Root)
}

func TestFailsOutsideGitRepository(t *testing.T) {
	_, err := project.Detect(realPath(t, t.TempDir()))

	assertError(t, err)
}

func TestFailsWithoutExtensionOrSkinJson(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "composer.json", `{}`)

	_, err := project.Detect(root)

	assertError(t, err)
}

func TestFailsForNameThatIsNotADirectoryName(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "extension.json", `{ "name": "../Evil" }`)

	_, err := project.Detect(root)

	assertError(t, err)
}

func TestMainCheckoutHasNoExternalGitDir(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "extension.json", `{ "name": "PageApprovals" }`)

	p := detect(t, root)

	assertSame(t, "", p.ExternalGitDir)
}

func TestLinkedWorktreeReportsMainRepositoryGitDir(t *testing.T) {
	root := newRepo(t)
	writeFile(t, root, "extension.json", `{ "name": "PageApprovals" }`)
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "Initial")
	worktree := filepath.Join(realPath(t, t.TempDir()), "fix-ui")
	git(t, root, "worktree", "add", "-q", worktree)

	p := detect(t, worktree)

	assertSame(t, worktree, p.Root)
	assertSame(t, filepath.Join(root, ".git"), p.ExternalGitDir)
}

func detect(t *testing.T, dir string) project.Project {
	t.Helper()
	p, err := project.Detect(dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	return p
}

func newRepo(t *testing.T) string {
	t.Helper()
	root := realPath(t, t.TempDir())
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test")
	return root
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Without the developer's global config, commit signing cannot prompt or hang.
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func writeFile(t *testing.T, dir string, name string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func assertSame[T comparable](t *testing.T, expected T, actual T) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Error("expected an error, got nil")
	}
}
