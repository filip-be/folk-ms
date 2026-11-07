# 0001. Sites read a published JSON snapshot

- Status: accepted
- Date: 2026-11-07

## Context

Each Organization has a static Site that must show its Events. Visitors outnumber Moderators by orders of magnitude, the budget is under $1 a month, and a CMS outage must not take a Site down.

## Decision

On every Publish, Unpublish and Venue edit the CMS rebuilds one JSON document per Site, `pub/sites/<site>/events.json`, and writes it to a public blob container with `Cache-Control: public, max-age=60`. Sites fetch it in the browser through `contract/client`. The document's shape is the hand-written JSON Schema `contract/events.schema.json`, tested on both sides against shared fixtures. Occurrences are expanded at Publish, so Sites do no date arithmetic.

## Alternatives

- **Sites call a CMS API.** Puts compute on the read path, couples Site uptime to the CMS, and costs executions per visit.
- **Rebuild and redeploy the static Site on every Publish.** Needs a build pipeline per Publish, minutes of delay, and deploy credentials in the CMS.
- **One document per Event.** More requests per page; one document per Site stays small for years (revisit when it does not).

## Consequences

- Visitors touch only the Static Web App and blob storage; the CMS can be down.
- A change is visible within about a minute (cache).
- Concurrent Publishes must not lose updates: the rebuild reads the snapshot ETag before the Events and writes with `If-Match`, retrying on conflict.
- No per-Event link previews in v1 (no prerendering).
