# mwenv

mwenv starts a disposable MediaWiki for the extension or skin in the current git worktree. Each worktree gets its own
containers, database and port, so you can run several at once, for example one per coding agent. The worktree is the
live code: edits show on the next reload.

You need Docker or Podman, and git.

## Install

With Go 1.21 or later:

```sh
go install github.com/ProfessionalWiki/mwenv/cmd/mwenv@latest
```

This puts `mwenv` in `$(go env GOPATH)/bin`, usually `~/go/bin`; make sure that directory is on your `PATH`.

Without Go, download the binary for your platform from the latest release into a directory on your `PATH`:

```sh
curl -fsSL --create-dirs -o ~/.local/bin/mwenv "https://github.com/ProfessionalWiki/mwenv/releases/latest/download/mwenv-$(uname -s)-$(uname -m)"
chmod +x ~/.local/bin/mwenv
```

Releases have binaries for macOS and Linux, on amd64 and arm64. On macOS, download with curl, not a browser: macOS
blocks unsigned binaries that a browser downloaded.

To upgrade, run the install command again. `mwenv version` prints the version you have.

## Use

Run mwenv anywhere in an extension or skin repository, or in a worktree of one:

```sh
mwenv start                       # MediaWiki 1.43, PHP 8.3, MariaDB
mwenv start --mw 1.45 --php 8.4   # flags apply when the environment is created
mwenv run -- composer phpunit     # runs in the container, in the matching directory
mwenv status --json
mwenv destroy
```

`start` prints the wiki's URL and the admin login. The first `start` for a MediaWiki and PHP version builds images,
which takes a few minutes. After that, a new environment is ready in well under a minute. `mwenv help` lists all
commands.

Images are reused, so a branch such as `1.45` or `master` stays at the commit it had when its image was built. To get
newer commits, remove the image (`docker image ls mwenv-core`, then `docker image rm`); environments created after
that use a fresh one.

## Add other extensions

mwenv installs only MediaWiki core and your extension or skin. You install anything else through `mwenv run`.
MediaWiki is at `/var/www/html/w` in the container:

```sh
mwenv run -- git clone --depth 1 -b REL1_43 https://gerrit.wikimedia.org/r/mediawiki/extensions/AdminLinks /var/www/html/w/extensions/AdminLinks
mwenv run -- sh -c 'echo "wfLoadExtension( \"AdminLinks\" );" >> /var/www/html/w/LocalSettings.php'
mwenv start
```

`start` runs `update.php` and waits for the Main Page every time. If your extension requires another one, the first
`start` fails with MediaWiki's error. Install the missing extension this way, then run `start` again.

## Develop mwenv

You need Go 1.21 or later, which downloads the Go version mwenv needs. Run the tests with `go test ./...`.

Pushing a tag such as `v1.0.0` makes CI attach the binaries to a new release. Never move or reuse a tag: `go install`
fetches through Go's module proxy, which keeps serving the commit it first saw for each tag.
