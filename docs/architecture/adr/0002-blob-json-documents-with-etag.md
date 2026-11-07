# 0002. Data as JSON documents in blob storage with ETag concurrency

- Status: accepted
- Date: 2026-11-07

## Context

A few Moderators manage hundreds of Events. A database service would dominate the cost, and the CMS must be able to move to a VPS.

## Decision

Every aggregate is one JSON document in the private `cms` container (`orgs/<org>/events/<id>.json`, `users/<id>.json`, and so on) behind the `DocumentStore` port: Get, Put with an ETag precondition (empty = create-only, `*` = unconditional), List by prefix, Delete. Editors send the ETag they loaded (`If-Match`), so two Moderators cannot silently overwrite each other. Uniqueness that needs an index (an Event's `shortId`) uses a create-only index document. The Audit Log is a separate append-only port (one append blob per day). Blob versioning and soft delete are on for recovery.

Adapters: Azure Blob REST (`azblob`), filesystem with atomic rename and a lock file (`fs`), and memory. All run the same contract tests (`cms/internal/adapters/storetest`); the blob adapter runs them against Azurite.

## Alternatives

- **Azure SQL or Cosmos DB.** Costs more than the whole budget, or adds an SDK and a query engine that hundreds of documents do not need.
- **Table storage.** Cheap, but another API and a poor fit for nested Events.
- **SQLite on a file share.** Locking over SMB is unreliable on serverless compute.

## Consequences

- Lists read every document of an Organization (in parallel); fine at hundreds of Events. An index document is the next step if it is not.
- Cross-document changes (a Venue edit freezing past Occurrences in many Events) are sequences of single-document compare-and-swap writes, not transactions; each step is idempotent.
