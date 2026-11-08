#!/usr/bin/env bash
# Starts Azurite (pinned digest, same as .github/workflows/ci.yml) on
# 127.0.0.1:10000. In a container without a running Docker daemon (e.g. a
# Claude Code cloud session) it starts dockerd first.
# Then: (cd cms && AZURITE_BLOB_URL=http://127.0.0.1:10000/devstoreaccount1 go test ./internal/adapters/azblob/)
set -euo pipefail
image="mcr.microsoft.com/azure-storage/azurite@sha256:830430c1da1a2d537e08f3e6764dd1f5ae00cf0346bcaf625b968ec3f0971fd5"
if ! docker info >/dev/null 2>&1; then
  command -v dockerd >/dev/null && [ "$(id -u)" = 0 ] || { echo "Docker is not running" >&2; exit 1; }
  nohup dockerd >/tmp/dockerd.log 2>&1 &
  for _ in $(seq 1 30); do docker info >/dev/null 2>&1 && break; sleep 1; done
fi
if [ -z "$(docker ps -q -f name='^azurite$')" ]; then
  docker run -d --rm --name azurite -p 10000:10000 "$image" >/dev/null
fi
for _ in $(seq 1 30); do curl -s -o /dev/null http://127.0.0.1:10000/ && break; sleep 1; done
echo "Azurite on http://127.0.0.1:10000/devstoreaccount1" >&2
