# folk-ms documentation

folk-ms is a small CMS for bal folk Organizations (Bal Folk Poznań, Celtycki Wrocław). Moderators create events on their Organization's Facebook page; one Go CMS syncs them into bilingual Events, where Moderators add English text and more; each Organization's static Site reads a published JSON snapshot of its Events; Moderators announce Events as Social Posts on Facebook and Instagram. The target cost is about $2 a month on Azure, with every cloud service behind a port so that the CMS can move to a VPS.

This documentation is organized by capability. Terms are defined in [CONTEXT.md](../CONTEXT.md). The design dates from 2026-11-05; the first implementation from 2026-11-07. Since 2026-11-13 Facebook Events are the source of Events ([ADR 0009](architecture/adr/0009-facebook-events-are-the-source-of-events.md)). Decisions with alternatives are recorded as ADRs in [architecture/adr](architecture/adr/).

## Building blocks

| Block | What it is | Path |
| --- | --- | --- |
| CMS | One Go binary: admin SPA under `/app/*`, JSON API under `/api/*`, sign-in under `/auth/*`, all on one origin | [`cms/`](../cms/) |
| Admin SPA | TypeScript UI for Moderators in Polish or English, built with Bun and embedded into the CMS binary | [`cms/admin/`](../cms/admin/) |
| Storage | Private container `cms` for the CMS's documents; public container `pub` for snapshots and media | [`infra/azure/`](../infra/azure/) |
| Secret store | Key Vault (a `0600` file store locally or on a VPS) for the Meta app secret, `SESSION_KEY` and the Facebook page tokens of Organizations with Autosync on | [`infra/azure/`](../infra/azure/), [`cms/internal/adapters/keyvault/`](../cms/internal/adapters/keyvault/) |
| Contract | JSON Schema of the snapshot, shared fixtures and the TypeScript reader used by Sites | [`contract/`](../contract/) |
| Sites | One static website per Organization, each with its own design | [`sites/`](../sites/) |
| Site config | The fixed list of Organizations and Sites | [`config/sites.json`](../config/sites.json) |
| External services | Google and Facebook (sign-in), Meta Graph API (Sync and Social Posts) | |

## Capabilities

| Capability | What it covers |
| --- | --- |
| [Sync](sync/README.md) | Pulling an Organization's Facebook Events into Events, on demand or hourly with Autosync; the gone rule, former pages, the local recorder |
| [Event management](event-management/README.md) | Overrides and CMS-only fields of synced Events, Hide, Manual Events, images and the Bin |
| [Site delivery](site-delivery/README.md) | The per-Site snapshot, its contract, and how Sites read it |
| [Social posting](social-posting/README.md) | The Social Account Connection and its Autosync switch, and posting Events to Facebook and Instagram with a Meta sign-in or the stored token |
| [Access](access/README.md) | Sign-in, Super Admin, Moderators and Invitations, the Audit Log, the admin UI language |
| [Design system](design-system/README.md) | Colours, type, spacing, components and the collapsible app shell of the admin SPA, on desktop and phone; based on the work of [Felipe Cardona](https://dribbble.com/felipecardona/) |
| [Platform](platform/README.md) | Hosting, ports and adapters, dependency policy, infrastructure, CI/CD, local development, cost, open risks |

A Moderator creates the event on the Organization's Facebook page; **Sync** turns it into an Event, through the Facebook page of the Social Account Connection. **Event management** is what Moderators do in the CMS: add English text, Summary, Price and a Social Image, override a Facebook value, hide an Event, or enter a Manual Event for a co-hosted Facebook Event Sync cannot read. Every change feeds **Site delivery**, which rebuilds the Organization's snapshot that its Site reads without touching the CMS. Events on the Site can be announced through **Social posting**, because a Social Post links to the Event's Permalink. **Access** decides who may do any of this, per Organization. **Platform** describes where it all runs and how it is built.

## Read next

- [CONTEXT.md](../CONTEXT.md): the glossary
- [Platform](platform/README.md): start here before changing anything
- [ADRs](architecture/adr/): 0001 snapshot delivery, 0002 blob documents, 0003 dependency policy, 0004 hosting, 0005 Meta sign-in for every post (amended by 0010), 0006 Facebook Events through a Handoff and Autofill (superseded), 0007 latest toolchains and Dependabot cooldown, 0008 Facebook sign-in matched by email, 0009 Facebook Events are the source of Events, 0010 Autosync keeps a page token in Key Vault
- [Root README](../README.md): local setup and operations runbooks
