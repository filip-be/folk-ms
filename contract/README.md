# contract

The Site snapshot contract; why it exists and how it is used is in [Site delivery](../docs/site-delivery/README.md).

| Path | What |
| --- | --- |
| `events.schema.json` | Canonical JSON Schema of `events.json` (draft 2020-12, restricted keyword set) |
| `videos.schema.json` | Canonical JSON Schema of `videos.json`, the Video Collection ([Video Collection](../docs/video-collection/README.md)) |
| `fixtures/valid/`, `fixtures/invalid/` | Shared fixtures for the Go and TypeScript sides (`fixtures/videos/` for `videos.json`) |
| `client/` | Zero-dependency TypeScript readers for Sites (`src/index.ts`, `src/videos.ts`) |

Tests: `bun test contract` (reader) and `go test ./internal/schema/ ./internal/domain/` in `cms/` (schema, fixtures, builder output).

When changing the schema: use only keywords listed in `cms/internal/schema/schema.go` (`SupportedKeywords`), update the fixtures, the Go snapshot types in `cms/internal/domain/snapshot.go` and the types in `client/src/index.ts`. A breaking change bumps `schemaVersion` on both sides.

Schema version 2 is such a breaking change: Events come from Facebook Events (or Manual Events), so the `venues` map, `recurrence`, `kind` and `publishedAt` are gone. Each Event has `updatedAt` and one or more Occurrences; each Occurrence carries its own optional `location` (`name`, optional `address` and `mapUrl`), `online` (present only when true) and a required `facebookEventUrl`. A repeating Facebook Event is one Event with several Occurrences, any of which may be cancelled; multi-day is derived from an Occurrence's dates. `price` has an optional `text` and an optional tickets `url`. The reader refuses any other `schemaVersion`, so the CMS and the Sites deploy together.

An optional property added later for Sites to show is additive: `schemaVersion` stays, because the reader ignores unknown properties. Deploy the Sites before or with the CMS that starts writing it.
