#!/usr/bin/env bash
# Installs the latest Bun release (linux-x64) and puts it on the job's PATH.
# The archive is checked against the release's SHASUMS256.txt.
set -euo pipefail
asset=bun-linux-x64
tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' https://github.com/oven-sh/bun/releases/latest)"
tag="${tag##*/}"
base="https://github.com/oven-sh/bun/releases/download/$tag"
dir="${RUNNER_TEMP:?}/bun"
mkdir -p "$dir" && cd "$dir"
curl -fsSL -o "$asset.zip" "$base/$asset.zip"
curl -fsSL -o SHASUMS256.txt "$base/SHASUMS256.txt"
grep " $asset.zip\$" SHASUMS256.txt | sha256sum -c -
unzip -q -o "$asset.zip"
echo "$dir/$asset" >> "$GITHUB_PATH"
"$dir/$asset/bun" --version
