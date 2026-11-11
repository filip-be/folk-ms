# 0008. Sign in with Facebook; users matched by email

- Status: accepted
- Date: 2026-11-11
- Changes the "bound to the Google `sub`, never matched by email" rule of [Access](../../access/README.md)

## Context

Google was the only identity provider. Moderators already have Facebook accounts, because they post Social Posts with a Meta sign-in ([ADR 0005](0005-meta-sign-in-for-every-post.md)), and some would rather sign in to the CMS with Facebook too.

A user was identified by issuer and subject only. A second provider gives the same person a second identity, and the CMS had to decide how that identity reaches the person's user.

Facebook's web login is plain OAuth, not OIDC: there is no ID token, and the identity comes from Graph `/me` (`id`, `name`, `email`). Meta returns `email` only when the account has a valid primary email, but the field has no "verified" flag, and Meta does not promise that the person confirmed it.

The Meta app is a Business app in Standard Access, so only people with a role on the app (Testers) can authorize it.

## Decision

- **Facebook is a second identity provider** on the same Meta app as posting. It uses the authorization code flow with PKCE and state, then calls Graph `/me` with `appsecret_proof`. The user token is dropped at once. The scopes are `email,public_profile`.
- **Users are matched by email across providers.** A sign-in first looks for the user of its own identity. If there is none, it looks for a user with the same email, and it does this again on every sign-in, with no stored alias, so a changed email stops matching at once. A matched identity is recorded on the user, and the first match is written to the Audit Log (`identity.matched`). An Invitation is still bound to whichever identity redeems it first.
- **An email from Facebook counts as confirmed for Moderators.** We accept Meta's "valid primary email" as proof that the person owns the address. A Facebook identity with no email is refused.
- **The Super Admin stays on its one bound identity.** Facebook identities are marked `NoSuperAdmin`, and three rules follow:
  - A Facebook identity with `SUPER_ADMIN_EMAIL` is refused, and so is any identity whose user is the Super Admin.
  - The Super Admin's email never goes through matching.
  - Matching never resolves to the bound Super Admin, also when its email changed after binding.
- **Testers only.** The app stays in Standard Access: only Testers of the Meta app can sign in with Facebook, and everyone else uses Google.

## Alternatives

- **One provider per person:** each user keeps the provider they first used. No email trust is needed, but someone who already uses Google cannot switch.
- **Account linking:** a signed-in user adds Facebook from inside the CMS. Nothing rests on Meta's email, but it needs more model and UI work. Not chosen: the owner preferred matching.
- **Facebook's OIDC ID token:** it would need JWKS verification against another issuer. It gives nothing over a server-side Graph call signed with the app secret, and still has no email-verified claim.
- **Advanced Access, or a separate consumer Meta app, so that anyone can use Facebook sign-in:** this means business verification or app review, or a second app and secret. Not needed while the Moderators who use Facebook are Testers anyway.

## Consequences

- A Moderator can sign in with Google or Facebook and is the same user, with the same Organizations, as long as both accounts return the same email.
- Anyone who controls a Facebook account whose email equals a Moderator's email can sign in as that Moderator. Meta's email check is trusted for Moderators, never for the Super Admin.
- The Meta app's valid OAuth redirect URIs must include `<CMS_BASE_URL>/auth/callback`.
- `/auth/login` takes `provider=google|facebook`. A non-secret cookie remembers the last provider that worked, so an expired session goes back to it.
