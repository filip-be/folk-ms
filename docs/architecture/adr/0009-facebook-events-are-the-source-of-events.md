# 0009. Facebook Events are the source of Events

- Status: accepted
- Date: 2026-11-13
- Supersedes [ADR 0006](0006-facebook-events-through-handoff-and-autofill.md)

## Context

Under [ADR 0006](0006-facebook-events-through-handoff-and-autofill.md) the CMS was the source of truth: a Moderator authored an Event in the CMS, published it, and then created the matching Facebook Event by hand from a Facebook Event Handoff, optionally typed by Autofill. Every Event was entered twice, Autofill broke whenever Facebook changed its form, and it acted inside Moderators' own Facebook sessions, which Meta's terms restrict.

Moderators create their events on Facebook first anyway: that is where the audience is. The Graph API cannot create events, but it can read them.

## Spike results (2026-11-13)

A throwaway command (`cms/cmd/fbspike`, Graph v24.0, a test Facebook page, the app in Standard Access with the Moderator as Tester):

- `GET /{page-id}/events` with the Facebook page token returns the events the page created, with `pages_read_engagement`. Cancelled events are left out unless `include_canceled=true`. A user token sees fewer; the page token is used.
- Fields returned: `id`, `name`, `description`, `start_time`, `timezone`, `place` (often only a free-text name), `is_canceled`, `updated_time`, `is_online`, `type`, `is_draft`, `category`; when set, `end_time`, `cover`, `ticket_uri`, `event_times`. `owner` is never returned.
- A repeating event lists its dates in `event_times`, each with its own id, readable by id.
- An event another page created, even one the Organization's page co-hosts, is neither listed nor readable by id: that needs Page Public Content Access, which this app does not have, and Meta refuses `user_events` at sign-in.
- A deleted event read by id answers code 100, subcode 33 ("does not exist, cannot be loaded due to missing permissions"), the same answer as for a missing permission.
- One full run made about 30 Graph calls, far from any rate limit.

## Decision

Facebook is the source of Events. A Moderator creates and edits the event on Facebook; the CMS pulls it with **Sync** and the Site shows it.

- Sync lists the connected Facebook page's events with `include_canceled=true`, from those upcoming at the first Sync onward. A repeating Facebook Event becomes one Event with one Permalink and one Occurrence per date of `event_times`, each linking to its own Facebook page.
- A synced Event is on the Site at once. A Hide switch keeps it off, and Sync never undoes it. Draft, Publish and Unpublish are gone.
- The CMS adds what Facebook lacks: English name and description, Summary, Price text, Social Image. Facebook text is the Polish content.
- A Moderator may **Override** a Facebook-sourced field (name, description, location, cover, ticket link); the override wins. When Facebook later changes that field, the override stays and the Event is flagged, and the Moderator can follow Facebook again. Dates and cancellation are never overridden. English text and the Summary are flagged as possibly out of date when the Facebook text they were written against changes.
- An event missing from the list is read by id. It counts as gone only when Graph answers code 100, subcode 33 **and** the same run listed the page's events successfully with the same token; then the Event moves to the Bin. Any other answer changes nothing.
- An event another page created (typically one the Organization co-hosts) cannot be read, so a Moderator enters it as a **Manual Event**: names, descriptions, dates one by one, Location, Cover Image, ticket link, and a required link to its Facebook Event. Sync never touches it. One Facebook Event id is held by one Event only, synced or manual.
- When the connection changes to another Facebook page, the old page's Events stay as they were and are no longer synced.
- Sync runs on demand ("Sync now", with a Meta sign-in as for posting) and, for Organizations that opt in to **Autosync**, hourly with a stored Facebook page token; see [ADR 0010](0010-autosync-keeps-a-page-token-in-key-vault.md).
- Existing CMS Events, Venues, Recurrence rules, the Handoff and Autofill are deleted.

## Alternatives

- **Keep the Handoff and Autofill** (ADR 0006): double entry and a fragile, terms-restricted helper.
- **Page Public Content Access**, to read co-hosted events: an App Review for a feature meant for public content analysis; out of scope for a small association's app.
- **Facebook Event as an optional link on CMS Events** (as before ADR 0006): keeps double entry.

## Consequences

- Events are entered once, on Facebook. CMS authoring is limited to the additions and to Manual Events.
- The Site depends on Graph returning page events to Standard Access apps; if Meta closes that, Organizations fall back to Manual Events.
- The reusable Venue list goes: Location is free text from Facebook.
- Co-hosted events created elsewhere need Manual Events, kept up to date by hand.
- The snapshot contract moves to schema version 2 (no Recurrence, no Venues, a Facebook link on every Occurrence).
