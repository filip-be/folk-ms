# 0005. No stored Meta tokens: sign in with Meta for every post

- Status: accepted
- Date: 2026-11-08

## Context

To post on a Facebook page and its Instagram account, the CMS needs a Facebook page token. The first implementation kept one per Organization: a Moderator connected the Facebook page once, and the CMS stored the Facebook page token, which never expires, encrypted with AES-256-GCM under a key in the app settings.

That token was the CMS's most valuable secret, and storing it had weak points:

- The encryption key sat in the app settings of the same app that reads the blobs. Encryption protected against a storage-only leak, not against anyone who could run code in the app or read its settings or the GitHub secrets.
- "Disconnect" deleted the current document only. Blob versions and soft delete kept the encrypted token for up to 90 days, and Meta kept it valid.
- After the Facebook sign-in, the tokens of every Facebook page the Moderator administers were stored, encrypted, until they chose one, and longer if they never did.
- When the Moderator who connected lost access, posting still went out under their token until someone reconnected.

## Decision

The CMS stores no Meta token. A Moderator signs in with Meta every time they post:

- Posting creates a Post Request (Event, platforms, Captions; no secret) and sends the Moderator to the Meta sign-in. The `state` is in a signed cookie bound to the CMS user, the Organization and the Post Request.
- The callback redeems the code for a short-lived user token, asks Graph for the Facebook page token of the Organization's Facebook page, posts every selected platform, and drops both tokens. The user token is never exchanged for a long-lived one.
- Graph returns the Facebook page token only when the Moderator has a role on the Facebook page, so each post is authorized by Meta at that moment. Instagram is posted only when the Facebook page, as this Moderator sees it, links to the connected Instagram account.
- The Social Account Connection keeps only the Facebook page and Instagram ids and names. Listing Facebook pages to choose from does not request their tokens.
- One sign-in covers Facebook and Instagram. The CMS uses Facebook Login and reaches Instagram through the Facebook page, not a separate Instagram Login.

## Alternatives

- **Keep the stored token and harden it:** a Key Vault key to wrap the token, a lifecycle rule to drop old versions, revoking the token at Meta on disconnect. Smaller change and no extra sign-in, but the CMS would still hold a token that never expires, and a compromised app could still use it.
- **Post from the browser with Meta's JavaScript SDK:** no token on the server at all, but the token lives in the page's JavaScript, `appsecret_proof` (which needs the app secret) is impossible, and the Audit Log would record what the browser claims.

## Consequences

- Nothing secret from Meta is at rest; there is no token-encryption key to manage, rotate or leak.
- Posting needs a Meta sign-in each time. When the Moderator is already signed in to Facebook and approved the app before, it is usually a redirect or one click.
- Every Moderator who posts must be a Tester on the Meta app and have a role on the Facebook page that allows posting, with access to its Instagram account. Before, only the Moderator who connected needed this.
- Removing someone's Facebook page role, Tester role or CMS access stops their posting at once. There is no "reconnect" state.
- The Meta callback runs the posts, Instagram's container polling included, so it can take tens of seconds.
