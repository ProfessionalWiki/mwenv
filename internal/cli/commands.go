package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ProfessionalWiki/mwenv/internal/docker"
	"github.com/ProfessionalWiki/mwenv/internal/environment"
	"github.com/ProfessionalWiki/mwenv/internal/mediawiki"
	"github.com/ProfessionalWiki/mwenv/internal/project"
)

func (a app) run(args []string) (int, error) {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return 2, errors.New("run: no command given")
	}

	p, env, err := a.currentEnv()
	if err != nil {
		return 0, err
	}

	state, err := a.state(env)
	if err != nil {
		return 0, err
	}
	if state != stateRunning {
		return 0, fmt.Errorf("the environment %s is %s, not running; run mwenv start first", env.ID, state)
	}

	workdir, err := containerWorkdir(p)
	if err != nil {
		return 0, err
	}

	return a.compose(env.ID).ExecAttached(workdir, args...)
}

func (a app) currentEnv() (project.Project, environment.Env, error) {
	p, err := currentProject()
	if err != nil {
		return p, environment.Env{}, err
	}

	env, err := a.store.Load(envID(p))
	if errors.Is(err, environment.ErrNotFound) {
		return p, env, fmt.Errorf("there is no environment for %s; run mwenv start first", p.Root)
	}
	return p, env, err
}

// containerWorkdir is the container directory matching the current directory, or the worktree's own directory when
// the current directory is outside the worktree.
func containerWorkdir(p project.Project) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}

	relative, err := filepath.Rel(p.Root, dir)
	if err != nil || !filepath.IsLocal(relative) {
		return docker.MountPath(p), nil
	}

	return filepath.ToSlash(filepath.Join(docker.MountPath(p), relative)), nil
}

func (a app) status(args []string) error {
	flags := newFlagSet("status")
	asJSON := flags.Bool("json", false, "")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	p, err := currentProject()
	if err != nil {
		return err
	}

	r, err := a.reportFor(p)
	if err != nil {
		return err
	}

	if *asJSON {
		a.printJSON(r)
	} else {
		a.printStatus(r)
	}
	return nil
}

func (a app) list(args []string) error {
	flags := newFlagSet("ls")
	asJSON := flags.Bool("json", false, "")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	envs, err := a.store.List()
	if err != nil {
		return err
	}

	containerStates := map[string]string{}
	if len(envs) > 0 {
		containerStates, err = docker.WebContainerStates()
		if err != nil {
			return err
		}
	}

	reports := []report{}
	for _, env := range envs {
		reports = append(reports, newReport(env, a.stateFrom(env, containerStates)))
	}

	if *asJSON {
		a.printJSON(reports)
	} else {
		a.printList(reports)
	}
	return nil
}

func (a app) stop(args []string) error {
	if err := parseFlags(newFlagSet("stop"), args); err != nil {
		return err
	}

	p, err := currentProject()
	if err != nil {
		return err
	}

	id := envID(p)
	if _, err := a.store.Load(id); errors.Is(err, environment.ErrNotFound) {
		fmt.Fprintf(a.stdout, "No environment for %s\n", p.Root)
		return nil
	} else if err != nil {
		return err
	}

	lock, err := a.store.TryLock(id)
	if err != nil {
		return err
	}
	defer lock.Release()

	if err := a.compose(id).Stop(); err != nil {
		return err
	}

	fmt.Fprintf(a.stdout, "Stopped %s\n", id)
	return nil
}

func (a app) logs(args []string) (int, error) {
	flags := newFlagSet("logs")
	follow := flags.Bool("follow", false, "")
	flags.BoolVar(follow, "f", false, "")
	if err := parseFlags(flags, args); err != nil {
		return 0, err
	}

	_, env, err := a.currentEnv()
	if err != nil {
		return 0, err
	}

	return a.compose(env.ID).Logs(*follow)
}

func (a app) destroy(ids []string) error {
	if len(ids) == 0 {
		return a.destroyCurrent()
	}

	envs, err := a.store.List()
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, env := range envs {
		known[env.ID] = true
	}

	for _, id := range ids {
		if !known[id] {
			return fmt.Errorf("unknown environment %q; mwenv ls lists the environments", id)
		}
	}

	for _, id := range ids {
		if err := a.destroyLocked(id); err != nil {
			return err
		}
	}
	return nil
}

func (a app) destroyLocked(id string) error {
	lock, err := a.store.TryLock(id)
	if err != nil {
		return err
	}
	defer lock.Release()

	return a.removeEnvironment(id)
}

func (a app) destroyCurrent() error {
	p, err := currentProject()
	if err != nil {
		return err
	}

	id := envID(p)
	lock, err := a.store.TryLock(id)
	if err != nil {
		return err
	}
	defer lock.Release()

	if _, err := a.store.Load(id); errors.Is(err, environment.ErrNotFound) {
		fmt.Fprintf(a.stdout, "No environment for %s\n", p.Root)
		return a.store.Remove(id)
	}

	return a.removeEnvironment(id)
}

func (a app) removeEnvironment(id string) error {
	if exists(a.store.ComposeFile(id)) {
		a.progress("removing the containers and volumes of %s", id)
		if err := a.compose(id).Down(); err != nil {
			return err
		}
		a.pruneImages()
	}

	if err := a.store.Remove(id); err != nil {
		return err
	}

	fmt.Fprintf(a.stdout, "Destroyed %s\n", id)
	return nil
}

// pruneImages does not fail the command: a failure only leaves images behind, which the next prune removes.
func (a app) pruneImages() {
	if err := docker.PruneImages(); err != nil {
		a.progress("removing unused images failed: %v", err)
	}
}

func (a app) rebuild(args []string) error {
	flags := newFlagSet("rebuild")
	mediaWiki := flags.String("mw", defaultMediaWiki, "")
	php := flags.String("php", defaultPHP, "")
	if err := parseFlags(flags, args); err != nil {
		return err
	}

	version, err := mediawiki.ParseVersion(*mediaWiki)
	if err != nil {
		return err
	}
	if err := docker.ValidatePHPVersion(*php); err != nil {
		return err
	}

	onBuild := func(image string) { a.progress("rebuilding %s", image) }
	if err := docker.RebuildImages(version, *php, onBuild); err != nil {
		return err
	}
	a.pruneImages()

	fmt.Fprintf(a.stdout, "Rebuilt the images for MediaWiki %s and PHP %s\n", version.Requested, *php)
	return nil
}
