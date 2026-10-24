package cli

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ProfessionalWiki/mwenv/internal/environment"
	"github.com/ProfessionalWiki/mwenv/internal/project"
)

func TestUnfinishedCreationIsBroken(t *testing.T) {
	a := newApp(t)

	assertSame(t, stateBroken, a.stateFrom(env(false), runningContainer()))
}

func TestUnfinishedCreationIsCreatingWhileLocked(t *testing.T) {
	a := newApp(t)
	lock := takeLock(t, a, env(false).ID)
	defer lock.Release()

	assertSame(t, stateCreating, a.stateFrom(env(false), runningContainer()))
}

func TestEnvironmentWithoutContainerIsBroken(t *testing.T) {
	a := newApp(t)

	assertSame(t, stateBroken, a.stateFrom(env(true), map[string]string{}))
}

func TestEnvironmentWithRunningWebContainerIsRunning(t *testing.T) {
	a := newApp(t)

	assertSame(t, stateRunning, a.stateFrom(env(true), runningContainer()))
}

func TestEnvironmentWithExitedWebContainerIsStopped(t *testing.T) {
	a := newApp(t)

	assertSame(t, stateStopped, a.stateFrom(env(true), map[string]string{env(true).ComposeProject(): "exited"}))
}

func TestStatusReportsCreatingBeforeStateIsSaved(t *testing.T) {
	a := newApp(t)
	p := pageApprovalsAt("/home/dev/PageApprovals")
	lock := takeLock(t, a, envID(p))
	defer lock.Release()

	report, err := a.reportFor(p)

	assertNoError(t, err)
	assertSame(t, stateCreating, report.(stateReport).State)
}

func TestStartRefusesUnfinishedCreation(t *testing.T) {
	a := newApp(t)
	lock := takeLock(t, a, env(false).ID)
	defer lock.Release()

	err := a.resume(env(false), startOptions{passed: map[string]bool{}})

	assertErrorContains(t, err, "mwenv destroy")
}

func TestPickPortSkipsPortsOfStoredEnvironments(t *testing.T) {
	a := newApp(t)
	other := env(true)
	other.Port = environment.StartPort("/home/dev/PageApprovals")
	assertNoError(t, a.store.Save(other))

	port, err := a.pickPort("/home/dev/PageApprovals")

	assertNoError(t, err)
	if port == other.Port {
		t.Errorf("expected a port other than %d", other.Port)
	}
}

func TestRunWorksInMatchingSubdirectory(t *testing.T) {
	root := realPath(t, t.TempDir())
	mkdir(t, filepath.Join(root, "src", "Domain"))
	t.Chdir(filepath.Join(root, "src", "Domain"))

	workdir, err := containerWorkdir(pageApprovalsAt(root))

	assertNoError(t, err)
	assertSame(t, "/var/www/html/w/extensions/PageApprovals/src/Domain", workdir)
}

func TestRunResolvesSymlinkedCurrentDirectory(t *testing.T) {
	root := realPath(t, t.TempDir())
	mkdir(t, filepath.Join(root, "src"))
	link := filepath.Join(t.TempDir(), "link")
	assertNoError(t, os.Symlink(root, link))
	t.Chdir(filepath.Join(link, "src"))

	workdir, err := containerWorkdir(pageApprovalsAt(root))

	assertNoError(t, err)
	assertSame(t, "/var/www/html/w/extensions/PageApprovals/src", workdir)
}

func TestRunUsesWorktreeRootFromOutsideWorktree(t *testing.T) {
	root := realPath(t, t.TempDir())
	t.Chdir(t.TempDir())

	workdir, err := containerWorkdir(pageApprovalsAt(root))

	assertNoError(t, err)
	assertSame(t, "/var/www/html/w/extensions/PageApprovals", workdir)
}

func TestRunWithRootRunsTheCommandAsRoot(t *testing.T) {
	options, err := parseRunOptions([]string{"--root", "--", "install-php-extensions", "pcov"})

	assertNoError(t, err)
	assertSame(t, true, options.root)
	assertCommand(t, []string{"install-php-extensions", "pcov"}, options.command)
}

func TestRunWithoutRootRunsTheCommandAsTheUser(t *testing.T) {
	options, err := parseRunOptions([]string{"composer", "test"})

	assertNoError(t, err)
	assertSame(t, false, options.root)
}

func TestRunPassesArgumentsAfterTheCommandToIt(t *testing.T) {
	options, err := parseRunOptions([]string{"composer", "phpunit", "--", "--filter", "Root"})

	assertNoError(t, err)
	assertCommand(t, []string{"composer", "phpunit", "--", "--filter", "Root"}, options.command)
}

func TestRunWithoutCommandFails(t *testing.T) {
	_, err := parseRunOptions([]string{"--root", "--"})

	assertErrorContains(t, err, "no command given")
}

func TestEnvironmentWithMissingWorktreeIsOrphaned(t *testing.T) {
	missing := env(true)
	missing.Worktree = filepath.Join(t.TempDir(), "deleted")
	existing := env(true)
	existing.Worktree = t.TempDir()

	assertSame(t, true, newReport(missing, stateRunning).Orphaned)
	assertSame(t, false, newReport(existing, stateRunning).Orphaned)
}

func TestDestroyFailsForUnknownId(t *testing.T) {
	a := newApp(t)

	if err := a.destroy([]string{"pageapprovals-d29ed706"}); err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestDestroyRejectsIdOutsideStateDirectory(t *testing.T) {
	home := t.TempDir()
	a := newAppIn(home)
	outside := filepath.Join(home, "outside")
	mkdir(t, outside)
	assertNoError(t, os.WriteFile(filepath.Join(outside, "env.json"), []byte("{}"), 0o644))

	err := a.destroy([]string{"../outside"})

	if err == nil {
		t.Error("expected an error, got nil")
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Errorf("expected %s to survive, got %v", outside, statErr)
	}
}

func TestDestroyFailsWhileStartIsPreparing(t *testing.T) {
	a := newApp(t)
	root := newExtensionRepo(t)
	t.Chdir(root)
	id := environment.ID("PageApprovals", root)
	lock := takeLock(t, a, id)
	defer lock.Release()

	err := a.destroy(nil)

	if !errors.Is(err, environment.ErrBusy) {
		t.Errorf("expected ErrBusy, got %v", err)
	}
}

func TestRebuildRejectsUnsupportedMediaWikiVersion(t *testing.T) {
	hideDocker(t)

	err := newApp(t).rebuild([]string{"--mw", "1.39"})

	assertErrorContains(t, err, "1.39")
}

func TestRebuildRejectsInvalidPHPVersion(t *testing.T) {
	hideDocker(t)

	err := newApp(t).rebuild([]string{"--php", "eight"})

	assertErrorContains(t, err, "not a PHP version")
}

func TestRebuildFailsWhenTheBuildFails(t *testing.T) {
	hideDocker(t)

	if err := newApp(t).rebuild(nil); err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestPortHeldOnIpv6LoopbackIsNotFree(t *testing.T) {
	listener, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skip("this machine has no IPv6 loopback")
	}
	defer listener.Close()

	assertSame(t, false, portIsFree(listener.Addr().(*net.TCPAddr).Port))
}

func TestPortReleasedWithinTimeoutIsFree(t *testing.T) {
	listener := listenOnLoopback(t)
	time.AfterFunc(100*time.Millisecond, func() { _ = listener.Close() })

	assertSame(t, true, portIsFreeWithin(listener.Addr().(*net.TCPAddr).Port, 2*time.Second))
}

func TestPortHeldPastTimeoutIsNotFree(t *testing.T) {
	listener := listenOnLoopback(t)
	defer listener.Close()

	assertSame(t, false, portIsFreeWithin(listener.Addr().(*net.TCPAddr).Port, 100*time.Millisecond))
}

func TestOmittedStartFlagsKeepExistingValues(t *testing.T) {
	options, err := parseStartOptions([]string{})
	assertNoError(t, err)

	if err := checkUnchangedOptions(env(true), options); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestPassingTheExistingValueIsAccepted(t *testing.T) {
	options, err := parseStartOptions([]string{"--mw", "1.45"})
	assertNoError(t, err)

	if err := checkUnchangedOptions(env(true), options); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestPassingADifferentValueIsRejected(t *testing.T) {
	options, err := parseStartOptions([]string{"--php", "8.3"})
	assertNoError(t, err)

	if err := checkUnchangedOptions(env(true), options); err == nil {
		t.Error("expected an error, got nil")
	}
}

// hideDocker makes every docker call fail, so that a test never builds or removes the images the user's
// environments share.
func hideDocker(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
}

func listenOnLoopback(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	assertNoError(t, err)
	return listener
}

func takeLock(t *testing.T, a app, id string) *environment.Lock {
	t.Helper()
	lock, err := a.store.TryLock(id)
	assertNoError(t, err)
	return lock
}

func pageApprovalsAt(root string) project.Project {
	return project.Project{Root: root, Type: project.Extension, Name: "PageApprovals"}
}

func newExtensionRepo(t *testing.T) string {
	t.Helper()
	root := realPath(t, t.TempDir())
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, output)
	}
	assertNoError(t, os.WriteFile(filepath.Join(root, "extension.json"), []byte(`{ "name": "PageApprovals" }`), 0o644))
	return root
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	assertNoError(t, err)
	return resolved
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	assertNoError(t, os.MkdirAll(dir, 0o755))
}

func assertContains(t *testing.T, haystack string, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Errorf("expected to find %q in %q", needle, haystack)
	}
}

func assertCommand(t *testing.T, expected []string, actual []string) {
	t.Helper()
	if !slices.Equal(expected, actual) {
		t.Errorf("expected %q, got %q", expected, actual)
	}
}

func assertErrorContains(t *testing.T, err error, needle string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error containing %q, got nil", needle)
	}
	assertContains(t, err.Error(), needle)
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newApp(t *testing.T) app {
	return newAppIn(t.TempDir())
}

func newAppIn(home string) app {
	return app{store: environment.NewStore(home), stdout: io.Discard, stderr: io.Discard}
}

func env(created bool) environment.Env {
	return environment.Env{
		ID:        "pageapprovals-d29ed706",
		MediaWiki: "1.45",
		PHP:       "8.4",
		Database:  "sqlite",
		Created:   created,
	}
}

func runningContainer() map[string]string {
	return map[string]string{env(true).ComposeProject(): "running"}
}

func assertSame[T comparable](t *testing.T, expected T, actual T) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}
