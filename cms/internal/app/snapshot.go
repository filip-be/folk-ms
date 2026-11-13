package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/filip-be/folk-ms/cms/internal/domain"
	"github.com/filip-be/folk-ms/cms/internal/ports"
)

// SnapshotCacheControl is the Cache-Control of snapshots.
const SnapshotCacheControl = "public, max-age=60"

// RebuildSnapshot rebuilds an Organization's Site snapshot from its
// documents and writes it with If-Match, retrying on concurrent rebuilds.
//
// The snapshot's ETag is read before the Events: a concurrent rebuild that
// wrote a newer snapshot after we read the Events then makes our write fail
// and retry, so a stale build can never overwrite a fresher one.
func (a *App) RebuildSnapshot(ctx context.Context, org string) error {
	o, ok := a.orgByID[org]
	if !ok {
		return fmt.Errorf("%w: organization", domain.ErrNotFound)
	}
	path := SnapshotPath(o.Site.ID)
	for range 8 {
		cur, err := a.Pub.Get(ctx, path)
		etag := cur.ETag
		if errors.Is(err, ports.ErrNotFound) {
			etag = ""
		} else if err != nil {
			return err
		}
		events, err := a.events(ctx, org)
		if err != nil {
			return err
		}
		snap := domain.BuildSnapshot(o, events, a.Pub.URL, a.now())
		data, err := json.Marshal(snap)
		if err != nil {
			return err
		}
		_, err = a.Pub.Put(ctx, path, data, "application/json", SnapshotCacheControl, etag)
		if errors.Is(err, ports.ErrPrecondition) {
			continue
		}
		return err
	}
	return fmt.Errorf("%w: snapshot kept changing", domain.ErrConflict)
}

// RebuildAll rebuilds every Organization's snapshot (startup and repair).
func (a *App) RebuildAll(ctx context.Context) error {
	var errs []error
	for _, o := range a.orgs {
		errs = append(errs, a.RebuildSnapshot(ctx, o.ID))
	}
	return errors.Join(errs...)
}
