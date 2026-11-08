# Access

Who can use the CMS and for which Organizations: Google sign-in, the Super Admin, Moderators and their Invitations, sessions, and the Audit Log that records what they did. It uses the CMS, Storage and Google.

## How it works

### Sign-in

Google OIDC is the only identity provider in v1; Apple can follow as one more `IdentityProvider` adapter (it costs $99 a year and needs its client secret rotated at least every six months). The CMS runs the authorization code flow with PKCE, nonce and state, and verifies the ID token by hand: RS256 against the cached JWKS, then `iss`, `aud` (and `azp`), `exp`, `iat`, nonce and `email_verified`. PKCE verifier, nonce and state live in a short-lived signed cookie. A user is identified by issuer and subject, never by email. Locally, a dev OIDC stub replaces Google; it is compiled only with the `dev` build tag, and CI checks that the production binary does not contain it.

```mermaid
flowchart LR
  B[Browser] -->|/auth/login| C[CMS]
  C -->|redirect with PKCE, nonce, state| G[Google]
  G -->|/auth/callback with code| C
  C -->|verify ID token, match user or Invitation| S[Session cookie]
```

### Roles and Invitations

- The **Super Admin** is configured by the app setting `SUPER_ADMIN_EMAIL`, set at deploy from a GitHub secret (secrets are masked in logs, variables are not). It works as a permanent Invitation, bound to the Google `sub` on first sign-in, and can act as a Moderator of every Organization.
- A **Moderator** is invited by email for chosen Organizations. The Invitation is bound to the Google `sub` on first sign-in, because an email can change and `sub` cannot. It expires after 14 days if unused and can be revoked while pending. The CMS sends no email: the Super Admin passes the link on.
- The Super Admin can change a Moderator's Organizations or revoke all access. Removing an Organization from the person who created its Social Account Connection flags that connection for reconnection.
- Every use case checks the user's access to the Organization it touches.

### Sessions and request security

- The session cookie (`__Host-folkms`) is HMAC-signed, `HttpOnly; Secure; SameSite=Lax; Path=/`, valid for 8 hours. The user document is re-read on every request, so a revocation takes effect immediately; revocation also invalidates sessions issued before it.
- `SameSite=Lax`, not `Strict`, because a `Strict` cookie is not sent on the navigation back from Google. This is safe only because the CMS is same-origin; see [Platform](../platform/README.md#hosting).
- Unsafe methods are protected against CSRF: `Sec-Fetch-Site` must be `same-origin` when present, otherwise `Origin` must be the CMS origin. JSON bodies must be `application/json`.
- Sign-in endpoints are rate limited; responses carry a CSP and security headers.

### Admin UI language

The admin SPA and the CMS's own pages (sign-in errors, privacy, data deletion) are in Polish or English. The language comes from the user's browser settings: the first of Polish or English among the browser's preferred languages (`navigator.languages` in the SPA, `Accept-Language` on the server), else English. The switch in the header overrides it for a year in the `folkms-lang` cookie, which the SPA sets and the server reads, and reloads the page in that language.

The API does not depend on the language: every message it returns (errors, validation problems, the reconnection note) carries both Polish and English, and the SPA shows the one it uses. This is independent of Event content, which is authored in Polish with optional English for Sites.

### Audit Log

An append-only record of Event changes, Publish and Unpublish, Social Posts and access grants, in one append blob per day. Only the Super Admin can read it. It outlives Events purged from the Bin.

## Where it lives

| What | Path |
| --- | --- |
| Roles and Invitation lifecycle | [`cms/internal/domain/access.go`](../../cms/internal/domain/access.go), [`cms/internal/app/auth.go`](../../cms/internal/app/auth.go), [`cms/internal/app/access.go`](../../cms/internal/app/access.go) |
| Google and dev OIDC adapters | [`cms/internal/adapters/google/`](../../cms/internal/adapters/google/), [`cms/internal/adapters/devoidc/`](../../cms/internal/adapters/devoidc/) |
| Session cookie, CSRF, headers, rate limit | [`cms/internal/http/`](../../cms/internal/http/) |
| Admin UI language: choice, switch and server pages | [`cms/admin/src/i18n.ts`](../../cms/admin/src/i18n.ts), [`cms/internal/http/lang.go`](../../cms/internal/http/lang.go) |
| Moderators, Invitations and Audit Log screens | [`cms/admin/src/views/access.ts`](../../cms/admin/src/views/access.ts), [`cms/admin/src/views/audit.ts`](../../cms/admin/src/views/audit.ts) |

## Change together

| Rule | Files | What breaks |
| --- | --- | --- |
| The SPA and the server use the same language cookie | `cms/admin/src/i18n.ts`, `cms/internal/http/lang.go` | Server pages ignore the language chosen in the SPA. Enforced by `TestLangCookieMatchesAdmin` in `cms/internal/http`. |

## Read next

- [Platform](../platform/README.md): hosting and secrets
- [Root README](../../README.md#operations): Google OAuth client and Super Admin bootstrap
- [CONTEXT.md](../../CONTEXT.md#access): the terms this capability uses
