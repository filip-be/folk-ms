#!/usr/bin/env bash
# Local CMS: filesystem stores in .data/, dev sign-in (any email), Social
# Posts recorded instead of sent. Open http://localhost:8080/app/
set -euo pipefail
export PATH="/usr/bin:/bin:$PATH"
root="$(cd "$(dirname "$0")/.." && pwd)"
for tool in go bun; do
  command -v "$tool" >/dev/null || { echo "$tool is not on PATH; see README.md#local-development" >&2; exit 1; }
done
bun "$root/cms/admin/build.ts"
cd "$root/cms"
export FOLKMS_ENV=local DATA_DIR="$root/.data" SITES_CONFIG="$root/config/sites.json"
export SUPER_ADMIN_EMAIL="${SUPER_ADMIN_EMAIL:-admin@example.org}" PORT="${PORT:-8080}"
exec go run -tags dev ./cmd/cms
