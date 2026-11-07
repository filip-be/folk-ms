# folk-ms documentation

folk-ms is a small CMS for bal folk Organizations (Bal Folk Poznań, Celtycki Wrocław). Moderators manage bilingual Events in one Go CMS; each Organization's static Site reads a published JSON snapshot of its Events; Moderators announce Published Events as Social Posts on Facebook and Instagram. The target cost is under $1 a month on Azure, with every cloud service behind a port so that the CMS can move to a VPS.

This documentation is organized by capability. Terms are defined in [CONTEXT.md](../CONTEXT.md). The design dates from 2026-11-05; the first implementation from 2026-11-07. Decisions with alternatives are recorded as ADRs in [architecture/adr](architecture/adr/).

## Building blocks

| Block | What it is | Path |
| --- | --- | --- |
| CMS | One Go binary: admin SPA under `/app/*`, JSON API under `/api/*`, sign-in under `/auth/*`, all on one origin | [`cms/`](../cms/) |
| Admin SPA | TypeScript UI for Moderators, built with Bun and embedded into the CMS binary | [`cms/admin/`](../cms/admin/) |
| Storage | Private container `cms` for the CMS's documents; public container `pub` for snapshots and media | [`infra/azure/`](../infra/azure/) |
| Contract | JSON Schema of the snapshot, shared fixtures and the TypeScript reader used by Sites | [`contract/`](../contract/) |
| Sites | One static website per Organization, each with its own design | [`sites/`](../sites/) |
| Site config | The fixed list of Organizations and Sites | [`config/sites.json`](../config/sites.json) |
| External services | Google (sign-in), Meta Graph API (Social Posts) | |

## Capabilities

| Capability | What it covers |
| --- | --- |
| [Event management](event-management/README.md) | Authoring Events, Recurrences, Venues and images; Draft, Publish, Unpublish and the Bin |
| [Site delivery](site-delivery/README.md) | The per-Site snapshot, its contract, and how Sites read it |
| [Social posting](social-posting/README.md) | Social Account Connections and posting Published Events to Facebook and Instagram |
| [Access](access/README.md) | Sign-in, Super Admin, Moderators and Invitations, the Audit Log |
| [Platform](platform/README.md) | Hosting, ports and adapters, dependency policy, infrastructure, CI/CD, local development, cost, open risks |

**Event management** is the core: Moderators edit Drafts and Publish them. Every Publish feeds **Site delivery**, which rebuilds the Organization's snapshot that its Site reads without touching the CMS. Only Published Events can be announced through **Social posting**, because a Social Post links to the Event's Permalink. **Access** decides who may do any of this, per Organization. **Platform** describes where it all runs and how it is built.

## Read next

- [CONTEXT.md](../CONTEXT.md): the glossary
- [Platform](platform/README.md): start here before changing anything
- [ADRs](architecture/adr/): 0001 snapshot delivery, 0002 blob documents, 0003 dependency policy, 0004 hosting
- [Root README](../README.md): local setup and operations runbooks
