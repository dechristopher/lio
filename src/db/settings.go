package db

import (
	"github.com/dechristopher/lio/db/gen"
)

// Runtime site controls (arch/ADMIN_MODERATION.md Phase 3). Thin storage
// accessors only: the defaults, the typing of values, and the read cache all
// live in the settings package, which is the one place that knows what a key
// means. Degrades to "no overrides" without Postgres, so a PG-less local dev
// server runs on built-in defaults rather than failing.

// LoadSettings reads every stored override as a raw key/value map.
func LoadSettings() (map[string]string, error) {
	if Pool == nil {
		return map[string]string{}, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

// SetSetting stores (or replaces) one override, stamping who changed it.
func SetSetting(key, value string, updatedBy int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).UpsertSetting(ctx, gen.UpsertSettingParams{
		Key:       key,
		Value:     value,
		UpdatedBy: &updatedBy,
	})
}

// ClearSetting removes an override, returning the switch to its built-in
// default. Deleting rather than writing the default keeps "unset" and
// "explicitly set to the current default" from drifting apart when a default
// later changes.
func ClearSetting(key string) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteSetting(ctx, key)
}
