package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"text/tabwriter"

	"github.com/ProfessionalWiki/mwenv/internal/docker"
	"github.com/ProfessionalWiki/mwenv/internal/environment"
	"github.com/ProfessionalWiki/mwenv/internal/mediawiki"
	"github.com/ProfessionalWiki/mwenv/internal/project"
)

const (
	stateAbsent   = "absent"
	stateCreating = "creating"
	stateBroken   = "broken"
	stateRunning  = "running"
	stateStopped  = "stopped"
)

func (a app) state(env environment.Env) (string, error) {
	if !env.Created {
		return a.stateFrom(env, nil), nil
	}

	containerStates, err := docker.WebContainerStates()
	if err != nil {
		return "", err
	}
	return a.stateFrom(env, containerStates), nil
}

func (a app) stateFrom(env environment.Env, containerStates map[string]string) string {
	if !env.Created {
		if a.store.IsLocked(env.ID) {
			return stateCreating
		}
		return stateBroken
	}

	containerState, found := containerStates[env.ComposeProject()]
	if !found {
		return stateBroken
	}
	if containerState == "running" {
		return stateRunning
	}
	return stateStopped
}

type report struct {
	ID            string          `json:"id"`
	State         string          `json:"state"`
	Orphaned      bool            `json:"orphaned"`
	Worktree      string          `json:"worktree"`
	Type          string          `json:"type"`
	Name          string          `json:"name"`
	URL           string          `json:"url"`
	Admin         adminReport     `json:"admin"`
	MediaWiki     mediaWikiReport `json:"mediawiki"`
	LocalSettings string          `json:"localSettings"`
	PHP           string          `json:"php"`
	Database      string          `json:"database"`
}

type adminReport struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type mediaWikiReport struct {
	Requested string  `json:"requested"`
	Commit    *string `json:"commit"`
	Path      string  `json:"path"`
}

type stateReport struct {
	State    string `json:"state"`
	Worktree string `json:"worktree"`
}

func (a app) reportFor(p project.Project) (any, error) {
	env, err := a.store.Load(envID(p))
	if errors.Is(err, environment.ErrNotFound) {
		if a.store.IsLocked(envID(p)) {
			return stateReport{State: stateCreating, Worktree: p.Root}, nil
		}
		return stateReport{State: stateAbsent, Worktree: p.Root}, nil
	}
	if err != nil {
		return nil, err
	}

	state, err := a.state(env)
	if err != nil {
		return nil, err
	}

	return newReport(env, state), nil
}

func newReport(env environment.Env, state string) report {
	r := report{
		ID:            env.ID,
		State:         state,
		Orphaned:      !exists(env.Worktree),
		Worktree:      env.Worktree,
		Type:          env.Type,
		Name:          env.Name,
		URL:           mainPageURL(env.Port),
		Admin:         adminReport{Username: mediawiki.AdminUser, Password: mediawiki.AdminPassword},
		MediaWiki:     mediaWikiReport{Requested: env.MediaWiki, Path: docker.InstallPath},
		LocalSettings: docker.InstallPath + "/LocalSettings.php",
		PHP:           env.PHP,
		Database:      env.Database,
	}

	if env.CoreCommit != "" {
		r.MediaWiki.Commit = &env.CoreCommit
	}

	return r
}

func mainPageURL(port int) string {
	return mediawiki.ServerURL(port) + mediawiki.MainPagePath
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, fs.ErrNotExist)
}

func (a app) printJSON(value any) {
	content, _ := json.MarshalIndent(value, "", "\t")
	fmt.Fprintln(a.stdout, string(content))
}

func (a app) printReady(env environment.Env) {
	fmt.Fprintf(a.stdout, "MediaWiki is ready at %s\n", mainPageURL(env.Port))
	fmt.Fprintf(a.stdout, "Admin username: %s\n", mediawiki.AdminUser)
	fmt.Fprintf(a.stdout, "Admin password: %s\n", mediawiki.AdminPassword)
}

func (a app) printStatus(value any) {
	r, isReport := value.(report)
	if !isReport {
		fmt.Fprintf(a.stdout, "No environment for %s\n", value.(stateReport).Worktree)
		return
	}

	commit := "not installed yet"
	if r.MediaWiki.Commit != nil {
		commit = *r.MediaWiki.Commit
	}

	table := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(table, "ID:\t%s\n", r.ID)
	fmt.Fprintf(table, "State:\t%s\n", r.State)
	fmt.Fprintf(table, "URL:\t%s\n", r.URL)
	fmt.Fprintf(table, "Admin:\t%s / %s\n", r.Admin.Username, r.Admin.Password)
	fmt.Fprintf(table, "MediaWiki:\t%s (%s)\n", r.MediaWiki.Requested, commit)
	fmt.Fprintf(table, "PHP:\t%s\n", r.PHP)
	fmt.Fprintf(table, "Database:\t%s\n", r.Database)
	fmt.Fprintf(table, "Worktree:\t%s\n", r.Worktree)
	fmt.Fprintf(table, "Installation:\t%s (in the container)\n", r.MediaWiki.Path)
	fmt.Fprintf(table, "LocalSettings:\t%s (in the container)\n", r.LocalSettings)
	_ = table.Flush()
}

func (a app) printList(reports []report) {
	table := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tSTATE\tURL\tWORKTREE")

	for _, r := range reports {
		worktree := r.Worktree
		if r.Orphaned {
			worktree += " (orphaned)"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", r.ID, r.State, r.URL, worktree)
	}

	_ = table.Flush()
}
