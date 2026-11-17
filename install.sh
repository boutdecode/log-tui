#!/usr/bin/env bash
# log-tui installer.
#   curl -fsSL https://raw.githubusercontent.com/boutdecode/log-tui/main/install.sh | bash
#
# Environment:
#   LOG_TUI_VERSION      version to install, e.g. v1.2.0 (default: latest release)
#   LOG_TUI_INSTALL_DIR  target directory (default: /usr/local/bin if writable, else ~/.local/bin)
set -euo pipefail

REPO="boutdecode/log-tui"
BIN="log-tui"

say() { printf '%s\n' "$*"; }
die() { printf 'install: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "'$1' is required"; }

need curl
need tar
need uname

case "$(uname -s)" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) die "unsupported OS: $(uname -s) (Linux and macOS only)" ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

version="${LOG_TUI_VERSION:-}"
if [ -z "$version" ]; then
  version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1 || true)"
fi

# Pick the install directory.
dir="${LOG_TUI_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir"

install_from_source() {
  command -v go >/dev/null 2>&1 || die "no release found and Go is not installed; see https://github.com/$REPO/releases"
  need git
  say "No release found, building from source with Go…"
  src="$(mktemp -d)"
  trap 'rm -rf "$src"' EXIT
  git clone --depth 1 "https://github.com/$REPO.git" "$src/repo" >/dev/null 2>&1 || die "git clone failed"
  (cd "$src/repo" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o "$dir/$BIN" "./cmd/$BIN")
}

if [ -z "$version" ]; then
  install_from_source
else
  asset="${BIN}_${version#v}_${os}_${arch}.tar.gz"
  base="https://github.com/$REPO/releases/download/$version"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT

  say "Downloading $BIN $version ($os/$arch)…"
  curl -fsSL "$base/$asset" -o "$tmp/$asset" || die "download failed: $base/$asset"

  # Verify the checksum when the release publishes one.
  if curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" 2>/dev/null; then
    expected="$(grep " $asset\$" "$tmp/checksums.txt" | awk '{print $1}')"
    if command -v sha256sum >/dev/null 2>&1; then actual="$(sha256sum "$tmp/$asset" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then actual="$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')"
    else actual=""; fi
    if [ -n "$expected" ] && [ -n "$actual" ] && [ "$expected" != "$actual" ]; then
      die "checksum mismatch for $asset"
    fi
  fi

  tar -xzf "$tmp/$asset" -C "$tmp" "$BIN"
  install -m 0755 "$tmp/$BIN" "$dir/$BIN"
fi

say "Installed $BIN to $dir/$BIN"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "Note: $dir is not in your PATH. Add it, e.g.: export PATH=\"$dir:\$PATH\"" ;;
esac
say "Run: $BIN --help"
