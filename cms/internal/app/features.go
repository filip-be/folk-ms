package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/filip-be/folk-ms/cms/internal/domain"
)

// featureFlagsKey is the one document holding every Feature Flag set so far.
const featureFlagsKey = "system/feature-flags.json"

// FeatureFlagView is a Feature Flag as the Super Admin sees it.
type FeatureFlagView struct {
	domain.FeatureFlagDef
	Enabled        bool       `json:"enabled"`
	ChangedAt      *time.Time `json:"changedAt,omitempty"`
	ChangedByEmail string     `json:"changedByEmail,omitempty"`
}

// loadFeatureFlags reads the stored flags; a missing document means none
// was set yet (etag "": create it).
func (a *App) loadFeatureFlags(ctx context.Context) (*domain.FeatureFlags, string, error) {
	f, etag, err := load[domain.FeatureFlags](ctx, a.Docs, featureFlagsKey)
	if errors.Is(err, domain.ErrNotFound) {
		return &domain.FeatureFlags{}, "", nil
	}
	return f, etag, err
}

func featureFlagViews(f *domain.FeatureFlags) []FeatureFlagView {
	defs := domain.FeatureFlagDefs()
	out := make([]FeatureFlagView, 0, len(defs))
	for _, d := range defs {
		v := FeatureFlagView{FeatureFlagDef: d, Enabled: f.Enabled(d.Key)}
		if set, ok := f.Values[d.Key]; ok {
			at := set.ChangedAt
			v.ChangedAt, v.ChangedByEmail = &at, set.ChangedByEmail
		}
		out = append(out, v)
	}
	return out
}

// FeatureFlags lists every known Feature Flag with its value (Super Admin
// only).
func (a *App) FeatureFlags(ctx context.Context, p domain.Principal) ([]FeatureFlagView, error) {
	if err := requireSuperAdmin(p); err != nil {
		return nil, err
	}
	f, _, err := a.loadFeatureFlags(ctx)
	if err != nil {
		return nil, err
	}
	return featureFlagViews(f), nil
}

// SetFeatureFlag turns a Feature Flag on or off and answers every flag. A
// flag can change how people sign in, so it is guarded like an access
// change: the Super Admin with a Recent sign-in.
func (a *App) SetFeatureFlag(ctx context.Context, p domain.Principal, key string, on bool) ([]FeatureFlagView, error) {
	if err := a.requireAccessChange(p); err != nil {
		return nil, err
	}
	k := domain.FeatureFlag(key)
	if _, ok := domain.FeatureFlagDefFor(k); !ok {
		return nil, fmt.Errorf("%w: feature flag", domain.ErrNotFound)
	}
	for range 5 {
		f, etag, err := a.loadFeatureFlags(ctx)
		if err != nil {
			return nil, err
		}
		changed, err := f.Set(k, on, p.ID(), p.User.Email, a.now())
		if err != nil {
			return nil, err
		}
		if !changed {
			return featureFlagViews(f), nil
		}
		if _, err := save(ctx, a.Docs, featureFlagsKey, f, etag); errors.Is(err, domain.ErrConflict) {
			continue
		} else if err != nil {
			return nil, err
		}
		a.audit(ctx, p, "feature_flag.changed", "", key, map[string]any{"enabled": on})
		return featureFlagViews(f), nil
	}
	return nil, fmt.Errorf("%w: too many concurrent changes to %s", domain.ErrConflict, featureFlagsKey)
}

// FeatureEnabled reports whether a Feature Flag is on, for any caller (the
// sign-in screen is shown before sign-in). When the flags cannot be read it
// answers off and logs: a gated feature stays shut rather than open.
func (a *App) FeatureEnabled(ctx context.Context, k domain.FeatureFlag) bool {
	f, _, err := a.loadFeatureFlags(ctx)
	if err != nil {
		a.Log.Error("feature flags unreadable; treating as off", "flag", k, "err", err)
		return false
	}
	return f.Enabled(k)
}
