# contract

The Site snapshot contract; why it exists and how it is used is in [Site delivery](../docs/site-delivery/README.md).

| Path | What |
| --- | --- |
| `events.schema.json` | Canonical JSON Schema (draft 2020-12, restricted keyword set) |
| `fixtures/valid/`, `fixtures/invalid/` | Shared fixtures for the Go and TypeScript sides |
| `client/` | Zero-dependency TypeScript reader for Sites |

Tests: `bun test contract` (reader) and `go test ./internal/schema/ ./internal/domain/` in `cms/` (schema, fixtures, builder output).

When changing the schema: use only keywords listed in `cms/internal/schema/schema.go` (`SupportedKeywords`), update the fixtures, the Go snapshot types in `cms/internal/domain/snapshot.go` and the types in `client/src/index.ts`. A breaking change bumps `schemaVersion` on both sides.

An optional property added for Sites to show (such as `facebookEventUrl` on an Occurrence, from [Facebook events](../docs/facebook-events/README.md)) is additive: `schemaVersion` stays, because the reader ignores unknown properties. Deploy the Sites before or with the CMS that starts writing it.
