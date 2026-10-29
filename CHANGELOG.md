# Changelog

What changed in each sortdo release, newest first.

<!-- Every change to sortdo adds a line under Unreleased, written for users. A release turns it into the version's notes. -->

## Unreleased

## 0.1.4 (2026-10-29)

- Every release is now tested on Linux, macOS and Windows before it is published, and installed and tried on each system after.

## 0.1.3 (2026-10-28)

- `sortdo update` now checks that each release is signed with sortdo's release key before it trusts any download. A release without a valid signature is refused, and nothing changes. You can check a release yourself with `minisign`; see the install docs.
- sortdo is now built with the newest version of Go, with its latest security fixes.

## 0.1.2 (2026-10-28)

- In a terminal, results are now tables and readable lines, not raw JSON. Pipes, scripts and coding agents get the same output as before, and `--json` still prints JSON.
- sortdo has a new look in the terminal: the Sort logo, results in framed panels, colors, and an animation while it waits. Set `NO_COLOR=1` to turn the colors off.
- Run `sortdo` on its own to choose a command with the arrow keys, or type to filter. A command without its action, such as `sortdo teams`, lets you choose the action. sortdo shows the full command before it runs it. Menus open only in a terminal, never for scripts or coding agents; `SORTDO_NO_INPUT=1` turns them off.
- `sortdo uninstall` signs out and removes sortdo, its settings and the agent skill.
- `sortdo status` and `sortdo update` now warn when more than one copy of sortdo is installed.
- `SORTDO_TOKEN` is now sent only to its own server: the one in `SORTDO_SERVER`, or trysort.com. A `--server` option cannot send it anywhere else.
- sortdo uses plain `http://` only for a server on this computer, such as `localhost`. Other servers need `https://`.
- `sortdo update` gives npm, Homebrew and winget users their update command at once, without a download first.
- A workspace command such as `sortdo teams list` now says to sign in when you are not signed in.
- The install script no longer tries a download again without its HTTPS-only settings.
- The npm packages are now published with npm trusted publishing, so no long-lived npm token can publish them.

## 0.1.1 (2026-10-28)

- sortdo is now on npm: `npm install -g sortdo`.

## 0.1.0 (2026-10-28)

The first release of sortdo, Sort's command line.

- Sign in with `sortdo login`. In a terminal it opens your browser; a coding agent gets a link to give you and `sortdo login poll` to wait for your answer.
- Every Sort tool is a command, with actions and options from the server: `sortdo teams list`, `sortdo comments add`, `sortdo inbox list` and more. New tools appear without an update.
- `sortdo search`, `sortdo read`, `sortdo pages create` and `sortdo pages edit` for everyday work. `--json` prints the data for scripts.
- `sortdo skill install` teaches your coding agents to use sortdo.
- Your sign-in stays in your computer's keychain. `sortdo update` installs a new version after it checks the download.
- Install with `curl -fsSL https://sort.do/install | bash`, `irm https://sort.do/install.ps1 | iex`, npm, Homebrew or winget.
