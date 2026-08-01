-- Per-account display preferences. Read as a whole set for one account — a
-- player holds a handful of overrides and the prefs package caches the resolved
-- snapshot, so there is no per-key read path to optimize.

-- name: ListUserPrefs :many
SELECT key, value FROM user_prefs WHERE user_id = $1;

-- name: UpsertUserPref :exec
-- Set one preference for one account. An absent row means "default", so
-- returning a preference to its default is a delete, not a write of the default
-- value — see DeleteUserPref.
INSERT INTO user_prefs (user_id, key, value)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, key) DO UPDATE
    SET value = excluded.value,
        updated_at = now();

-- name: DeleteUserPref :exec
-- Return one preference to its built-in default by removing the override.
DELETE FROM user_prefs WHERE user_id = $1 AND key = $2;
