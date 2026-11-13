# fbspike: can the CMS read a Facebook page's events?

A throwaway check (step 1 of the plan "Facebook Events as the source of Events"). It is not part of the CMS and is deleted once its answer is on record. If the events edge is not readable, nothing else of that plan is built (decision 15).

## Prepare a test Facebook page

On a Facebook page you have a role on (not the real Organization page), create:

1. Two or more **upcoming** events hosted by the page, one with a cover image and a ticket link.
2. One **repeating** event (several dates).
3. One event **co-hosted** with another page, created by that other page, if you can arrange it. Note its id.
4. For the "gone" signal: one event you then **delete**, one **private** event, and one left as a **draft**. Note their ids first (the number in `facebook.com/events/<id>`).

In the Meta app (*App settings → Advanced*) switch **Require App Secret** on. You can run once with it off and once with it on to see the difference.

## Run

From `cms/`, with the Meta app's id and secret:

```sh
export META_APP_ID=… META_APP_SECRET=…
go run ./cmd/fbspike -page <page-id> -login \
  -cohosted <id> -probe <deleted-id>,<private-id>,<draft-id> > fbspike-report.md
```

`-login` prints a Facebook sign-in link and waits on `http://localhost:8765/callback`; add that address to the app's *Valid OAuth Redirect URIs* if Facebook refuses it. Sign in as a Tester who has a role on the test page. Instead of `-login` you can set `FBSPIKE_USER_TOKEN` to a user token from the Graph API Explorer for this app.

If the events edge is refused for lack of a permission, run again with `-scopes <permission>` (for example `-scopes pages_read_user_content`) to find which one it needs. `-version` picks the Graph version (default `v24.0`).

## What comes back

A Markdown report on stdout, by question: tokens (long-lived page token expiry, `data_access_expires_at`, scopes, whether a call without `appsecret_proof` is refused), the events edge with four variants, which fields are filled, repeating events and their child ids, co-hosted events, what Graph answers for the deleted, private and draft ids, the rate-limit headers, and a **GO** / **NO-GO** verdict.

Tokens and the app secret are redacted from the report and nothing is written to disk. Event names and descriptions of the test page do appear in it; read it before sharing it. Afterwards remove the app from your Facebook account (*Settings → Business integrations*) to end the page token.
