package docker_test

import (
	"encoding/json"
	"testing"

	"github.com/ProfessionalWiki/mwenv/internal/docker"
	"github.com/ProfessionalWiki/mwenv/internal/environment"
	"github.com/ProfessionalWiki/mwenv/internal/project"
)

type composeFile struct {
	Services map[string]struct {
		Environment map[string]string `json:"environment"`
		Ports       []string          `json:"ports"`
		Volumes     []struct {
			Source   string `json:"source"`
			Target   string `json:"target"`
			ReadOnly bool   `json:"read_only"`
		} `json:"volumes"`
	} `json:"services"`
}

func TestWorktreeIsMountedAtItsInstallationPath(t *testing.T) {
	file := compose(t, newEnv("mariadb"), pageApprovals(""))

	assertHasVolume(t, file, "/home/dev/PageApprovals", "/var/www/html/w/extensions/PageApprovals", false)
}

func TestPortIsBoundToLoopbackOnly(t *testing.T) {
	file := compose(t, newEnv("mariadb"), pageApprovals(""))

	assertSame(t, "127.0.0.1:25094:80", file.Services["web"].Ports[0])
}

func TestMariaDbEnvironmentHasDatabaseService(t *testing.T) {
	file := compose(t, newEnv("mariadb"), pageApprovals(""))

	if _, found := file.Services["db"]; !found {
		t.Error("expected a db service")
	}
}

func TestSqliteEnvironmentHasNoDatabaseService(t *testing.T) {
	file := compose(t, newEnv("sqlite"), pageApprovals(""))

	if _, found := file.Services["db"]; found {
		t.Error("expected no db service")
	}
}

func TestMainCheckoutMountsNoGitDirectory(t *testing.T) {
	file := compose(t, newEnv("mariadb"), pageApprovals(""))

	for _, volume := range file.Services["web"].Volumes {
		if volume.ReadOnly {
			t.Errorf("did not expect a read-only mount, got %+v", volume)
		}
	}
}

func TestLinkedWorktreeMountsMainGitDirReadOnly(t *testing.T) {
	file := compose(t, newEnv("mariadb"), pageApprovals("/home/dev/main/.git"))

	assertHasVolume(t, file, "/home/dev/main/.git", "/home/dev/main/.git", true)
}

func TestApacheRunsAsContainerUser(t *testing.T) {
	file := compose(t, newEnv("mariadb"), pageApprovals(""))

	assertSame(t, "#501", file.Services["web"].Environment["APACHE_RUN_USER"])
}

func TestHostUserRunsAsItselfInContainer(t *testing.T) {
	assertSame(t, docker.User{UID: 501, GID: 20}, docker.ContainerUser(501, 20))
}

func TestRootRunsAsWwwDataInContainer(t *testing.T) {
	assertSame(t, docker.User{UID: 33, GID: 33}, docker.ContainerUser(0, 0))
}

func newEnv(database string) environment.Env {
	return environment.Env{ID: "pageapprovals-d29ed706", Database: database, Port: 25094}
}

func pageApprovals(externalGitDir string) project.Project {
	return project.Project{
		Root:           "/home/dev/PageApprovals",
		Type:           project.Extension,
		Name:           "PageApprovals",
		ExternalGitDir: externalGitDir,
	}
}

func compose(t *testing.T, env environment.Env, p project.Project) composeFile {
	t.Helper()

	content, err := docker.ComposeFile(env, p, "mwenv-core:test", docker.User{UID: 501, GID: 20})
	if err != nil {
		t.Fatal(err)
	}

	var file composeFile
	if err := json.Unmarshal(content, &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func assertHasVolume(t *testing.T, file composeFile, source string, target string, readOnly bool) {
	t.Helper()

	for _, volume := range file.Services["web"].Volumes {
		if volume.Source == source && volume.Target == target && volume.ReadOnly == readOnly {
			return
		}
	}
	t.Errorf("expected a volume from %s at %s (read-only: %v) in %+v", source, target, readOnly, file.Services["web"].Volumes)
}

func assertSame[T comparable](t *testing.T, expected T, actual T) {
	t.Helper()
	if expected != actual {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}
