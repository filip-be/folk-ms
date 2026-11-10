# 0004. Same-origin admin on a Flex Consumption custom handler

- Status: accepted, pending the cloud spike
- Date: 2026-11-07

## Context

The admin SPA, the JSON API and sign-in should share one origin, so cookies are first-party (`SameSite=Lax`) and no CORS is needed. The CMS must cost nothing at rest.

## Decision

One Go binary runs as an Azure Functions custom handler on Flex Consumption. `host.json` sets `routePrefix: ""` and request forwarding; one HTTP function with the catch-all route `{*path}` forwards every request to the binary, which serves `/app/*` (embedded SPA), `/api/*`, `/auth/*`, `/privacy`, `/data-deletion` and `/healthz`. A timer function `purge` in the same app runs the daily Bin purge; the host invokes it as `POST /purge`. The same binary also runs as a plain server (`PORT`) on a VPS or in a container.

## Spike results

Verified locally on 2026-11-07 with the official host image `mcr.microsoft.com/azure-functions/base:4` (host 4.636.2.2) and Azurite:

| Check | Result |
| --- | --- |
| `enableProxyingHttpRequest: true` alone | **Not honoured** by this host: requests reached the binary as invocation payloads (`POST /http`), so every page failed. |
| `enableForwardingHttpRequest: true` | Works: `/`, `/healthz`, `/app/...`, `/api/...`, `/auth/...` and `/privacy` reach the binary with method, path, body and headers intact. `host.json` sets both flags. |
| Host-reserved paths | `/admin/*` and `/runtime/*` answer 401 from the host and never reach the binary. The CMS uses neither. |
| `Set-Cookie` passthrough | Works (the `folkms-oidc` state cookie on `/auth/login`). |
| Timer next to forwarded HTTP | Works: `purge` fired on schedule and succeeded while HTTP was served. |
| Request size | 40 MB passes the host (the CMS caps uploads at 16 MB); 120 MB is refused by the host. |

Still to verify on a real Flex Consumption app, which cannot be done locally: the `SameSite=Lax` session cookie after the Google redirect on the production host name, several `Set-Cookie` headers in one response, cold start time, the client IP in `X-Forwarded-For` (the CMS uses the last hop), the managed-identity token endpoint for blob access, and Meta Standard Access for `pages_manage_posts` with a test Facebook page.

## Exit criterion

If the catch-all route or cookie passthrough fails on Flex, move the CMS to Azure Container Apps (consumption plan, scale to zero) running the same binary from an image on GHCR: replace the Flex resources in `infra/azure/main.bicep` and keep everything else.

## Consequences

- `/purge` is reachable from the internet as well as from the timer, because the host cannot attach a secret to timer invocations. Purging only removes Events that are overdue, and a purge runs at most once every 10 minutes per instance (one minute after a failure; extra calls are no-ops), so a call is harmless. A public call shortly before the timer can make the timer skip; an Event that became overdue in between is then purged up to a day late.
- Scale-to-zero costs a 1–2 s first request, acceptable for an admin UI.
