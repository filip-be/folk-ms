#!/usr/bin/env bash
# Downloads the pinned Go and Bun into .tools/ and verifies their SHA-256
# against tools/checksums.txt. Usage: tools/fetch.sh [go|bun]...
# Afterwards: export PATH="$PWD/.tools/go/bin:$PWD/.tools/bun:$PATH"
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=/dev/null
source "$root/tools/versions.env"
dest="$root/.tools"
mkdir -p "$dest/dl"

case "$(uname -s)" in
  Linux*) os=linux ;;
  Darwin*) os=darwin ;;
  MINGW*|MSYS*|CYGWIN*) os=windows ;;
  *) echo "unsupported OS" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64) goarch=amd64; bunarch=x64 ;;
  aarch64|arm64) goarch=arm64; bunarch=aarch64 ;;
  *) echo "unsupported arch" >&2; exit 1 ;;
esac

sha256() { if command -v sha256sum >/dev/null; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }

fetch() { # name url
  local name="$1" url="$2" file="$dest/dl/$1"
  local want
  want="$(awk -v n="$name" '$2==n {print $1}' "$root/tools/checksums.txt")"
  [ -n "$want" ] || { echo "no checksum for $name in tools/checksums.txt" >&2; exit 1; }
  if [ ! -f "$file" ] || [ "$(sha256 "$file")" != "$want" ]; then
    echo "downloading $url" >&2
    curl -fsSL -o "$file.part" "$url"
    mv "$file.part" "$file"
  fi
  local got
  got="$(sha256 "$file")"
  if [ "$got" != "$want" ]; then rm -f "$file"; echo "checksum mismatch for $name: $got" >&2; exit 1; fi
}

unzip_to() { # archive dir
  if command -v unzip >/dev/null; then unzip -q -o "$1" -d "$2";
  else (cd "$2" && tar -xf "$1"); fi
}

get_go() {
  local ext=tar.gz; [ "$os" = windows ] && ext=zip
  local name="go${GO_VERSION}.${os}-${goarch}.${ext}"
  if [ -x "$dest/go/bin/go" ] || [ -x "$dest/go/bin/go.exe" ]; then
    if "$dest/go/bin/go" version 2>/dev/null | grep -q "go${GO_VERSION} "; then return; fi
  fi
  fetch "$name" "https://go.dev/dl/$name"
  rm -rf "$dest/go"
  if [ "$ext" = zip ]; then unzip_to "$dest/dl/$name" "$dest"; else tar -xzf "$dest/dl/$name" -C "$dest"; fi
}

get_bun() {
  local name="bun-v${BUN_VERSION}-${os}-${bunarch}.zip" asset="bun-${os}-${bunarch}"
  if "$dest/bun/bun" --version 2>/dev/null | grep -qx "$BUN_VERSION"; then return; fi
  fetch "$name" "https://github.com/oven-sh/bun/releases/download/bun-v${BUN_VERSION}/${asset}.zip"
  rm -rf "$dest/bun" "$dest/$asset"
  unzip_to "$dest/dl/$name" "$dest"
  mv "$dest/$asset" "$dest/bun"
}

targets=("$@"); [ ${#targets[@]} -eq 0 ] && targets=(go bun)
for t in "${targets[@]}"; do "get_$t"; done
echo "ok: add $dest/go/bin and $dest/bun to PATH" >&2
