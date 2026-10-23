package environment_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ProfessionalWiki/mwenv/internal/environment"
)

func TestIdCombinesNormalizedNameAndPathHash(t *testing.T) {
	assertSame(t, "pageapprovals-d29ed706", environment.ID("PageApprovals", "/home/dev/PageApprovals"))
}

func TestIdReplacesRunsOfOtherCharactersWithDash(t *testing.T) {
	assertSame(t, "my-skin-2-d29ed706", environment.ID("My  Skin_2", "/home/dev/PageApprovals"))
}

func TestIdDiffersPerWorktreePath(t *testing.T) {
	if environment.ID("PageApprovals", "/home/dev/a") == environment.ID("PageApprovals", "/home/dev/b") {
		t.Error("expected different IDs for different worktrees")
	}
}

func TestStartPortIsInRangeAndDiffersPerWorktreePath(t *testing.T) {
	first := environment.StartPort("/home/dev/a")
	second := environment.StartPort("/home/dev/b")

	for _, port := range []int{first, second} {
		if port < 20000 || port > 29999 {
			t.Errorf("port %d is outside 20000-29999", port)
		}
	}
	if first == second {
		t.Error("expected different start ports for different worktrees")
	}
}

func TestPickPortReturnsStartPortWhenFree(t *testing.T) {
	port, err := environment.PickPort(25094, nil, allFree)

	assertNoError(t, err)
	assertSame(t, 25094, port)
}

func TestPickPortSkipsPortsOfOtherEnvironments(t *testing.T) {
	port, err := environment.PickPort(25094, map[int]bool{25094: true, 25095: true}, allFree)

	assertNoError(t, err)
	assertSame(t, 25096, port)
}

func TestPickPortSkipsPortsInUse(t *testing.T) {
	inUse := func(port int) bool { return port != 25094 }

	port, err := environment.PickPort(25094, nil, inUse)

	assertNoError(t, err)
	assertSame(t, 25095, port)
}

func TestPickPortWrapsAroundToStartOfRange(t *testing.T) {
	port, err := environment.PickPort(29999, map[int]bool{29999: true}, allFree)

	assertNoError(t, err)
	assertSame(t, 20000, port)
}

func TestPickPortFailsWhenNoPortIsFree(t *testing.T) {
	_, err := environment.PickPort(25094, nil, func(int) bool { return false })

	if err == nil {
		t.Error("expected an error, got nil")
	}
}

func TestStoreLoadsWhatWasSaved(t *testing.T) {
	store := environment.NewStore(t.TempDir())
	saved := newEnv("pageapprovals-d29ed706")

	assertNoError(t, store.Save(saved))
	loaded, err := store.Load(saved.ID)

	assertNoError(t, err)
	assertSame(t, saved, loaded)
}

func TestStoreReportsMissingEnvironment(t *testing.T) {
	store := environment.NewStore(t.TempDir())

	_, err := store.Load("pageapprovals-d29ed706")

	if !errors.Is(err, environment.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestStoreListsAllEnvironments(t *testing.T) {
	store := environment.NewStore(t.TempDir())
	assertNoError(t, store.Save(newEnv("pageapprovals-d29ed706")))
	assertNoError(t, store.Save(newEnv("chameleon-7432de7d")))

	envs, err := store.List()

	assertNoError(t, err)
	assertSame(t, 2, len(envs))
	assertSame(t, "chameleon-7432de7d", envs[0].ID)
	assertSame(t, "pageapprovals-d29ed706", envs[1].ID)
}

func TestStoreListsNothingForFreshHome(t *testing.T) {
	store := environment.NewStore(filepath.Join(t.TempDir(), "missing"))

	envs, err := store.List()

	assertNoError(t, err)
	assertSame(t, 0, len(envs))
}

func TestStoreListIgnoresEnvironmentDirectoryWithoutState(t *testing.T) {
	store := environment.NewStore(t.TempDir())
	lock, err := store.TryLock("pageapprovals-d29ed706")
	assertNoError(t, err)
	lock.Release()

	envs, err := store.List()

	assertNoError(t, err)
	assertSame(t, 0, len(envs))
}

func TestStoreListIgnoresStrayFiles(t *testing.T) {
	home := t.TempDir()
	store := environment.NewStore(home)
	assertNoError(t, store.Save(newEnv("pageapprovals-d29ed706")))
	assertNoError(t, os.WriteFile(filepath.Join(home, "envs", ".DS_Store"), nil, 0o644))

	envs, err := store.List()

	assertNoError(t, err)
	assertSame(t, 1, len(envs))
}

func TestStoreRemovesEnvironment(t *testing.T) {
	store := environment.NewStore(t.TempDir())
	assertNoError(t, store.Save(newEnv("pageapprovals-d29ed706")))

	assertNoError(t, store.Remove("pageapprovals-d29ed706"))
	_, err := store.Load("pageapprovals-d29ed706")

	if !errors.Is(err, environment.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestEnvironmentLockIsExclusive(t *testing.T) {
	store := environment.NewStore(t.TempDir())
	lock, err := store.TryLock("pageapprovals-d29ed706")
	assertNoError(t, err)

	_, err = store.TryLock("pageapprovals-d29ed706")

	if !errors.Is(err, environment.ErrBusy) {
		t.Errorf("expected ErrBusy, got %v", err)
	}
	lock.Release()
}

func TestEnvironmentLockCanBeTakenAfterRelease(t *testing.T) {
	store := environment.NewStore(t.TempDir())
	lock, err := store.TryLock("pageapprovals-d29ed706")
	assertNoError(t, err)
	lock.Release()

	lock, err = store.TryLock("pageapprovals-d29ed706")

	assertNoError(t, err)
	lock.Release()
}

func TestLockPortsWaitsForHolderToRelease(t *testing.T) {
	store := environment.NewStore(t.TempDir())
	holder, err := store.LockPorts()
	assertNoError(t, err)

	acquired := make(chan *environment.Lock)
	go func() {
		lock, _ := store.LockPorts()
		acquired <- lock
	}()

	select {
	case <-acquired:
		t.Fatal("took the lock while another holder had it")
	case <-time.After(100 * time.Millisecond):
	}

	holder.Release()
	(<-acquired).Release()
}

func newEnv(id string) environment.Env {
	return environment.Env{
		ID:         id,
		Worktree:   "/home/dev/PageApprovals",
		Type:       "extension",
		Name:       "PageApprovals",
		MediaWiki:  "1.43",
		PHP:        "8.3",
		Database:   "mariadb",
		Port:       25094,
		CoreCommit: "81202f0b064bb1a398529516e25843674055cab9",
		Created:    true,
	}
}

func allFree(int) bool {
	return true
}

func assertSame[T comparable](t *testing.T, expected T, actual T) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
