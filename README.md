# mwenv

mwenv starts a disposable MediaWiki for the extension or skin in the current git worktree. Each worktree gets its own
containers, database and port, so you can run several at once, for example one per coding agent. The worktree is the
live code: edits show on the next reload.

You need Docker and git.

## Install

Download the binary for your platform from the latest release and put it on your `PATH`:

```sh
gh release download --repo ProfessionalWiki/mwenv --pattern mwenv-darwin-arm64 --output ~/.local/bin/mwenv
chmod +x ~/.local/bin/mwenv
```

The other binaries are `mwenv-darwin-amd64`, `mwenv-linux-amd64` and `mwenv-linux-arm64`. To build from source
instead, run `go build -o bin/mwenv ./cmd/mwenv`.

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

Pushing a tag such as `v1.0.0` makes CI attach the binaries to a new release.
