package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ProfessionalWiki/mwenv/internal/docker"
	"github.com/ProfessionalWiki/mwenv/internal/environment"
	"github.com/ProfessionalWiki/mwenv/internal/mediawiki"
	"github.com/ProfessionalWiki/mwenv/internal/project"
)

const (
	mainPageTimeout = 60 * time.Second
	retryAdvice     = "fix the cause through mwenv run, then run mwenv start again"
)

type startOptions struct {
	mediaWiki string
	php       string
	database  string
	json      bool
	passed    map[string]bool
}

func (a app) start(args []string) error {
	options, err := parseStartOptions(args)
	if err != nil {
		return err
	}

	p, err := currentProject()
	if err != nil {
		return err
	}

	env, err := a.startLocked(p, options)

	if !options.json {
		if err == nil {
			a.printReady(env)
		}
		return err
	}

	var report any = newReport(env, stateRunning)
	if err != nil {
		var reportErr error
		if report, reportErr = a.reportFor(p); reportErr != nil {
			return errors.Join(err, reportErr)
		}
	}
	a.printJSON(report)
	return err
}

func (a app) startLocked(p project.Project, options startOptions) (environment.Env, error) {
	id := envID(p)
	lock, err := a.store.TryLock(id)
	if err != nil {
		return environment.Env{}, err
	}
	defer lock.Release()

	env, err := a.store.Load(id)
	switch {
	case errors.Is(err, environment.ErrNotFound):
		env, err = a.create(id, p, options)
	case err == nil:
		err = a.resume(env, options)
	}

	if err != nil {
		return env, err
	}

	return env, a.finish(env)
}

func parseStartOptions(args []string) (startOptions, error) {
	options := startOptions{passed: map[string]bool{}}

	flags := newFlagSet("start")
	flags.StringVar(&options.mediaWiki, "mw", "1.43", "")
	flags.StringVar(&options.php, "php", "8.3", "")
	flags.StringVar(&options.database, "db", string(mediawiki.MariaDB), "")
	flags.BoolVar(&options.json, "json", false, "")

	if err := parseFlags(flags, args); err != nil {
		return options, err
	}
	flags.Visit(func(f *flag.Flag) { options.passed[f.Name] = true })

	return options, nil
}

func (a app) resume(env environment.Env, options startOptions) error {
	if err := checkUnchangedOptions(env, options); err != nil {
		return err
	}

	if !env.Created {
		return errBroken(env.ID)
	}

	state, err := a.state(env)
	if err != nil {
		return err
	}

	switch state {
	case stateBroken:
		return errBroken(env.ID)
	case stateStopped:
		if !portIsFree(env.Port) {
			return fmt.Errorf("port %d of the environment %s is in use by another process", env.Port, env.ID)
		}
		a.progress("starting containers")
		return a.compose(env.ID).Start()
	}

	return nil
}

func errBroken(id string) error {
	return fmt.Errorf("the environment %s is broken; run mwenv destroy, then mwenv start", id)
}

func checkUnchangedOptions(env environment.Env, options startOptions) error {
	for _, option := range []struct{ name, existing, requested string }{
		{"mw", env.MediaWiki, options.mediaWiki},
		{"php", env.PHP, options.php},
		{"db", env.Database, options.database},
	} {
		if options.passed[option.name] && option.requested != option.existing {
			return fmt.Errorf(
				"the environment %s was created with --%s %s, not %s; run mwenv destroy first to change it",
				env.ID, option.name, option.existing, option.requested,
			)
		}
	}

	return nil
}

func (a app) create(id string, p project.Project, options startOptions) (environment.Env, error) {
	version, err := mediawiki.ParseVersion(options.mediaWiki)
	if err != nil {
		return environment.Env{}, err
	}
	database, err := mediawiki.ParseDatabase(options.database)
	if err != nil {
		return environment.Env{}, err
	}
	if err := docker.ValidatePHPVersion(options.php); err != nil {
		return environment.Env{}, err
	}

	onBuild := func(image string) { a.progress("building %s, which takes a few minutes the first time", image) }
	if err := docker.EnsureImages(version, options.php, onBuild); err != nil {
		return environment.Env{}, err
	}

	env := environment.Env{
		ID:        id,
		Worktree:  p.Root,
		Type:      string(p.Type),
		Name:      p.Name,
		MediaWiki: version.Requested,
		PHP:       options.php,
		Database:  string(database),
	}

	if err := a.record(&env, p, docker.CoreImage(version, options.php)); err != nil {
		return env, err
	}

	return env, a.install(&env, p)
}

func (a app) record(env *environment.Env, p project.Project, image string) error {
	lock, err := a.store.LockPorts()
	if err != nil {
		return err
	}
	defer lock.Release()

	env.Port, err = a.pickPort(env.Worktree)
	if err != nil {
		return err
	}

	composeFile, err := docker.ComposeFile(*env, p, image, a.user)
	if err != nil {
		return err
	}
	if err := a.store.SaveComposeFile(env.ID, composeFile); err != nil {
		return err
	}

	return a.store.Save(*env)
}

func (a app) pickPort(worktree string) (int, error) {
	envs, err := a.store.List()
	if err != nil {
		return 0, err
	}

	taken := map[int]bool{}
	for _, other := range envs {
		taken[other.Port] = true
	}

	return environment.PickPort(environment.StartPort(worktree), taken, portIsFree)
}

// portIsFree also tries to connect, because on macOS binding 127.0.0.1 succeeds while another process listens on all
// interfaces or on ::1, where http://localhost can reach it.
func portIsFree(port int) bool {
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()

	for _, host := range []string{"127.0.0.1", "::1"} {
		connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 200*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return false
		}
	}
	return true
}

func (a app) install(env *environment.Env, p project.Project) error {
	compose := a.compose(env.ID)

	a.progress("starting containers")
	if err := compose.Up(); err != nil {
		return fmt.Errorf("starting the containers failed: %w", err)
	}

	commit, err := compose.Exec("", "git", "rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("reading the MediaWiki commit failed: %w", err)
	}
	env.CoreCommit = strings.TrimSpace(commit)

	a.progress("installing composer dependencies")
	if _, err := compose.Exec(mediawiki.ComposerLocal(p), "sh", "-c", "cat > composer.local.json"); err != nil {
		return fmt.Errorf("writing composer.local.json failed: %w", err)
	}
	if _, err := compose.Exec("", "composer", "update", "--no-interaction", "--no-progress"); err != nil {
		return fmt.Errorf("composer update failed: %w", err)
	}

	a.progress("installing MediaWiki")
	installArgs := mediawiki.InstallArgs(p, mediawiki.Database(env.Database), env.Port)
	if _, err := compose.Exec("", append([]string{"php"}, installArgs...)...); err != nil {
		return fmt.Errorf("installing MediaWiki failed: %w", err)
	}
	if _, err := compose.Exec(mediawiki.Settings(p, env.ID), "sh", "-c", "cat >> LocalSettings.php"); err != nil {
		return fmt.Errorf("extending LocalSettings.php failed: %w", err)
	}

	env.Created = true
	return a.store.Save(*env)
}

// finish runs on every start, so that running start again retries it after the cause of a failure is fixed.
func (a app) finish(env environment.Env) error {
	a.progress("running update.php")
	if _, err := a.compose(env.ID).Exec("", "php", "maintenance/run.php", "update", "--quick"); err != nil {
		return fmt.Errorf("update.php failed; %s: %w", retryAdvice, err)
	}

	a.progress("waiting for the Main Page")
	return waitForMainPage(env.Port)
}

func waitForMainPage(port int) error {
	request, err := http.NewRequest(http.MethodGet, mainPageURL(port), nil)
	if err != nil {
		return err
	}
	// Connect where the port is published, keeping the Host header, since localhost may resolve to ::1.
	request.URL.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	client := http.Client{Timeout: 10 * time.Second}
	deadline := time.Now().Add(mainPageTimeout)
	var lastProblem string

	for time.Now().Before(deadline) {
		response, err := client.Do(request)
		if err != nil {
			lastProblem = err.Error()
		} else {
			body, _ := io.ReadAll(response.Body)
			_ = response.Body.Close()

			problem, retry := mediawiki.PageProblem(response.StatusCode, string(body))
			if problem == "" {
				return nil
			}
			if !retry {
				return fmt.Errorf("the Main Page does not work; %s\n%s", retryAdvice, problem)
			}
			lastProblem = problem
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("the Main Page did not load within %s; %s\n%s", mainPageTimeout, retryAdvice, lastProblem)
}
