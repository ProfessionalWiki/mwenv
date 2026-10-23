package docker

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const outputTailLines = 40

func output(args ...string) (string, error) {
	return outputWithInput("", args...)
}

// outputWithInput runs docker and returns its combined output. On failure, the error includes the output's last
// lines, which is where docker and the commands it runs explain what went wrong.
func outputWithInput(input string, args ...string) (string, error) {
	cmd := exec.Command("docker", args...)
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}

	combined, err := cmd.CombinedOutput()
	if err == nil {
		return string(combined), nil
	}

	if tail := lastLines(strings.TrimSpace(string(combined)), outputTailLines); tail != "" {
		return "", fmt.Errorf("%w\n%s", err, tail)
	}
	return "", err
}

func lastLines(text string, count int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= count {
		return text
	}
	return strings.Join(lines[len(lines)-count:], "\n")
}

// passthrough runs docker attached to this process's standard streams and returns docker's exit code.
func passthrough(args ...string) (int, error) {
	cmd := exec.Command("docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()

	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode(), nil
	}
	return 0, err
}

type Engine string

const (
	Docker Engine = "docker"
	Podman Engine = "podman"
)

// DetectEngine finds Podman both when the docker command is Podman and when it is Docker using Podman's socket.
func DetectEngine() Engine {
	version, _ := output("version")
	return engineFromVersion(version)
}

func engineFromVersion(version string) Engine {
	if strings.Contains(version, "Podman Engine") {
		return Podman
	}
	return Docker
}

// WebContainerStates maps Compose project names to the Docker state of their web container.
func WebContainerStates() (map[string]string, error) {
	listing, err := output(
		"ps", "--all",
		"--filter", "label=com.docker.compose.service=web",
		"--format", `{{.Label "com.docker.compose.project"}}	{{.State}}`,
	)
	if err != nil {
		return nil, err
	}

	states := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(listing), "\n") {
		if project, state, found := strings.Cut(line, "\t"); found {
			states[project] = state
		}
	}
	return states, nil
}
