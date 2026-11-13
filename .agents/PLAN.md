# Plan: Facebook Events as the source of Events

Status: approved. Step 1 (spike) done on 2026-11-13, verdict GO. Decisions 1, 10, 11, 14, 17 and 20, T9, T12 and steps 3, 5, 6, 7, 9 and 11 were revised from its results; the user confirmed them, Manual Event dates entered one by one. Nothing after step 1 is implemented.

## Goal

Today the CMS is the source of truth for Events, and Facebook Events are created by hand from a Facebook Event Handoff, optionally typed by Autofill. That is tedious for Moderators. The new design turns it around: a Moderator creates the event on Facebook, the CMS pulls (syncs) the Organization's Facebook Events and the Site shows them. The CMS adds what Facebook lacks (English text, Price, Summary, Social Image) and keeps Social Posts.

## Decisions (settled with the user)

| # | Decision | Choice |
| --- | --- | --- |
| 1 | Source of Events | Facebook, through Sync. CMS authoring is removed, except Manual Events (decision 20). |
| 2 | Sync trigger | "Sync now" is always available. Autosync (hourly) is optional per Organization, off by default; a Moderator of the Organization or the Super Admin turns it on or off. |
| 3 | Token | Only while Autosync is on: one stored long-lived Facebook page token for that Organization, used for sync and for Social Posts. While Autosync is off the CMS stores nothing from Meta: "Sync now" and each Social Post go through a Meta sign-in (ADR 0005 as today). Turning Autosync on stores the token, after a warning; turning it off deletes it. A zero-trust security audit is part of this plan. |
| 4 | Token store | Azure Key Vault, behind a new port (file on a VPS or locally). |
| 5 | Editing | Facebook-sourced fields may be overridden in the CMS; an override wins. When Facebook later changes an overridden field, the override stays, the Event is flagged, and the Moderator can "Follow Facebook again" per field. Dates and cancellation are never overridable. |
| 6 | CMS-only fields | English name and description, Summary (PL/EN), Price text (PL/EN), Social Image. |
| 7 | Languages | Facebook text is the Polish content; English is added in the CMS; Translation fallback as today. |
| 8 | Visibility | A synced Event is on the Site immediately. Each Event has a Hide switch that sync never undoes. CMS edits go live on save. Draft, Publish, Unpublish disappear. |
| 9 | Repeating Facebook Events | One Event with one Permalink and many Occurrences, each linking to its own Facebook page. |
| 10 | Import scope | Events the connected Facebook page created, cancelled ones included, from those upcoming at first sync onward (no history backfill); they stay after they pass. **Revised after the spike:** an event another page created and the Organization's page co-hosts is neither listed nor readable by id (that needs Page Public Content Access, out of scope; Meta refuses `user_events` for this app). Such events are Manual Events (decision 20); "Add by link" is dropped. Two Organizations co-hosting one Facebook Event each get an independent Event. |
| 11 | Disappeared from Facebook | Cancelled: shown as cancelled. Missing from the list: read by id. **Revised after the spike:** Graph answers a deleted event with code 100, subcode 33, the same answer as for missing permission, so it counts as "gone" only when the same run listed the page's events successfully with the same token; then the Event moves to the Bin (30 days). Any other error, a network error or an empty answer changes nothing. |
| 12 | Who connects | Any Moderator of the Organization or the Super Admin; audited. |
| 13 | Posting guard | With a stored token (Autosync on): posting a Social Post, turning Autosync on or off, and changing or removing the connection need a CMS sign-in in the last 15 minutes. Without one (Autosync off) the Meta sign-in is the guard, as today. A per-Organization daily post limit applies either way. |
| 14 | Old code and data | Authoring of synced Events (create, Recurrence, Duplicate, Venues), the Handoff, `autofill/` and the practice page are deleted. Existing CMS Events are discarded. |
| 15 | If the Graph API cannot read page events | Stop after the spike and re-plan. Nothing else is built. |
| 16 | CMS texts when Facebook's Polish text changes | English name and description and the Summary are flagged as possibly out of date when the Facebook name or description they were written against changes; the Moderator updates them or dismisses the flag. The Site keeps showing them meanwhile. |
| 17 | Connection replaced with a different Facebook page | Everything is kept. Events of the old page stay on the Site frozen as they were and are no longer synced; the Moderator hides what is unwanted. The screen asks for confirmation naming both pages. Manual Events are unaffected. |
| 18 | Detecting token misuse | An Azure Monitor alert emails the Super Admin on unusual Key Vault reads. Costs about $0.50 to $1.50 a month, so the total goes from under $1 to roughly $2 a month; accepted. |
| 19 | The person who turned Autosync on loses CMS access to the Organization | Autosync turns off at once and the stored token is deleted; the Organization falls back to manual Sync and a Meta sign-in per post, with a banner asking another Moderator to turn Autosync on again. |
| 20 | Manual Events (new after the spike; confirmed 2026-11-13) | A Moderator enters by hand a co-hosted event another page created: Polish and English name and description, one or more Occurrences (dates entered one by one, no Recurrence rules), Location, Cover Image upload, ticket link, and the required link to its Facebook Event. CMS-only fields as decision 6. Sync never touches it; the Moderator keeps it up to date. Hide and the Bin work as for synced Events; there is no Draft. |

## Spike results (2026-11-13, `cms/cmd/fbspike`, Graph v24.0, test page "Folk MS")

- **Events edge: GO.** `GET /{page-id}/events` with the page token returns the page's events under Standard Access with `pages_read_engagement`. Cancelled events are left out unless `include_canceled=true` (5 events without, 6 with). The long-lived user token sees fewer (4); Sync uses the page token.
- **Fields.** Filled: `id`, `name`, `description`, `start_time`, `timezone`, `place` (often only `name`, free text), `is_canceled`, `updated_time`, `is_online`, `type`, `is_draft`, `category`; when set: `end_time`, `cover`, `ticket_uri`, `event_times`. `owner` is never returned, so the host is not visible.
- **Repeating events.** `event_times` lists each date with its own id, start and end; each child is readable by id.
- **Co-hosted events** created by another page: not in the list; reading one by id fails with code 100 and asks for `pages_read_engagement` or Page Public Content Access; `user_events` is refused at sign-in ("Invalid Scopes"). Hence decisions 10 and 20.
- **Deleted event** read by id: code 100, subcode 33 ("does not exist, cannot be loaded due to missing permissions"), with page and user token alike. Hence decision 11. Private and draft events were not probed.
- **Tokens.** The page token derived from a long-lived user token has `expires_at` 0 (never), but `data_access_expires_at` is 90 days after the sign-in. With "Require App Secret" on, a call without `appsecret_proof` is refused.
- **Page ids.** The number in a `profile.php?id=` link is not the page id Graph uses; the page id comes from `/me/accounts` at connect, as today.
- **Rate limits.** One full run made about 30 calls; `X-App-Usage` and `X-Business-Use-Case-Usage` stayed far from any limit.

## Assumptions made by the agent (not asked; say if wrong)

- Location is free text from Facebook (place name, address, coordinates as a map link). The reusable Venue list is gone.
- The Facebook cover is copied into `pub` as JPEG variants (Facebook CDN links expire), through the existing image processor.
- An Event stops being synced one day after its last Occurrence ends; it then stays as it was.
- A broken token (revoked, role lost) leaves Sites as they are; the CMS shows a "reconnect" banner and writes to the Audit Log. No email is sent (the CMS has no mail).
- The Bin keeps its meaning for Events gone from Facebook and for Manual Events a Moderator removes; restoring a synced one un-bins it until the next sync confirms it is still gone.
- **Data access expiry (confirmed by the spike).** The connection stores `data_access_expires_at`; from 14 days before it the CMS shows a "reconnect before" banner. Turning Autosync on again (a fresh Meta sign-in) replaces the token and the date. Autosync is therefore not fully unattended: someone signs in with Meta at least every 90 days, and the Autosync warning says so.
- Snapshot schema goes to version 2; CMS and both Sites deploy together (nothing real is in production).
- CONTEXT.md is updated as the first implementation step, not during planning, so the glossary never contradicts the code on `main`.

## Domain model changes (CONTEXT.md)

- **Event**: the CMS's record of one Facebook Event of the Organization, shown on its Site. Changed.
- **Facebook Event**: the event page on Facebook; the source of an Event. Created and edited only on Facebook. Changed.
- **Occurrence**: one date of an Event; a repeating Facebook Event yields several. Changed.
- New: **Sync** (pulling an Organization's Facebook Events into the CMS, hourly or on demand), **Override** (a CMS value replacing one Facebook-sourced field), **Hidden** (kept off the Site), **Manual Event** (an Event a Moderator enters by hand for a Facebook Event another page created, typically one the Organization co-hosts), **Location**.
- **Social Account Connection**: unchanged in meaning; while Autosync is on, the CMS additionally keeps a Facebook page token for it. Changed.
- **Social Post**: posted with a Meta sign-in, or, while Autosync is on, with the stored token after a recent CMS sign-in. Changed.
- New: **Autosync** (an Organization's opt-in to hourly Sync, which makes the CMS keep a Facebook page token for it).
- Removed: Recurring Event, Recurrence, Multi-day Event (a plain property), Duplicate, Venue, Draft, Publish, Unpublish, Facebook Event Handoff, Autofill. Post Request stays, and gains a sibling for a manual Sync.

## Security audit: the stored Facebook page token (zero trust)

**When there is a token.** Only for Organizations whose Autosync is on. An Organization that never opts in has nothing from Meta at rest, exactly as under ADR 0005; everything below applies to the others.

**What the token can do.** Publish, edit and delete posts on the Facebook page, publish to its Instagram account, read page content and events. It cannot change page roles, sign in as a person, or reach other pages. It does not expire, but its data access lapses 90 days after the sign-in that issued it (spike); it dies when the person who turned Autosync on changes their Facebook password, loses the page role, or removes the app.

**Threats and the design answer to each.**

| # | Threat | Mitigation built into the design |
| --- | --- | --- |
| T1 | Token stolen from storage (blob leak, backups, old versions) | Token is never in the storage account. One Key Vault secret per Organization; vault uses Azure RBAC; only the CMS's managed identity has a data role, scoped to that vault. No human has a standing data role. |
| T2 | Stolen token used from elsewhere | "Require App Secret" is switched on for the Meta app, so Graph rejects any call without `appsecret_proof`. A token alone is useless; the attacker also needs the app secret. The app secret moves from app settings into Key Vault as well (Key Vault reference), so neither is in deploy logs or app-setting exports. |
| T3 | Code execution in the CMS (dependency, RCE, SSRF to the identity endpoint) | Cannot be fully prevented once inside the process; reduced by: stdlib-only Go (ADR 0003), outbound calls only to fixed hosts, the token fetched per use and never cached, logged or returned by any API, and detection (T8). Recovery: reconnect issues a new token; the person who turned Autosync on removes the app on Facebook to kill the old one. |
| T4 | Deploy pipeline or repository compromise (malicious code deployed) | Existing controls: OIDC deploys, pinned actions, protected environment. Today the deploy identity holds `User Access Administrator` on the resource group, so it could grant itself a Key Vault role. It is replaced by `Role Based Access Control Administrator` with a condition: it may assign only the two roles the CMS identity needs (blob data, Key Vault secrets) and only to the CMS identity. For the condition to name that identity before the first deploy, the CMS switches from a system-assigned to a user-assigned managed identity, created once in the first-deployment runbook; it also survives the app being recreated. The first-deployment runbook changes accordingly. Residual: whoever controls deploys can still ship code that uses the token; accepted and documented. |
| T5 | Hijacked or forged Moderator session | The API never returns the token. Posting, connect and disconnect require a sign-in within 15 minutes; otherwise the SPA sends the Moderator to sign in again. Sessions are a stateless HMAC cookie today, so whoever holds `SESSION_KEY` could forge a fresh one: with a stored token that key becomes a posting credential. Therefore (a) `SESSION_KEY` moves to a Key Vault reference like the Meta app secret, and (b) each real Google or Facebook sign-in writes its time and a random session nonce to the user document; the cookie carries the nonce and guarded actions check both on the server. A forged cookie without the stored nonce posts nothing. Per-Organization daily post limit (10). CSRF protection as today. Every use is in the Audit Log with the acting Moderator. |
| T6 | Malicious or mistaken Moderator connects the wrong page | Meta only issues the token to someone with a role on the page; the connection screen shows page name and id; connect, replace and disconnect are audited; Moderators are per-Organization. |
| T7 | Untrusted content from Facebook reaches Sites and the admin (a compromised page, or a hostile co-hosted event) | All Facebook text is treated as untrusted input: length limits, plain text only, URLs must be `https`, rendered escaped (Sites already escape; verified in `sites/shared/render.ts`, tests added). Cover images are downloaded only over `https` from Facebook CDN hosts returned by Graph, with a size cap, and re-encoded by the image processor before being stored. The Hide switch and disconnect are the kill switches. |
| T8 | Misuse goes unnoticed | Audit Log entries for every token use (sync run summary, post, connect, disconnect). Key Vault diagnostic logs go to a Log Analytics workspace (pay-as-you-go tier, inside its free monthly allowance). A scheduled-query alert rule with an action group emails the Super Admin when a secret is read by any identity other than the CMS's, or far more often than the hourly pattern (decision 18). A sync status panel shows last run, last success and errors. |
| T9 | A Manual Event's Facebook link used for SSRF or phishing | The CMS never fetches it. It is accepted only as a Facebook event address and stored normalised to `https://www.facebook.com/events/{numeric id}/`; anything else is refused. Manual Event text is Moderator input, escaped like all text. Adding and editing are audited and limited to Moderators of the Organization. |
| T10 | Public timer endpoint (`/sync`, like `/purge` today, is reachable by anyone through the catch-all route) | It accepts no input, and a minimum interval between runs is enforced. Purge keeps that interval in memory, which is per instance and the app can scale out, so Sync keeps it in storage: a per-Organization sync lease document claimed with an ETag (see step 6). Calling `/sync` repeatedly cannot multiply Graph calls. |
| T11 | Disconnect does not really remove access | Disconnect deletes the secret (Key Vault soft delete, retention at the 7-day minimum; each time Autosync is turned on the token is stored under a new secret name recorded in the connection document, so a soft-deleted name is never reused and the CMS identity needs no purge right). A page token cannot revoke itself, so the screen names the person who turned Autosync on, whose Meta sign-in issued the token, and tells them to remove the app under Facebook, Settings, Business integrations; the runbook says the same. Because of T2 a leftover token is useless without the app secret. |
| T12 | Token scope too wide | Only the permissions needed are requested: `pages_show_list`, `pages_read_engagement` (enough for the events edge, spike), `pages_manage_posts`, `instagram_basic`, `instagram_content_publish`. `business_management`, requested today, is dropped. The long-lived user token used to derive the page token is dropped in the same request. |

**Residual risk, stated plainly:** whoever can run code as the CMS, or deploy it, can post to the Organizations' Facebook and Instagram. That is the price of unattended sync, paid only by Organizations that turn Autosync on; the switch's warning says so. The new ADR records this.

## Steps

### 1. Spike (go / no-go): done

Result GO; see "Spike results". `cms/cmd/fbspike` is deleted in step 3.

### 2. Decisions on record

- ADR 0009 "Facebook Events are the source of Events" (supersedes 0006), with the spike results and the Manual Event fallback.
- ADR 0010 "Autosync keeps a Facebook page token in Key Vault" with the audit above (amends 0005, which still holds while Autosync is off).
- Update CONTEXT.md as listed.

### 3. Remove the old design

Delete `autofill/`, `cms/cmd/cms/practice/`, `cms/cmd/fbspike/`, the Handoff (`domain/facebookevent.go`, `app/facebookevents.go`, `views/facebookEvent.ts`, `admin/src/autofill.ts`, userscript serving), Recurrence and schedule rules, Venues, Duplicate, Draft/Publish/Unpublish; their tests, CI steps, build hooks and README runbooks. The Event editor shrinks to what Manual Events need (decision 20) and the Override and CMS-only controls of synced Events (step 9). A one-off note in the runbook: empty `data/events` and `data/venues`. Post Requests and the Meta sign-in per post stay: they serve Organizations with Autosync off.

### 4. Secret store port and Key Vault

- Port `SecretStore` (Get, Set, Delete by name). Adapters: Key Vault REST with the managed identity (stdlib, like `azblob`), a `0600` file store for local and VPS, memory for tests; one shared contract suite.
- Bicep: Key Vault (RBAC, soft delete 7 days), role assignment for the CMS identity only, diagnostic settings to a Log Analytics workspace (`PerGB2018`), the alert rule and action group of T8 (the Super Admin's email as a deploy parameter), the Meta app secret and `SESSION_KEY` as Key Vault references.
- User-assigned managed identity for the CMS (T4): created in the first-deployment runbook and passed to Bicep; used for the Functions host storage, the deployment storage, Key Vault references and the Go adapters for blob and Key Vault, which pass its client id when asking for tokens.
- First-deployment runbook: create that identity, then give the deploy identity its conditioned role per T4.
- Cost: roughly $2 a month with the alert (decision 18); the Platform cost table and the stated target are updated.

### 5. Connection and the Autosync switch

Connect works as today and stores no token. Turning Autosync on (Moderator of the Organization or Super Admin, recent CMS sign-in, a warning naming what is stored, what it allows, and that it needs a new Meta sign-in at least every 90 days): Meta sign-in by someone with a role on the page, exchange for a long-lived token, derive the page token, store it in the secret store, record `data_access_expires_at`, drop the user token. Turning it off deletes the secret (T11) and returns the Organization to Meta sign-in for every Sync and post. The connection document keeps ids, names, the Autosync state, who turned it on and when, and the data access expiry; never the token. Changing the page while Autosync is on turns it off first. So does revoking the access of the person who turned it on, or removing them from the Organization (decision 19): user revocation and Organization membership changes delete the secret and write to the Audit Log. Replace and disconnect as in T11. A token Graph rejects marks the connection "needs reconnect".

### 6. Event model and Sync

- Event document: the Facebook part (id, fields, Occurrences with their Facebook ids, cover, updated time; written only by Sync), Overrides (name, description, location, cover, ticket link; each remembers the Facebook value it replaced, which is how "changed on Facebook since" is detected), CMS fields (decision 6), Hidden, Bin state. A Manual Event (decision 20) has the Moderator's values and its Facebook link instead of the Facebook part and Overrides. The stable id and Permalink are created at first import or at creation.
- Sync per connected Organization: list the page's events with `include_canceled=true` (a repeating event becomes one Event whose Occurrences come from `event_times`); read by id every followed Event of the current page missing from the list; upsert; apply decision 11; import changed covers; rebuild the snapshot once if anything changed; write a sync status document. Sync writes only the Facebook part, with ETag retry, so it never loses a Moderator's edit. Sync never reads or writes Manual Events.
- Concurrency: the timer, the public `/sync` and "Sync now" can run at once on different instances. Each run first claims the Organization's sync lease (a status document written with its ETag, holding an expiry time); a run that cannot claim it does nothing. "Sync now at most once a minute" reads the same document. Event identity is a create-only index document keyed by the Facebook Event id that points to the Event; an import that loses the create race uses the existing Event, so one Facebook Event can never become two Events or two Permalinks. A Manual Event's Facebook id goes in the same index, so a Facebook Event is never both synced and manual: when Sync meets an id a Manual Event holds, it leaves it alone and the Event list flags it.
- Each Event remembers which Facebook page it was synced from. Sync follows only Events of the currently connected page (decision 17).
- CMS texts remember the Facebook name and description they were written against, which is how decision 16's flag is detected (the same mechanism as overrides).
- Triggers: a new timer function `sync` (hourly) calling `POST /sync` guarded as in T10, which syncs only Organizations with Autosync on. "Sync now" (at most once a minute per Organization): with Autosync on, `POST /api/orgs/{org}/sync` uses the stored token; with it off, the same button starts a Meta sign-in whose callback runs the Sync with the token in memory and drops it, the way a Post Request works today.
- With Autosync off, the list shows when the last Sync ran, since nothing updates unattended.
- Locally a recorder with canned Facebook Events replaces Graph, as the Meta recorder does today.

### 7. Snapshot contract v2 and Sites

- Schema 2: no `recurrence` and no `venues` map; each Occurrence carries its location and a required `facebookEventUrl` (for a Manual Event, its one link); `publishedAt` becomes `updatedAt`; an Event has one or more Occurrences. Fixtures, the Go builder and validator, the TypeScript reader and both Sites change together.
- The snapshot holds every non-hidden, non-binned Event with effective values (override, else Facebook; the Moderator's values for Manual Events) and Translation fallback.

### 8. Social Posts, two ways

Autosync off: unchanged (Post Request, Meta sign-in, ADR 0005). Autosync on: posting is one request guarded by decision 13: check recent sign-in and the daily limit, record `pending`, call Graph with the stored token, record the outcome. Both ways share the `pending` record, the confirmed-retry rule, the daily limit and the Captions; Captions use effective values. Hidden or binned Events cannot be posted. Manual Events are posted like synced ones.

### 9. Admin SPA

- Event list: Events with flags (hidden, cancelled, manual, override or CMS text out of date, from a former page, synced id held by a Manual Event, in Bin), sync status with "Sync now", "Add co-hosted event", the reconnect and "reconnect before" banners.
- Manual Event screen: the fields of decision 20 with the Facebook link required, Occurrences added and removed one by one, CMS fields, Hide switch, Post dialog; a note that Sync does not update it.
- Synced Event screen: Facebook values read-only beside each override control ("Override" / "Follow Facebook again"), CMS fields, Hide switch, link to the Facebook Event, Post dialog.
- Connection screen: the Autosync switch with its warning and who turned it on, page, who connected, replace (confirmation naming both pages when the page differs, decision 17), disconnect with the T11 instructions, and the data access expiry date.
- Recent sign-in prompt for guarded actions; sign-in writes the session nonce (T5).
- Follows the design system rules in AGENTS.md; new patterns shown on `/app/design`, checked in light and dark.

### 10. Documentation

The public `/privacy` and `/data-deletion` pages the CMS serves to Meta and page owners (`cms/internal/http/auth.go`) say today that no Meta token is stored. They are rewritten: a Facebook page token is kept only while Autosync is on, what it is used for, and how to remove it (turn Autosync off; remove the app under Facebook, Settings, Business integrations).

With the `document-capability` skill: rewrite Event management and Facebook events (now Sync), update Social posting, Site delivery, Access (recent sign-in), Platform (Key Vault, timer, cost, open risks), `docs/README.md` and the root README runbooks (Meta app settings incl. "Require App Secret", connect, token incident response, removing Autofill from browsers).

### 11. Verification

Go tests for the domain (effective values, override flags, gone rule including "subcode 33 counts only after a successful list", Manual Event validation and its Facebook link), Sync against the recorder (cancelled and repeating events, a Manual Event's id met by Sync), the secret store suite, HTTP guards (recent sign-in, limits, timer), both token modes (Autosync on and off) for Sync and posting, and that turning Autosync off, or removing the enabling person's access, leaves no secret, and that off-then-on within the soft-delete window works; contract and Site tests; `bun test scripts`; a manual run of the local CMS through connect, sync, override, hide, a Manual Event and post with the recorder.

> **Final step:** when the implementation is done, run `/adversarial-review code`. Only after it ends, report the work as done or create a PR.
