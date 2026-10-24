package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/ProfessionalWiki/mwenv/internal/docker"
	"github.com/ProfessionalWiki/mwenv/internal/environment"
	"github.com/ProfessionalWiki/mwenv/internal/project"
)

const usage = `mwenv starts a disposable MediaWiki for the extension or skin in the current git worktree.

Usage:
  mwenv start [--mw <version>] [--php <version>] [--db mariadb|sqlite] [--json]
  mwenv run [--] <command> [<args>...]
  mwenv status [--json]
  mwenv ls [--json]
  mwenv stop
  mwenv logs [--follow]
  mwenv destroy [<id>...]
  mwenv rebuild [--mw <version>] [--php <version>]
  mwenv version

start creates the environment if needed, starts it, runs update.php and waits for the Main Page.
  --mw   1.45 (REL1_45 branch), 1.45.2 (release tag), master, or another branch or tag. Default: 1.43
  --php  PHP version. Default: 8.3
  --db   mariadb or sqlite. Default: mariadb
  Flags apply when the environment is created; destroy it to change them.

run executes a command in the web container, in the directory matching the current one.
MediaWiki is installed at /var/www/html/w; install and configure other extensions through run.

rebuild builds the images for --mw and --php again (same values and defaults as start), from the newest commits of
the MediaWiki branch and the newest PHP base image. Existing environments keep their images; destroy and start one
again to use the new ones.
`

type app struct {
	store  environment.Store
	user   docker.User
	stdout io.Writer
	stderr io.Writer
}

func Main(args []string) int {
	a := app{
		store:  environment.NewStore(home()),
		user:   docker.ContainerUser(os.Getuid(), os.Getgid()),
		stdout: os.Stdout,
		stderr: os.Stderr,
	}

	exitCode, err := a.dispatch(args)
	if err != nil {
		fmt.Fprintf(a.stderr, "mwenv: %v\n", err)
		if exitCode == 0 {
			exitCode = 1
		}
	}
	return exitCode
}

func home() string {
	if configured := os.Getenv("MWENV_HOME"); configured != "" {
		return configured
	}

	userHome, err := os.UserHomeDir()
	if err != nil {
		return ".mwenv"
	}
	return filepath.Join(userHome, ".mwenv")
}

func (a app) dispatch(args []string) (int, error) {
	if len(args) == 0 {
		fmt.Fprint(a.stderr, usage)
		return 2, nil
	}

	command, args := args[0], args[1:]

	switch command {
	case "start":
		return 0, a.start(args)
	case "run":
		return a.run(args)
	case "status":
		return 0, a.status(args)
	case "ls":
		return 0, a.list(args)
	case "stop":
		return 0, a.stop(args)
	case "logs":
		return a.logs(args)
	case "destroy":
		return 0, a.destroy(args)
	case "rebuild":
		return 0, a.rebuild(args)
	case "help", "-h", "--help":
		fmt.Fprint(a.stdout, usage)
		return 0, nil
	case "version", "--version":
		fmt.Fprintln(a.stdout, version())
		return 0, nil
	}

	return 2, fmt.Errorf("unknown command %q; run mwenv help", command)
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return info.Main.Version
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string) error {
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%s: %w", flags.Name(), err)
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("%s: unexpected argument %q", flags.Name(), flags.Arg(0))
	}
	return nil
}

func currentProject() (project.Project, error) {
	dir, err := os.Getwd()
	if err != nil {
		return project.Project{}, err
	}
	return project.Detect(dir)
}

func envID(p project.Project) string {
	return environment.ID(p.Name, p.Root)
}

func (a app) compose(id string) docker.Compose {
	return docker.Compose{File: a.store.ComposeFile(id), User: a.user}
}

func (a app) progress(format string, args ...any) {
	fmt.Fprintf(a.stderr, "mwenv: "+format+"\n", args...)
}
