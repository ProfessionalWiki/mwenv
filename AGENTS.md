# AGENTS.md

Every change must keep mwenv environments cheap to start and to throw away, so that many can run in parallel, one per
git worktree.

## Design constraints

- One binary for macOS and Linux, using only the Go standard library. It shells out to `git` and `docker`, which may
  be Podman.
- mwenv reads no configuration file of its own from the worktree; users choose MediaWiki, PHP and the database
  through `start` flags. Don't add flags for other extensions, settings or services: users install those through
  `mwenv run`.
- A `start` flag applies only when an environment is created. Store a new one in `Env` and add it to
  `checkUnchangedOptions`; environments created before the flag have the empty value there.
- `LocalSettings.php` is written once, when the environment is created. After that it is the user's to edit, and
  `start` must not rewrite it.
- Results go to stdout, progress and errors to stderr. The `--json` output of `start`, `status` and `ls` is a
  contract.
- A command that changes an environment holds its lock (`Store.TryLock`) throughout.
- The `usage` text in `internal/cli/cli.go` and `README.md` document the commands, flags and defaults by hand; update
  both with them.

## Existing environments

An environment keeps what it was created with: its `env.json`, and a `compose.json` that pins the image tag and the
mount paths. A change to `ComposeFile`, the images or `InstallPath` reaches only new environments.

- A field you add to `Env` is empty in existing environments.
- Environments are found by an ID derived from the worktree path (`environment.ID`), and their containers by the
  `mwenv-` Compose project prefix. Changing either orphans existing environments.
- Image tags hash the files listed in `hashBuildFiles` (`internal/docker/images.go`). Add a new file in
  `internal/docker/build/` there, or images built without it keep being reused.

## Verify a change

- `gofmt -l .` prints nothing, and `go vet ./...` and `go test ./...` pass. CI runs them on Linux only; after changing
  OS-specific code, also run `GOOS=darwin go vet ./...`.
- A test that commits with git ignores the user's git config, as the `git` helper in
  `internal/project/project_test.go` does.
- For a change to the images, the Compose file or the Docker calls, also run the binary you built against a real
  extension, as the `smoke` job in `.github/workflows/CI.yml` does. Set `MWENV_HOME` to a temporary directory, and
  destroy the environment before deleting that directory: images, host ports and the composer cache volume are shared
  with the user's environments. The smoke job uses the default flags; pass `--db`, `--mw` or `--php` when your change
  depends on them.
- CI covers neither Docker on macOS nor rootless Podman with SELinux. When you change mounts, users or ports in
  `ComposeFile`, say in the PR which of them you tested.
