#!/bin/sh
#
# Installs the `ward` CLI.
#
#   curl -fsSL https://raw.githubusercontent.com/Tobe0504/Warder/main/install.sh | sh
#
# Pin a version with WARD_VERSION, and choose where it lands with WARD_INSTALL_DIR:
#
#   WARD_VERSION=v0.2.0 WARD_INSTALL_DIR="$HOME/.local/bin" ./install.sh
#
# POSIX sh on purpose: this has to run on a stock macOS shell, a slim Debian
# container, and an Alpine CI image without anyone installing bash first.

set -eu

REPO="Tobe0504/Warder"
BINARY="ward"

say()  { printf '%s\n' "$*"; }
fail() { printf 'install: %s\n' "$*" >&2; exit 1; }

need() {
    command -v "$1" >/dev/null 2>&1 || fail "this needs $1, which is not installed"
}

need uname
need tar
need mktemp

# curl or wget, whichever is present.
if command -v curl >/dev/null 2>&1; then
    fetch() { curl -fsSL "$1" -o "$2"; }
    fetch_stdout() { curl -fsSL "$1"; }
elif command -v wget >/dev/null 2>&1; then
    fetch() { wget -qO "$2" "$1"; }
    fetch_stdout() { wget -qO- "$1"; }
else
    fail "this needs curl or wget, and neither is installed"
fi

# ---------------------------------------------------------------------------
# Which build
# ---------------------------------------------------------------------------

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
    darwin|linux) ;;
    *) fail "no build for $os. Build from source: go install github.com/$REPO/cmd/$BINARY@latest" ;;
esac

arch=$(uname -m)
case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) fail "no build for $arch. Build from source: go install github.com/$REPO/cmd/$BINARY@latest" ;;
esac

version="${WARD_VERSION:-}"
if [ -z "$version" ]; then
    say "Looking up the latest release…"
    # The redirect from /releases/latest names the tag, which avoids parsing
    # JSON without jq.
    version=$(fetch_stdout "https://api.github.com/repos/$REPO/releases/latest" \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n 1)
    [ -n "$version" ] || fail "could not determine the latest version. Set WARD_VERSION to pick one."
fi

archive="${BINARY}_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

# ---------------------------------------------------------------------------
# Download and verify
# ---------------------------------------------------------------------------

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading $BINARY $version ($os/$arch)…"
fetch "$base/$archive" "$tmp/$archive" || fail "could not download $archive"
fetch "$base/checksums.txt" "$tmp/checksums.txt" || fail "could not download checksums.txt"

# Verifying is not optional. This binary will hold a credential that reaches
# every secret its identity is granted, so a tampered download is not a
# cosmetic problem.
# Matched with awk rather than grep. The checksum file lists names as
# "./name", and a grep alternation written with \| is a GNU extension that
# BSD grep: the one on every Mac: treats as a literal, so the lookup found
# nothing and the install failed on exactly the platform most developers use.
expected=$(awk -v want="$archive" '$2 == want || $2 == "./" want { print $1; exit }' "$tmp/checksums.txt")
[ -n "$expected" ] || fail "no checksum published for $archive"

if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
else
    fail "this needs sha256sum or shasum to verify the download"
fi

[ "$actual" = "$expected" ] || fail "checksum mismatch, refusing to install
  expected $expected
  got      $actual"

tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/$BINARY" ] || fail "the archive did not contain $BINARY"
chmod +x "$tmp/$BINARY"

# ---------------------------------------------------------------------------
# Where it lands
# ---------------------------------------------------------------------------

target="${WARD_INSTALL_DIR:-}"
if [ -z "$target" ]; then
    # A writable directory that is *already on PATH*, preferred over one that
    # merely exists. An earlier version checked only for existence and picked
    # ~/.local/bin, which plenty of machines have without ever putting it on
    # PATH, so the install reported success and the command was not found.
    #
    # /usr/local/bin is deliberately last: it is root-owned on Apple Silicon,
    # and a build written there with sudo is one nobody can update later
    # without sudo.
    for candidate in "$HOME/.local/bin" /opt/homebrew/bin "$HOME/bin" /usr/local/bin; do
        if [ -d "$candidate" ] && [ -w "$candidate" ]; then
            case ":$PATH:" in
                *":$candidate:"*) target="$candidate"; break ;;
            esac
        fi
    done

    # Nothing on PATH is writable. Fall back to any writable candidate and let
    # the closing message explain what to add.
    if [ -z "$target" ]; then
        for candidate in "$HOME/.local/bin" /opt/homebrew/bin "$HOME/bin"; do
            if [ -d "$candidate" ] && [ -w "$candidate" ]; then
                target="$candidate"
                break
            fi
        done
    fi
fi
[ -n "$target" ] || target="$HOME/.local/bin"

mkdir -p "$target"
[ -w "$target" ] || fail "$target is not writable. Set WARD_INSTALL_DIR to somewhere you own."

mv "$tmp/$BINARY" "$target/$BINARY"

say ""
say "Installed $BINARY $version to $target/$BINARY"

case ":$PATH:" in
    *":$target:"*) say "Run: $BINARY --help" ;;
    *)
        say ""
        say "$target is not on your PATH. Add it:"
        say "  echo 'export PATH=\"$target:\$PATH\"' >> ~/.zshrc && exec zsh"
        ;;
esac
