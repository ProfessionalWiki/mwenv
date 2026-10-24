# mwenv

mwenv starts a disposable MediaWiki for the extension or skin in the current git worktree. Each worktree gets its own
containers, database and port, so you can run several at once, for example one per coding agent. The worktree is the
live code: edits show on the next reload.

mwenv runs on Linux and macOS. You need git, and Docker with Compose or Podman with its `docker` command.

## Install

Download the binary from the latest release into `~/.local/bin`, or another directory on your `PATH`:

```sh
curl -fsSL --create-dirs -o ~/.local/bin/mwenv "https://github.com/ProfessionalWiki/mwenv/releases/latest/download/mwenv-$(uname -s)-$(uname -m)"
chmod +x ~/.local/bin/mwenv
```

Or, with Go 1.21 or later, install it into `$(go env GOPATH)/bin`, which must be on your `PATH`:

```sh
go install github.com/ProfessionalWiki/mwenv/cmd/mwenv@latest
```

To upgrade, run the install command again.

## Use

Run mwenv anywhere in an extension or skin repository, or in a worktree of one:

```sh
mwenv start                       # MediaWiki 1.43, PHP 8.3, MariaDB
mwenv start --mw 1.45 --php 8.4   # flags apply when the environment is created
mwenv run -- composer phpunit     # runs in the container, in the matching directory
mwenv status --json
mwenv destroy
```

`start` prints the wiki's URL and the admin login. Creating an environment installs your extension's Composer
dependencies. The first `start` for a MediaWiki and PHP version also builds images, which takes a few minutes; later
environments are ready in under a minute. `mwenv help` lists all commands and flags.

Images built from a branch, such as `--mw 1.45` (`REL1_45`) or `master`, keep the commit the branch had when they were
built. Run `mwenv rebuild` with the `--mw` and `--php` you pass to `start` to rebuild the images from the branch's
newest commits and the newest PHP base image. New environments use them. An existing environment keeps its image until
you destroy it and start it again, which discards its database and your changes in the container.

## Add other extensions

mwenv installs only MediaWiki core and your extension or skin. You install anything else through `mwenv run`.
MediaWiki is at `/var/www/html/w` in the container:

```sh
mwenv run -- git clone --depth 1 -b REL1_43 https://gerrit.wikimedia.org/r/mediawiki/extensions/AdminLinks /var/www/html/w/extensions/AdminLinks
mwenv run -- sh -c 'echo "wfLoadExtension( \"AdminLinks\" );" >> /var/www/html/w/LocalSettings.php'
mwenv start
```

`start` runs `update.php` every time. If your extension requires another one, the first `start` fails with MediaWiki's
error. Install the missing extension this way, then run `start` again.

## Develop mwenv

You need Go 1.21 or later, which downloads the Go version mwenv needs. Run the tests with `go test ./...`.

Pushing a tag such as `v1.0.0` makes CI attach the binaries to a new release. Never move or reuse a tag: Go's module
proxy keeps serving the commit it first saw for each tag.
