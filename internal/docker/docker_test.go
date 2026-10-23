package docker

import "testing"

func TestPodmanCommandIsPodman(t *testing.T) {
	assertEngine(t, Podman, "Client:        Podman Engine\nVersion:       5.8.7\nAPI Version:   5.8.7\n")
}

func TestDockerCommandOnPodmanSocketIsPodman(t *testing.T) {
	assertEngine(t, Podman, "Client: Docker Engine - Community\n Version: 28.5.1\n\n"+
		"Server: linux/amd64/fedora-44\n Podman Engine:\n  Version: 5.8.7\n")
}

func TestDockerEngineIsDocker(t *testing.T) {
	assertEngine(t, Docker, "Client: Docker Engine - Community\n Version: 28.5.1\n\n"+
		"Server: Docker Engine - Community\n Engine:\n  Version: 28.5.1\n")
}

func assertEngine(t *testing.T, expected Engine, version string) {
	t.Helper()
	if actual := engineFromVersion(version); actual != expected {
		t.Errorf("expected %s, got %s", expected, actual)
	}
}
