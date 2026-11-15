# 0011. Facebook sign-in behind a Feature Flag

- Status: accepted
- Date: 2026-11-15
- Amends [ADR 0008](0008-facebook-sign-in-matched-by-email.md), which still describes how Facebook sign-in works when it is on

## Context

Facebook sign-in to the CMS ([ADR 0008](0008-facebook-sign-in-matched-by-email.md)) needs a Meta app that lets people other than its developers sign in with Facebook Login, which means a Meta business app. The CMS does not have one yet, so the button on the sign-in screen leads to an error at Meta. The code works and is tested, and the Meta app will come; removing the feature and adding it back later would be churn.

Nothing in the CMS could turn a feature off at run time: configuration came from app settings and `config/sites.json`, which need a redeploy.

## Decision

- **Feature Flags.** A small, generic set of switches the Super Admin turns on or off at run time. The known flags are a fixed list in the domain (`cms/internal/domain/feature.go`), each with a name, a description and a default; any other name is refused (404). Adding a flag is one constant and one entry in that list.
- **Stored as one document.** The values set so far are one JSON document, `system/feature-flags.json`, in the private store, written with If-Match like every other document ([ADR 0002](0002-blob-json-documents-with-etag.md)). A missing document, or a flag never set, means its default. A value of a flag no longer in the list is ignored.
- **Super Admin only, as an access change.** `GET /api/feature-flags` lists them and `PUT /api/feature-flags/{flag}` with `{"enabled": true|false}` sets one. A flag can change how people sign in, so changing one needs the Super Admin with a Recent sign-in, like inviting or revoking. Each change is written to the Audit Log (`feature_flag.changed`). The screen is *Administration → Feature flags*.
- **Fail closed.** When the document cannot be read, every flag counts as off.
- **The first flag, `facebookSignIn`, is off by default.** While it is off, `/auth/login?provider=facebook` and a Facebook `/auth/callback` are refused, a remembered last provider of Facebook falls back to Google, and the SPA neither offers Facebook on the sign-in screen nor shows *Link Facebook*. The sign-in screen is shown before sign-in, so it asks the public `GET /auth/methods`, which answers only the providers offered (`{"methods": ["google"]}`); `/api/me` carries the same list as `signInMethods`. Google sign-in and the dev OIDC login are not affected. The Meta sign-in for the Social Account Connection, Sync and Social Posts (`/auth/meta/callback`) is not gated.

## Alternatives

- **Remove Facebook sign-in until the Meta app exists.** Clean, but the code, tests and docs would have to come back later, and might rot meanwhile.
- **An app setting (environment variable).** No new document or screen, but changing it needs a redeploy by someone with access to the infrastructure, and it is not in the Audit Log.
- **A flag per Organization.** Sign-in happens before the CMS knows the Organization, so a global switch is what this needs. Per-Organization flags can be added later if a feature needs them.

## Consequences

- Facebook sign-in is unavailable until the Super Admin turns it on, after the Meta app allows it. Moderators who used Facebook sign in with a Google account that returns the same email meanwhile; a linked Facebook identity stays on their user and works again when the flag is on.
- Every load of the sign-in screen, `/api/me` and each Facebook sign-in read one small document.
- `/auth/methods` is public, under the sign-in rate limit, and says only which providers are offered.
