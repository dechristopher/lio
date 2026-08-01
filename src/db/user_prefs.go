package db

import (
	"github.com/dechristopher/lio/db/gen"
)

// Per-account display preferences. Thin storage accessors only: the keys, the
// defaults, the typing of values and the read cache all live in the prefs
// package, which is the one place that knows what a key means.
//
// Degrades to "no overrides" without Postgres, so a PG-less local dev server
// renders every preference at its built-in default rather than failing. That
// costs nothing there: without Postgres there are no accounts to hold a
// preference in the first place.

// LoadUserPrefs reads one account's stored overrides as a raw key/value map.
func LoadUserPrefs(userID int64) (map[string]string, error) {
	if Pool == nil {
		return map[string]string{}, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListUserPrefs(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

// SetUserPref stores (or replaces) one override for one account.
func SetUserPref(userID int64, key, value string) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).UpsertUserPref(ctx, gen.UpsertUserPrefParams{
		UserID: userID,
		Key:    key,
		Value:  value,
	})
}

// ClearUserPref removes an override, returning the preference to its built-in
// default. Deleting rather than writing the default keeps "unset" and
// "explicitly set to the current default" from drifting apart when a default
// later changes.
func ClearUserPref(userID int64, key string) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteUserPref(ctx, gen.DeleteUserPrefParams{
		UserID: userID,
		Key:    key,
	})
}
