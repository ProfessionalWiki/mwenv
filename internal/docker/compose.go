package docker

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ProfessionalWiki/mwenv/internal/environment"
	"github.com/ProfessionalWiki/mwenv/internal/mediawiki"
	"github.com/ProfessionalWiki/mwenv/internal/project"
)

const (
	InstallPath         = "/var/www/html/w"
	composerCacheVolume = "mwenv-composer-cache"
	mariaDBImage        = "docker.io/library/mariadb:11.8"
	wwwData             = 33
)

type User struct {
	UID int
	GID int
}

func (u User) String() string {
	return fmt.Sprintf("%d:%d", u.UID, u.GID)
}

// ContainerUser is the user Apache and all commands run as in the container: the host user, so that files created in
// the worktree belong to them, except that root becomes www-data, because Apache refuses to serve pages as root.
func ContainerUser(uid int, gid int) User {
	if uid == 0 {
		return User{UID: wwwData, GID: wwwData}
	}
	return User{UID: uid, GID: gid}
}

func MountPath(p project.Project) string {
	return InstallPath + "/" + p.Dir()
}

func ComposeFile(env environment.Env, p project.Project, image string, user User, engine Engine) ([]byte, error) {
	volumes := []map[string]any{
		{"type": "bind", "source": p.Root, "target": MountPath(p)},
		{"type": "volume", "source": "composer-cache", "target": "/composer-cache"},
	}
	if p.ExternalGitDir != "" {
		volumes = append(volumes, map[string]any{
			"type": "bind", "source": p.ExternalGitDir, "target": p.ExternalGitDir, "read_only": true,
		})
	}

	web := map[string]any{
		"image":   image,
		"command": []string{"sh", "-c", addUserCommand(user) + " && exec apache2-foreground"},
		"ports":   []string{fmt.Sprintf("127.0.0.1:%d:80", env.Port)},
		"volumes": volumes,
		// SELinux denies containers the bind mounts. Relabelling them instead would change the labels of the
		// developer's own files.
		"security_opt": []string{"label=disable"},
		"environment": map[string]string{
			"APACHE_RUN_USER":  fmt.Sprintf("#%d", user.UID),
			"APACHE_RUN_GROUP": fmt.Sprintf("#%d", user.GID),
		},
	}

	// Rootless Podman maps container users to other host users, so the container could not write to the worktree.
	// keep-id maps the host user to itself, but also starts the container as that user, which cannot bind port 80.
	if engine == Podman {
		web["userns_mode"] = "keep-id"
		web["user"] = "0:0"
	}

	services := map[string]any{"web": web}
	fileVolumes := map[string]any{
		"composer-cache": map[string]any{"name": composerCacheVolume, "external": true},
	}

	if mediawiki.Database(env.Database) == mediawiki.MariaDB {
		web["depends_on"] = map[string]any{"db": map[string]string{"condition": "service_healthy"}}
		services["db"] = mariaDBService()
		fileVolumes["db-data"] = map[string]any{}
	}

	return json.MarshalIndent(
		map[string]any{"name": env.ComposeProject(), "services": services, "volumes": fileVolumes},
		"",
		"\t",
	)
}

// addUserCommand gives the user a name in the container. Apache and some tools refuse or warn without one.
func addUserCommand(user User) string {
	return fmt.Sprintf(
		"(getent group %[2]d >/dev/null || echo 'mwenv:x:%[2]d:' >> /etc/group)"+
			" && (getent passwd %[1]d >/dev/null || echo 'mwenv:x:%[1]d:%[2]d:mwenv:/tmp:/bin/sh' >> /etc/passwd)",
		user.UID, user.GID,
	)
}

func mariaDBService() map[string]any {
	return map[string]any{
		"image":   mariaDBImage,
		"command": []string{"--innodb-buffer-pool-size=64M", "--innodb-log-file-size=16M"},
		"environment": map[string]string{
			"MARIADB_ROOT_PASSWORD": mediawiki.DatabaseRootPassword,
		},
		"volumes": []map[string]any{
			{"type": "volume", "source": "db-data", "target": "/var/lib/mysql"},
		},
		"healthcheck": map[string]any{
			"test":     []string{"CMD", "healthcheck.sh", "--connect", "--innodb_initialized"},
			"interval": "1s",
			"timeout":  "5s",
			"retries":  120,
		},
	}
}

type Compose struct {
	File string
	User User
}

func (c Compose) args(args ...string) []string {
	return append([]string{"compose", "--file", c.File}, args...)
}

func (c Compose) Up() error {
	if err := createComposerCacheVolume(); err != nil {
		return err
	}

	_, err := output(c.args("up", "--detach", "--wait", "--quiet-pull")...)
	return err
}

// createComposerCacheVolume accepts an existing volume, as docker volume create does and podman volume create does
// not. Checking only after a failed create keeps parallel starts from racing.
func createComposerCacheVolume() error {
	_, err := output("volume", "create", composerCacheVolume)
	if err != nil && volumeExists(composerCacheVolume) {
		return nil
	}
	return err
}

func volumeExists(name string) bool {
	_, err := output("volume", "inspect", name)
	return err == nil
}

func (c Compose) Start() error {
	_, err := output(c.args("start")...)
	return err
}

func (c Compose) Stop() error {
	_, err := output(c.args("stop")...)
	return err
}

func (c Compose) Down() error {
	_, err := output(c.args("down", "--volumes", "--remove-orphans")...)
	return err
}

// Exec runs a command in the MediaWiki installation directory and returns its output.
func (c Compose) Exec(input string, command ...string) (string, error) {
	return outputWithInput(input, c.execArgs(InstallPath, false, command)...)
}

// ExecAttached runs a command attached to this process's standard streams and returns its exit code. It allocates a
// terminal only when stdin and stdout are both terminals.
func (c Compose) ExecAttached(workdir string, command ...string) (int, error) {
	tty := isTerminal(os.Stdin) && isTerminal(os.Stdout)
	return passthrough(c.execArgs(workdir, tty, command)...)
}

func (c Compose) execArgs(workdir string, tty bool, command []string) []string {
	args := c.args("exec", "--user", c.User.String(), "--workdir", workdir)
	if !tty {
		args = append(args, "--no-TTY")
	}
	return append(append(args, "web"), command...)
}

func (c Compose) Logs(follow bool) (int, error) {
	args := c.args("logs")
	if follow {
		args = append(args, "--follow")
	}
	return passthrough(args...)
}
