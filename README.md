# sortdo

Sort from the command line. Search, read and write your team's knowledge in [Sort](https://trysort.com), as you, with your permissions. It is built for people and for coding agents: in a terminal, results are readable tables; for pipes and agents, they are plain text; `--json` prints data; and every tool on the Sort server is a command.

## Install

```sh
curl -fsSL https://sort.do/install | bash      # macOS and Linux
irm https://sort.do/install.ps1 | iex          # Windows PowerShell
npm install -g sortdo                          # anywhere with Node.js 18 or later
brew install sortdo/tap/sortdo                 # Homebrew on macOS
winget install Sort.sortdo                     # winget on Windows
```

The install scripts download the release for your system from this repository, check its SHA-256 against `checksums.txt`, and install into your user folder (no sudo). Read them first: [install.sh](install/install.sh), [install.ps1](install/install.ps1). `sortdo update` updates a scripted install, and `sortdo uninstall` removes sortdo.

## Start

```sh
sortdo login                 # opens the browser; allow Sort CLI when the page shows the same code
sortdo search onboarding     # titles and URLs
sortdo read <page id or URL> # a page as Markdown
sortdo teams --help          # every tool is a command, with actions and options
sortdo skill install         # teach Claude Code, Codex, Cursor and other agents to use sortdo
```

A coding agent runs `sortdo login` without a terminal: it gets a link to give you and `sortdo login poll` to wait for your answer. Actions that cannot be undone or that reach other people wait for your approval in Sort.

Documentation: https://docs.trysort.com/cli

## Security

- sortdo signs in with the OAuth device grant as Sort's own app. The sign-in page marks it as made by Sort and shows the code from your terminal.
- Tokens are kept in the OS keychain (macOS Keychain, Windows Credential Manager, Secret Service on Linux), or in `~/.config/sortdo/credentials.json` with mode 0600 when there is none.
- On macOS, the program is signed with Sort's Developer ID (Sort Works Ltd, `KDLWZ98LA3`) and notarized by Apple. Check it with `codesign -dv "$(command -v sortdo)"`.
- Releases are built by the workflow in this repository. Check one with `gh attestation verify <file> --repo sortdo/cli`.
- Each release's `checksums.txt` is signed with the release key, and `sortdo update` refuses a release whose signature does not check out. Check one by hand with `minisign -Vm checksums.txt -P RWQsuyzz4q/CJYGDrRz9vMHXgekMewZ+HVM3ux70Ft+oaNjrhxfCL+KL`.
- Report a vulnerability to security@trysort.com.

## Development

Each release is published here with its full source. Build it with Go 1.27 or later: `go build .` and `go test ./...`.

MIT License.
