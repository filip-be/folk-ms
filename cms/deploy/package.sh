#!/usr/bin/env bash
# Builds the Functions deployment package: cms/deploy/out/cms.zip with the
# Linux binary, host.json, the function folders and config/sites.json.
set -euo pipefail
root="$(cd "$(dirname "$0")/../.." && pwd)"
out="$root/cms/deploy/out"
rm -rf "$out" && mkdir -p "$out/pkg"
bun "$root/cms/admin/build.ts"
(cd "$root/cms" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o "$out/pkg/cms" ./cmd/cms)
cp -r "$root/cms/deploy/functions/." "$out/pkg/"
cp "$root/config/sites.json" "$out/pkg/sites.json"
(cd "$out/pkg" && zip -qr "$out/cms.zip" .)
echo "$out/cms.zip"
