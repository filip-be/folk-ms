# Video Collection

Each Organization keeps a Video Collection ("wideopomoc", video help): an ordered list of instructional dance Videos on YouTube or Vimeo, described with the Organization's own lists of Forms and Origins. Moderators manage it in the admin SPA; the CMS publishes it as a second per-Site snapshot, `videos.json`, which a Site may show. It uses the CMS, the admin SPA, Storage, the contract and the Sites. Started for Bal Folk Poznań (issue #7).

## How it works

- **One document per Organization.** The collection lives in the private store as `orgs/<org>/videos.json`: the Forms, the countries with their regions, the Videos, each list in the order the Moderators set. One document makes reordering and the "in use" checks atomic. Every change carries the version the screen loaded (`If-Match`); a stale version is refused with a conflict instead of overwriting another Moderator's change. A collection never saved reads as empty with the version `new`, and the first change creates the document.
- **Access** is the same as for Events: Moderators of the Organization and the Super Admin. Every change is written to the Audit Log (`video.*`, `form.*`, `origin.*`); a deleted Video keeps its title and link there.
- **Links** are reduced to provider, video id, the unlisted Vimeo hash and an optional start time; the pasted string is never kept or echoed. Only YouTube (`youtube.com`, `m.`, `music.`, `youtu.be`, shorts, embed) and Vimeo (`vimeo.com`, `player.vimeo.com`) are accepted. The start comes from `t=`, `start=` or `#t=` in the link, or from the m:ss field, which wins. The admin form asks `POST /api/orgs/<org>/videos/link-preview` as soon as a link is entered, so one Go parser decides what is saved.
- **Thumbnails:** YouTube's is derived from the id (`i.ytimg.com`). For Vimeo the CMS calls Vimeo oEmbed (`vimeo.com/api/oembed.json`, no key, 5 s timeout) on preview and when a Video is added or its link points to another video, never on read; a video Vimeo does not know or will not embed cannot be saved. Locally a fake answers unless `VIMEO_OEMBED=1`. The admin CSP allows images (only) from `i.ytimg.com` and `i.vimeocdn.com`.
- **Forms and Origins** are bilingual (Polish required, English optional), manually ordered, and refused when a duplicate. A Form, region or country that a Video uses cannot be removed (the answer names how many use it), nor a country that still has regions. Ids are 8-character base32 and never reused.
- **Snapshot:** every change, and every CMS start (and `POST /api/admin/rebuild`), rebuilds `pub/sites/<site>/videos.json` for every Organization, possibly empty: the Forms and Origins in list order (used or not; the reader hides empty ones) and the Videos that are not Hidden, in order, each with its canonical watch `url`, its player `embedUrl` (`www.youtube-nocookie.com` or `player.vimeo.com`, with the start) and its `thumbnail`. It has its own schema and `schemaVersion` (1); `events.json` is untouched. Written with ETag compare-and-swap like `events.json` ([Site delivery](../site-delivery/README.md)).
- **Reader:** `contract/client/src/videos.ts` reads the snapshot and gives the helpers a page needs: filter by Forms (any of them), by Origin (a country includes its regions) and by title (case- and diacritics-insensitive, Polish or English), grouping by Form (a Video under each of its Forms, the rest under Other) or by Origin (countries with region subgroups, the rest under Other), and the player address.
- **Sites** compose page modules from `sites/shared/` ([`sites/README.md`](../../sites/README.md)). Only Bal Folk Poznań shows the collection for now, at `/<lang>/videos`, as a proof of concept: chips for Forms, country and region selects, a title search and Group by, all kept in the query string (a filtered view can be shared; Back and Forward work). A card shows the thumbnail; the player loads only when the visitor clicks it, which is why Poznań's CSP allows `frame-src` for the two embed hosts. Celtycki Wrocław has a `videos.json` but no page yet.

## Where it lives

| What | Path |
| --- | --- |
| Domain: Video, Forms, Origins, link parsing, snapshot builder | [`cms/internal/domain/video.go`](../../cms/internal/domain/video.go) |
| Use cases and snapshot rebuild | [`cms/internal/app/videos.go`](../../cms/internal/app/videos.go) |
| API routes | [`cms/internal/http/api.go`](../../cms/internal/http/api.go) |
| Vimeo oEmbed adapter and the fake | [`cms/internal/adapters/vimeo/`](../../cms/internal/adapters/vimeo/), [`cms/internal/adapters/memory/videoinfo.go`](../../cms/internal/adapters/memory/videoinfo.go) |
| Admin screens | [`cms/admin/src/views/videos.ts`](../../cms/admin/src/views/videos.ts), [`cms/admin/src/views/video.ts`](../../cms/admin/src/views/video.ts) |
| Schema, fixtures, reader | [`contract/videos.schema.json`](../../contract/videos.schema.json), [`contract/fixtures/videos/`](../../contract/fixtures/videos/), [`contract/client/src/videos.ts`](../../contract/client/src/videos.ts) |
| Site page module | [`sites/shared/videos.ts`](../../sites/shared/videos.ts), wired in [`sites/balfolk-poznan/src/main.ts`](../../sites/balfolk-poznan/src/main.ts) |

## Change together

| Rule | Files | What breaks |
| --- | --- | --- |
| The schema, the Go builder and the TypeScript reader describe the same `videos.json`, at the same `schemaVersion` | `contract/videos.schema.json`, `contract/fixtures/videos/`, `cms/internal/domain/video.go`, `contract/client/src/videos.ts`, `sites/shared/videos.ts` | Sites fail to read or misread Videos. Enforced in CI: `cms/internal/schema` validates the fixtures, `cms/internal/domain` validates built snapshots, `contract/client/test/videos.test.ts` reads them. |
| A Site that plays Videos allows exactly the embed hosts in its CSP `frame-src` | `sites/<site>/staticwebapp.config.json`, `cms/internal/domain/video.go` (`EmbedURL`), `contract/client/src/videos.ts` (`EMBED_HOSTS`) | The player stays blank after a click. Enforced for Bal Folk Poznań by `sites/shared/videos.test.ts`; the schema pins the embed hosts on the CMS side. |
| The admin CSP allows the thumbnail hosts | `cms/internal/http/server.go` (`img-src`), `cms/internal/domain/video.go` (`YouTubeThumbnail`), `cms/internal/adapters/vimeo/vimeo.go` (`ThumbnailHost`) | Thumbnails in the admin are blank. `http_test.go` checks the CSP side. |
| A Video problem path has a matching `data-field` | `cms/internal/domain/video.go`, `cms/admin/src/views/video.ts`, `cms/admin/src/views/videos.ts` | The problem shows only in the summary (see [Design system, Change together](../design-system/README.md#change-together)). |

## Read next

- [CONTEXT.md](../../CONTEXT.md#video-collection): Video Collection, Video, Form, Origin, Hidden
- [Site delivery](../site-delivery/README.md): the snapshot pattern this follows
- [Access](../access/README.md): Moderators and the Audit Log
- [`contract/README.md`](../../contract/README.md), [`sites/README.md`](../../sites/README.md): building and testing
