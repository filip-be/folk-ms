# 0010. Autosync keeps a Facebook page token in Key Vault

- Status: accepted
- Date: 2026-11-13; threats revised 2026-11-14 after a red-team review (T1, T4, T5, T8, T11, T12, T13 to T16)
- Amends [ADR 0005](0005-meta-sign-in-for-every-post.md), which still holds for Organizations with Autosync off

## Context

With Facebook Events as the source of Events ([ADR 0009](0009-facebook-events-are-the-source-of-events.md)), the Site is only as fresh as the last Sync. Under [ADR 0005](0005-meta-sign-in-for-every-post.md) the CMS stores no Meta token, so every Sync needs a Moderator to sign in with Meta. Some Organizations want their Site to follow Facebook unattended; that needs a token at rest, which ADR 0005 rejected for good reasons.

## Decision

**Autosync** is an opt-in per Organization, off by default. A Moderator of the Organization or the Super Admin turns it on after a warning saying what is stored, what it allows, and that it needs a new Meta sign-in at least every 90 days.

- **While Autosync is off** nothing changes from ADR 0005: "Sync now" and each Social Post go through a Meta sign-in, and nothing from Meta is at rest.
- **Turning it on**: a Meta sign-in by someone with a role on the Facebook page; the code becomes a user token, which is exchanged for a long-lived one, from which the Facebook page token is derived. The page token is stored in the secret store under a new name; the user tokens are dropped. The Social Account Connection records who turned Autosync on, when, the secret's name and `data_access_expires_at` (90 days after the sign-in); never the token.
- **While it is on**: an hourly timer syncs the Organization with the stored token, and a Social Post is one request with the stored token.
- **Turning it off**, changing the Facebook page, or the person who turned it on losing CMS access to the Organization deletes the secret at once.
- **Secret store**: Azure Key Vault behind a `SecretStore` port (a `0600` file store locally and on a VPS).

### Zero-trust audit

| # | Threat | Answer |
| --- | --- | --- |
| T1 | Token stolen from storage | Never in the storage account. Page tokens are in their own Key Vault, one secret per Organization; RBAC vaults; only the CMS's managed identity has a data role. No human has a standing data role. |
| T2 | Stolen token used elsewhere | "Require App Secret" is on for the Meta app: Graph refuses a call without `appsecret_proof` (confirmed by the spike). The app secret is in a second vault the CMS identity can only read; it cannot change or delete it. A token copied together with the app secret (code execution, T3) works until the app secret is rotated. |
| T3 | Code execution in the CMS | Not preventable once inside; reduced by stdlib-only Go ([ADR 0003](0003-stdlib-only-go.md)), fixed outbound hosts, the token read per use and never cached, logged or returned, and detection (T8). |
| T4 | Deploy pipeline compromise | The deploy identity loses `User Access Administrator` for a conditioned `Role Based Access Control Administrator` that may assign only the three roles the CMS identity needs, to that identity only. The CMS runs as a user-assigned managed identity created in the first-deployment runbook. Builds and tests run in jobs with no environment, secrets or OIDC token; only the deploy jobs, which run Azure CLI on the built package, use the protected `production` environment (`main` only, a required reviewer). Residual: whoever controls deploys can ship code that uses the token. |
| T5 | Hijacked or forged session | The API never returns the token. A session counts only if its nonce matches a sign-in recorded on the user document, so a cookie forged with `SESSION_KEY` is refused outright; signing out deletes the record. Posting, Autosync on/off, changing or removing the connection, and the Super Admin's access changes (Invitations, Organizations, revocation) need a sign-in within 15 minutes, so a copied cookie cannot invite an accomplice. `SESSION_KEY` is a Key Vault reference. Daily post limit per Organization (10). Every use is in the Audit Log. |
| T6 | Wrong page connected | Meta issues the token only to someone with a role on the page; the screen names page and id; connect, replace and disconnect are audited. |
| T7 | Hostile content from Facebook | Facebook text is untrusted: length limits, plain text, `https` URLs only, escaped when rendered. Covers are fetched only over `https` from Facebook's CDN hosts, size-capped and re-encoded. Hide and disconnect are kill switches. |
| T8 | Misuse unnoticed | Every use of the stored token writes an Audit Log line before the token is read, and the token is not used if that fails; a full daily log blob rolls over to a new one. The lease and the daily post limit are checked before the token is read, so callers cannot raise the read count at will. Key Vault audit logs and blob writes go to Log Analytics; alerts email the Super Admin when either vault is used in any way by another identity, the `cms` container is written by another identity, page tokens are read far more often than hourly, or role assignments, diagnostic settings, alert rules or vaults change. About $1 a month more. |
| T9 | A Manual Event's Facebook link used for SSRF or phishing | Never fetched; accepted only as a Facebook event address, stored as `https://www.facebook.com/events/{id}/`. |
| T10 | Public `/sync` endpoint | No input; a per-Organization sync lease document claimed with an ETag, so repeated calls cannot multiply Graph calls across instances. |
| T11 | Disconnect leaves access | The secret is deleted and purged at once; a daily sweep deletes token secrets no connection names (a crash, a race) and purges any left in soft delete. Connection writes are conditional, so a concurrent change cannot drop the name of a stored token. The CMS reads a secret only under a name of its own Organization's form. A page token cannot revoke itself: the screen and runbook tell the person who turned Autosync on to remove the app under Facebook, Settings, Business integrations. A leftover token is useless without the app secret (T2). |
| T12 | Token scope too wide | Only `pages_show_list`, `pages_read_engagement`, `pages_manage_posts`, `instagram_basic`, `instagram_content_publish` are requested. Before a token is stored, any other permission the person granted the app earlier (such as `business_management`) is revoked, and `debug_token` must show a valid PAGE token of this app for the connected page with no other scope. The token is not used after Meta's data access expires. Residual: it can still post, because posting with Autosync on uses it; a read-only second Meta app for Sync would remove that and is an open option. |
| T13 | A crafted id reaches another Organization's documents | Ids in API paths must be plain names; use cases check an id's shape and that a loaded Event names the Organization and id asked for; every store adapter, Blob Storage included, refuses dot segments and other characters. |
| T14 | Facebook returns an email its user does not own | A Facebook identity seen for the first time is linked to an active user only from a session of that user. Accepted (ADR 0008): an Invitation, to a new or revoked user, can be redeemed by a Facebook identity returning its email. Sign-in started from another site needs a click on a CMS page. |
| T15 | Flooding and cost | Changing API requests are limited per user, sign-in and timer endpoints per address; one image upload at a time per instance, its slot taken before the body is read; a monthly budget emails the Super Admin. Flex Consumption cannot scale below 40 instances. |
| T16 | Token in a URL, log or stored error | Graph calls carry their token in the `Authorization` header, not the URL; only the two exchanges that take a token as a parameter (`fb_exchange_token`, and `input_token` of `debug_token`, once per Autosync on) send it in the query, to Graph over TLS; Graph errors are stripped of URLs and scrubbed of tokens and the app secret; the public timer endpoints answer without error details. |

**Residual risk:** whoever can run code as the CMS, or deploy it, can post to the Facebook page and Instagram account of every Organization with Autosync on. Organizations that leave Autosync off do not carry this risk.

## Alternatives

- **No stored token, ever** (ADR 0005 alone): every Sync needs a person; Sites go stale between sign-ins.
- **Token in the storage account, encrypted with an app-setting key**: the design ADR 0005 removed; the key sits next to the data.
- **A separate sync service with its own identity**: a second deployable for a small association, without removing T3 or T4.

## Consequences

- Organizations choose between freshness (Autosync, a token at rest) and no secret at rest (manual Sync).
- Autosync is not fully unattended: Meta ends data access 90 days after the sign-in that issued the token; from 14 days before, the CMS shows a "reconnect before" banner.
- A Key Vault, a Log Analytics workspace and an alert rule join the platform: about $2 a month in total instead of under $1.
- Posting with Autosync on needs a recent CMS sign-in instead of a Meta sign-in.
