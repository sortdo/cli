#!/bin/sh
# Installs sortdo, Sort's command line, on macOS and Linux:
#
#   curl -fsSL https://sort.do/install | bash
#
# It downloads the release for this system from https://github.com/sortdo/cli/releases, checks its SHA-256 against
# the release's checksums.txt, and puts the program in ~/.local/bin (no sudo). Read it before you run it; it is short.
#
#   SORTDO_VERSION=1.2.3          Install this version instead of the latest.
#   SORTDO_INSTALL_DIR=<folder>   Install here instead of ~/.local/bin.
#   SORTDO_NO_MODIFY_PATH=1       Do not add the folder to PATH in your shell profile.
#   SORTDO_INSTALL_SKILL=1 or 0   Install the agent skill without asking, or skip it.
#   SORTDO_RELEASES_URL=<url>     Download from this mirror of github.com/sortdo/cli instead (for tests and mirrors).
set -eu

REPO="${SORTDO_RELEASES_URL:-https://github.com/sortdo/cli}"
VERSION="${SORTDO_VERSION:-latest}"
INSTALL_DIR="${SORTDO_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
fail() { printf 'sortdo install: %s\n' "$*" >&2; exit 1; }

download() {
    if command -v curl >/dev/null 2>&1; then
        curl -fsSL --proto '=https' --tlsv1.2 --retry 3 -o "$2" "$1"
    elif command -v wget >/dev/null 2>&1; then
        wget -q -O "$2" "$1"
    else
        fail "curl or wget is needed."
    fi
}

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    elif command -v openssl >/dev/null 2>&1; then
        openssl dgst -sha256 "$1" | awk '{print $NF}'
    else
        fail "sha256sum, shasum or openssl is needed to check the download."
    fi
}

detect_target() {
    case "$(uname -s)" in
        Darwin) os=darwin ;;
        Linux) os=linux ;;
        MINGW* | MSYS* | CYGWIN*) fail "On Windows, run in PowerShell: irm https://sort.do/install.ps1 | iex" ;;
        *) fail "$(uname -s) is not supported. Use npm instead: npm install -g sortdo" ;;
    esac
    case "$(uname -m)" in
        x86_64 | amd64) arch=amd64 ;;
        arm64 | aarch64) arch=arm64 ;;
        *) fail "$(uname -m) processors are not supported." ;;
    esac
    # A terminal that runs under Rosetta on Apple silicon reports x86_64; the native program is faster.
    if [ "$os" = darwin ] && [ "$arch" = amd64 ] && [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || echo 0)" = 1 ]; then
        arch=arm64
    fi
}

add_to_path() {
    case ":$PATH:" in *":$INSTALL_DIR:"*) return 0 ;; esac
    line="export PATH=\"$INSTALL_DIR:\$PATH\""
    if [ "${SORTDO_NO_MODIFY_PATH:-0}" = 1 ]; then
        say "Add sortdo to your PATH: $line"
        return 0
    fi
    case "$(basename "${SHELL:-sh}")" in
        zsh) profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
        bash) if [ "$(uname -s)" = Darwin ]; then profile="$HOME/.bash_profile"; else profile="$HOME/.bashrc"; fi ;;
        fish) profile="$HOME/.config/fish/conf.d/sortdo.fish"; line="fish_add_path \"$INSTALL_DIR\"" ;;
        *) profile="$HOME/.profile" ;;
    esac
    if ! grep -Fq "$INSTALL_DIR" "$profile" 2>/dev/null; then
        mkdir -p "$(dirname "$profile")"
        printf '\n# sortdo\n%s\n' "$line" >>"$profile"
        say "Added $INSTALL_DIR to PATH in $profile. Open a new terminal, or run: $line"
    fi
}

offer_skill() {
    program="$1"
    answer="${SORTDO_INSTALL_SKILL:-}"
    # The script itself comes through a pipe, so a question goes to the terminal, when there is one.
    if [ -z "$answer" ] && [ -t 1 ] && { : </dev/tty; } 2>/dev/null; then
        printf 'Teach your coding agents (Claude Code, Codex, Cursor and others) to use sortdo? [Y/n] '
        read -r reply </dev/tty || reply=n
        case "$reply" in n* | N*) answer=0 ;; *) answer=1 ;; esac
    fi
    if [ "$answer" = 1 ]; then
        "$program" skill install || say "Could not install the agent skill. Run later: sortdo skill install"
    else
        say "For coding agents, run: sortdo skill install"
    fi
}

main() {
    detect_target
    asset="sortdo_${os}_${arch}.tar.gz"
    if [ "$VERSION" = latest ]; then
        base="$REPO/releases/latest/download"
    else
        base="$REPO/releases/download/v${VERSION#v}"
    fi
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT INT TERM

    say "Downloading sortdo ($os $arch)…"
    download "$base/checksums.txt" "$tmp/checksums.txt" || fail "Could not download $base/checksums.txt"
    download "$base/$asset" "$tmp/$asset" || fail "Could not download $base/$asset"
    expected="$(awk -v name="$asset" '$2 == name || $2 == "*" name {print $1}' "$tmp/checksums.txt")"
    [ -n "$expected" ] || fail "checksums.txt has no entry for $asset."
    [ "$(sha256 "$tmp/$asset")" = "$expected" ] || fail "The download does not match its checksum. Nothing was installed."

    tar -xzf "$tmp/$asset" -C "$tmp" sortdo
    mkdir -p "$INSTALL_DIR"
    chmod 755 "$tmp/sortdo"
    mv -f "$tmp/sortdo" "$INSTALL_DIR/sortdo"
    say "Installed $("$INSTALL_DIR/sortdo" --version) in $INSTALL_DIR/sortdo"

    add_to_path
    offer_skill "$INSTALL_DIR/sortdo"
    say ""
    say "Next: sortdo login"
}

main "$@"
