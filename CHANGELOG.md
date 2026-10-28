# Changelog

What changed in each sortdo release, newest first.

<!-- Every change to sortdo adds a line under Unreleased, written for users. A release turns it into the version's notes. -->

## Unreleased

## 0.1.0 (2026-10-28)

The first release of sortdo, Sort's command line.

- Sign in with `sortdo login`. In a terminal it opens your browser; a coding agent gets a link to give you and `sortdo login poll` to wait for your answer.
- Every Sort tool is a command, with actions and options from the server: `sortdo teams list`, `sortdo comments add`, `sortdo inbox list` and more. New tools appear without an update.
- `sortdo search`, `sortdo read`, `sortdo pages create` and `sortdo pages edit` for everyday work. `--json` prints the data for scripts.
- `sortdo skill install` teaches your coding agents to use sortdo.
- Your sign-in stays in your computer's keychain. `sortdo update` installs a new version after it checks the download.
- Install with `curl -fsSL https://sort.do/install | bash`, `irm https://sort.do/install.ps1 | iex`, npm, Homebrew or winget.
