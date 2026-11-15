package domain

import (
	"fmt"
	"slices"
	"time"
)

// FeatureFlag names a part of the CMS the Super Admin can turn on or off
// while it runs. Only the flags in featureFlags exist; any other name is
// refused.
type FeatureFlag string

// The known Feature Flags. To add one, add a constant and its entry in
// featureFlags; the API, the Super Admin's screen and the Audit Log pick it
// up from there.
const (
	// FeatureFacebookSignIn offers sign-in to the CMS with Facebook (ADR 0008).
	// It does not gate the Meta sign-in for the Social Account Connection,
	// Sync or Social Posts.
	FeatureFacebookSignIn FeatureFlag = "facebookSignIn"
)

// FeatureFlagDef describes a known Feature Flag.
type FeatureFlagDef struct {
	Key         FeatureFlag `json:"key"`
	Name        Text        `json:"name"`
	Description Text        `json:"description"`
	// Default is the value while the Super Admin has not set it.
	Default bool `json:"default"`
}

var featureFlags = []FeatureFlagDef{
	{
		Key:  FeatureFacebookSignIn,
		Name: Text{PL: "Logowanie przez Facebooka", EN: "Facebook sign-in"},
		Description: Text{
			PL: "Ekran logowania oferuje logowanie do CMS przez Facebooka, a moderatorzy mogą połączyć z nim konto. Wymaga aplikacji Meta, która na to pozwala. Nie dotyczy logowania do Meta przy łączeniu strony, synchronizacji i postach.",
			EN: "The sign-in screen offers signing in to the CMS with Facebook, and Moderators can link it to their account. Needs a Meta app that allows it. Does not affect the Meta sign-in for connecting the Facebook page, Sync and Social Posts.",
		},
	},
}

// FeatureFlagDefs lists the known Feature Flags in display order.
func FeatureFlagDefs() []FeatureFlagDef { return slices.Clone(featureFlags) }

// FeatureFlagDefFor returns the definition of a known Feature Flag.
func FeatureFlagDefFor(k FeatureFlag) (FeatureFlagDef, bool) {
	for _, d := range featureFlags {
		if d.Key == k {
			return d, true
		}
	}
	return FeatureFlagDef{}, false
}

// FeatureFlagValue is a value the Super Admin set, and who set it when.
type FeatureFlagValue struct {
	Enabled        bool      `json:"enabled"`
	ChangedAt      time.Time `json:"changedAt"`
	ChangedBy      string    `json:"changedBy"`
	ChangedByEmail string    `json:"changedByEmail,omitempty"`
}

// FeatureFlags is the stored state of the Feature Flags: the values set so
// far. A flag never set has its default; a stored value of a flag that no
// longer exists is ignored.
type FeatureFlags struct {
	Values map[FeatureFlag]FeatureFlagValue `json:"values"`
}

// Enabled reports whether a flag is on. f may be nil (nothing set yet);
// an unknown flag is off.
func (f *FeatureFlags) Enabled(k FeatureFlag) bool {
	d, ok := FeatureFlagDefFor(k)
	if !ok {
		return false
	}
	if f != nil {
		if v, ok := f.Values[k]; ok {
			return v.Enabled
		}
	}
	return d.Default
}

// Set turns a known flag on or off (ErrNotFound for an unknown one). It
// reports whether the value changed.
func (f *FeatureFlags) Set(k FeatureFlag, on bool, by, byEmail string, now time.Time) (bool, error) {
	if _, ok := FeatureFlagDefFor(k); !ok {
		return false, fmt.Errorf("%w: feature flag %q", ErrNotFound, k)
	}
	if f.Enabled(k) == on {
		return false, nil
	}
	if f.Values == nil {
		f.Values = map[FeatureFlag]FeatureFlagValue{}
	}
	f.Values[k] = FeatureFlagValue{Enabled: on, ChangedAt: now, ChangedBy: by, ChangedByEmail: byEmail}
	return true, nil
}
