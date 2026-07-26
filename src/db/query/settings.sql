-- Runtime site controls (arch/ADMIN_MODERATION.md Phase 3). Read as a whole
-- table — there are a handful of rows and the settings package caches the
-- snapshot, so there is no per-key read path to optimize.

-- name: ListSettings :many
SELECT key, value FROM settings;

-- name: UpsertSetting :exec
-- Set one switch. An absent row means "default", so clearing a switch back to
-- its default is a delete, not a write of the default value — see DeleteSetting.
INSERT INTO settings (key, value, updated_by)
VALUES ($1, $2, $3)
ON CONFLICT (key) DO UPDATE
    SET value = excluded.value,
        updated_at = now(),
        updated_by = excluded.updated_by;

-- name: DeleteSetting :exec
-- Reset a switch to its built-in default by removing the override.
DELETE FROM settings WHERE key = $1;
