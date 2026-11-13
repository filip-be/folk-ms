# folk-ms documentation

folk-ms is a small CMS for bal folk Organizations (Bal Folk Poznań, Celtycki Wrocław). Moderators manage bilingual Events in one Go CMS; each Organization's static Site reads a published JSON snapshot of its Events; Moderators announce Published Events as Social Posts on Facebook and Instagram. The target cost is under $1 a month on Azure, with every cloud service behind a port so that the CMS can move to a VPS.

This documentation is organized by capability. Terms are defined in [CONTEXT.md](../CONTEXT.md). The design dates from 2026-11-05; the first implementation from 2026-11-07. Decisions with alternatives are recorded as ADRs in [architecture/adr](architecture/adr/).

## Building blocks

| Block | What it is | Path |
| --- | --- | --- |
| CMS | One Go binary: admin SPA under `/app/*`, JSON API under `/api/*`, sign-in under `/auth/*`, all on one origin | [`cms/`](../cms/) |
| Admin SPA | TypeScript UI for Moderators in Polish or English, built with Bun and embedded into the CMS binary | [`cms/admin/`](../cms/admin/) |
| Storage | Private container `cms` for the CMS's documents; public container `pub` for snapshots and media | [`infra/azure/`](../infra/azure/) |
| Contract | JSON Schema of the snapshot, shared fixtures and the TypeScript reader used by Sites | [`contract/`](../contract/) |
| Sites | One static website per Organization, each with its own design | [`sites/`](../sites/) |
| Site config | The fixed list of Organizations and Sites | [`config/sites.json`](../config/sites.json) |
| External services | Google and Facebook (sign-in), Meta Graph API (Social Posts) | |
| Autofill | Optional browser helper (WebExtension and userscript) that types a Facebook Event Handoff into Facebook's form | [`autofill/`](../autofill/) |

## Capabilities

| Capability | What it covers |
| --- | --- |
| [Event management](event-management/README.md) | Authoring Events, Recurrences, Venues and images; Draft, Publish, Unpublish and the Bin |
| [Site delivery](site-delivery/README.md) | The per-Site snapshot, its contract, and how Sites read it |
| [Social posting](social-posting/README.md) | Social Account Connections and posting Published Events to Facebook and Instagram, with a Meta sign-in for every post |
| [Facebook events](facebook-events/README.md) | Facebook Events created by hand from a Facebook Event Handoff, optionally typed in by Autofill; their links on Sites and the out-of-date reminders |
| [Access](access/README.md) | Sign-in, Super Admin, Moderators and Invitations, the Audit Log, the admin UI language |
| [Design system](design-system/README.md) | Colours, type, spacing, components and the collapsible app shell of the admin SPA, on desktop and phone; based on the work of [Felipe Cardona](https://dribbble.com/felipecardona/) |
| [Platform](platform/README.md) | Hosting, ports and adapters, dependency policy, infrastructure, CI/CD, local development, cost, open risks |

**Event management** is the core: Moderators edit Drafts and Publish them. Every Publish feeds **Site delivery**, which rebuilds the Organization's snapshot that its Site reads without touching the CMS. Only Published Events can be announced through **Social posting**, because a Social Post links to the Event's Permalink. The same holds for **Facebook events**: the CMS prepares a Facebook Event Handoff for a Published Event, the Moderator creates the Facebook Event on the Organization's Facebook page, and its link reaches the Site through **Site delivery**. **Access** decides who may do any of this, per Organization. **Platform** describes where it all runs and how it is built.

## Read next

- [CONTEXT.md](../CONTEXT.md): the glossary
- [Platform](platform/README.md): start here before changing anything
- [ADRs](architecture/adr/): 0001 snapshot delivery, 0002 blob documents, 0003 dependency policy, 0004 hosting, 0005 Meta sign-in for every post, 0006 Facebook Events through a Handoff and Autofill (superseded), 0007 latest toolchains and Dependabot cooldown, 0008 Facebook sign-in matched by email, 0009 Facebook Events are the source of Events, 0010 Autosync keeps a page token in Key Vault
- [Root README](../README.md): local setup and operations runbooks
