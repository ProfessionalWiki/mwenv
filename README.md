# mwenv

mwenv is a CLI that starts a disposable MediaWiki for extension and skin development. Run it in your extension's or
skin's repository; you don't need a MediaWiki installation of your own.

## Features

- You can run several wikis at once, one per git worktree, each with its own database and port.
- The wiki runs your code in place: edits show on the next reload.
- Creating an environment installs your extension's or skin's Composer dependencies.
- You choose the MediaWiki version (`master` included), the PHP version, and MariaDB or SQLite.
- Coding agents can use it unattended: no command asks for input.

## Install

mwenv runs on Linux and macOS. You need git, and Docker with Compose or Podman with its `docker` command.

Download the binary from the latest release into `~/.local/bin`, or another directory on your `PATH`:

```sh
mkdir -p ~/.local/bin
curl -fsSL "https://github.com/ProfessionalWiki/mwenv/releases/latest/download/mwenv-$(uname -s)-$(uname -m).tar.gz" | tar -xz -C ~/.local/bin mwenv
```

Or, with Go 1.21 or later, install it into `$(go env GOPATH)/bin`, which must be on your `PATH`:

```sh
go install github.com/ProfessionalWiki/mwenv/cmd/mwenv@latest
```

Or, with [mise](https://mise.jdx.dev):

```sh
mise use -g github:ProfessionalWiki/mwenv
```

To upgrade, run the install command again.

## Use

Run these anywhere in the repository, or in a worktree of it:

```sh
mwenv start                     # MediaWiki 1.43, PHP 8.3, MariaDB
mwenv run -- composer phpunit   # runs in the container, in the matching directory
mwenv status
mwenv destroy
```

`start` prints the wiki's URL and the admin login. To use other versions, pass flags such as `--mw 1.45 --php 8.4` to
`start`; they apply when the environment is created, so destroy it to change them. The first `start` for a MediaWiki
and PHP version builds images, which takes a few minutes; later environments are ready in under a minute. `mwenv help`
lists all commands and flags.

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

## Add PHP extensions

To use a PHP extension the image lacks, install it as root. `mwenv run` sees it at once; the wiki loads it after you
restart the environment:

```sh
mwenv run --root -- install-php-extensions xdebug
mwenv stop && mwenv start
```

It stays until you destroy the environment. Use `--root` only for changes to the container: on Linux, files it
creates in your worktree do not belong to you.

## Pick up new MediaWiki commits

Images built from a branch, such as `--mw 1.45` (`REL1_45`) or `master`, keep the commit the branch had when they were
built. To rebuild them from the branch's newest commits and the newest PHP base image, run `mwenv rebuild` with the
`--mw` and `--php` you pass to `start`. New environments use the new images; an existing environment keeps its image
until you destroy it and start it again, which discards its database and your changes in the container.
