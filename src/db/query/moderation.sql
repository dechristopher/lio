-- Site administration & moderation (arch/ADMIN_MODERATION.md). The mutations
-- here are all privileged: the handlers in www/handlers/api/mod gate them on
-- role, refuse mod-on-mod actions, and write a mod_actions row for every one.

-- name: InsertModAction :one
-- Append one entry to the audit log. Called for every privileged action,
-- including title grants; reason is NOT NULL by schema, not by convention.
INSERT INTO mod_actions (actor_user_id, target_user_id, action, detail, reason)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: ListModActions :many
-- The global audit feed on /system, newest first, with both parties' display
-- names resolved (an id is useless to a human reading the log).
--
-- Two optional filters. @action narrows to one verb. @q is a single free-text
-- box matching the reason OR either party's username, so "drewtest" finds
-- everything that account was involved in, by them or against them — which is
-- the question a moderator actually asks. NULL/empty disables each.
--
-- The ILIKE has no index behind it; at this table's size a sequential scan of
-- a few thousand rows is cheaper than the pg_trgm extension it would take to
-- index, and the created_at index still serves the ordering.
SELECT m.id, m.created_at, m.action, m.detail, m.reason,
       a.username AS actor_username,
       t.username AS target_username
FROM mod_actions m
         JOIN users a ON a.id = m.actor_user_id
         LEFT JOIN users t ON t.id = m.target_user_id
WHERE (sqlc.narg('action')::text IS NULL OR m.action = sqlc.narg('action')::text)
  AND (sqlc.narg('q')::text IS NULL
    OR m.reason ILIKE '%' || sqlc.narg('q')::text || '%'
    OR a.username ILIKE '%' || sqlc.narg('q')::text || '%'
    OR t.username ILIKE '%' || sqlc.narg('q')::text || '%')
ORDER BY m.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountModActions :one
-- Total matching the same filters, for the pager. Kept beside ListModActions —
-- the two WHERE clauses must stay identical or the page count lies.
SELECT count(*)
FROM mod_actions m
         JOIN users a ON a.id = m.actor_user_id
         LEFT JOIN users t ON t.id = m.target_user_id
WHERE (sqlc.narg('action')::text IS NULL OR m.action = sqlc.narg('action')::text)
  AND (sqlc.narg('q')::text IS NULL
    OR m.reason ILIKE '%' || sqlc.narg('q')::text || '%'
    OR a.username ILIKE '%' || sqlc.narg('q')::text || '%'
    OR t.username ILIKE '%' || sqlc.narg('q')::text || '%');

-- name: ListModActionsForUser :many
-- One account's moderation history, shown on their player page to mods only.
SELECT m.id, m.created_at, m.action, m.detail, m.reason,
       a.username AS actor_username
FROM mod_actions m
         JOIN users a ON a.id = m.actor_user_id
WHERE m.target_user_id = $1
ORDER BY m.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountModActionsForUser :one
-- How many actions this account has on record, so the player page can say
-- whether the list it shows is the whole story.
SELECT count(*) FROM mod_actions WHERE target_user_id = $1;

-- name: AdminGrantor :one
-- Who last promoted this account to admin, from the audit log itself — the
-- log is the record of who did what, so it is also the record of who may undo
-- it. No row means the account's admin came from outside the app (the SQL
-- bootstrap), and nobody may demote it through the UI.
SELECT actor_user_id
FROM mod_actions
WHERE target_user_id = $1
  AND action = 'role'
  AND detail ->> 'to' = 'admin'
ORDER BY created_at DESC
LIMIT 1;

-- name: SetUserRole :exec
-- Appoint or demote. Admin-only at the handler; the "last admin cannot be
-- demoted" rule is enforced there against CountAdmins.
UPDATE users SET role = $2 WHERE id = $1;

-- name: SetUserBan :exec
-- Ban until $2 ('infinity' for permanent). The caller then revokes sessions,
-- drops the auth cache for the account, and forfeits any live game — see
-- ADMIN_MODERATION.md; this statement alone does not end a session.
UPDATE users SET banned_until = $2, ban_reason = $3 WHERE id = $1;

-- name: ClearUserBan :exec
-- Lift a ban early. An expired ban needs no statement: banned_until > now()
-- simply stops being true.
UPDATE users SET banned_until = NULL, ban_reason = NULL WHERE id = $1;

-- name: SetUserTitle :exec
-- Assign or clear ($2 NULL) an account's display title by titles row id.
UPDATE users SET title_id = $2 WHERE id = $1;

-- name: ForceRenameUser :exec
-- A moderator's forced rename, for a username that beat the registration
-- filter. Deliberately does NOT touch username_changed_at: the one-time
-- self-service rename allowance is the player's, and a sanction must not
-- consume it. Uniqueness is still the lower(username) index, whose violation
-- the caller maps like registration does.
UPDATE users SET username = $2 WHERE id = $1;

-- name: CountAdmins :one
-- Guards the last-admin rule on demotion.
SELECT count(*) FROM users WHERE role = 'admin';

-- name: ListTitles :many
-- The title picker on the player page's mod bar.
SELECT id, code, name FROM titles ORDER BY code;

-- name: ListModeratorIDs :many
-- Every account that may moderate, for pushing the unread-feedback count to
-- their open sockets (arch/NOTIFICATIONS.md). Feedback read state is site-wide,
-- so the badge belongs to all of them at once and there is no per-account row
-- to address instead. Tiny by nature: this is the site's staff, not its users.
SELECT id FROM users WHERE role IN ('mod', 'admin');

-- name: SearchUsers :many
-- The player picker behind the operator message composer on /system
-- (arch/NOTIFICATIONS.md Phase 3). A substring match rather than a prefix one,
-- so a moderator who remembers the middle of a name still finds it.
--
-- Ordered by how close the match is, not alphabetically: an exact name first,
-- then the shortest names — a four-character query matching a four-character
-- account is a better answer than the same query inside a twenty-character one.
-- Banned accounts are not filtered out: this is a staff tool, and hiding an
-- account a moderator is looking for is worse than showing one they cannot
-- usefully message.
SELECT id, username FROM users
WHERE username ILIKE $1
ORDER BY (lower(username) = lower($2)) DESC,
         length(username),
         lower(username)
LIMIT $3;

-- name: ListStaff :many
-- Everyone who holds a role above player, for the staff overview
-- (arch/ADMIN_MODERATION.md). Two surfaces read it: the panel on /system and
-- the public /staff page. One query serves both, and the *handler* decides
-- which columns reach a page — the appointment trail is staff-only, and gating
-- it here would mean a second query that could drift from this one.
--
-- The LATERAL is the appointment: the audit log is the record of who did what,
-- so it is also the record of who granted this role and when. It matches on the
-- role the account holds *now*, so an admin who was a mod first shows the
-- promotion that put them where they are rather than their first appointment.
-- No row means the role came from outside the app (the SQL bootstrap), which is
-- exactly the account that has no grantor and cannot be demoted through the UI.
--
-- Admins first, then alphabetically. The staff is a handful of rows, so the
-- ordering is for the reader rather than for a pager.
SELECT u.id,
       u.username,
       u.role,
       u.created_at,
       t.code                              AS title_code,
       t.name                              AS title_name,
       (u.banned_until IS NOT NULL)::bool  AS sanctioned,
       g.created_at                        AS granted_at,
       -- COALESCE, not the bare column: the LATERAL is a LEFT join, so this is
       -- NULL for a bootstrapped role, and sqlc cannot see that through a
       -- LATERAL — it would type the column as a plain string and the scan
       -- would fail on exactly the row that matters.
       COALESCE(g.actor_username, '')      AS granted_by
FROM users u
         LEFT JOIN titles t ON t.id = u.title_id
         LEFT JOIN LATERAL (
    SELECT m.created_at, a.username AS actor_username
    FROM mod_actions m
             JOIN users a ON a.id = m.actor_user_id
    WHERE m.target_user_id = u.id
      AND m.action = 'role'
      AND m.detail ->> 'to' = u.role
    ORDER BY m.created_at DESC
    LIMIT 1
    ) g ON TRUE
WHERE u.role IN ('mod', 'admin')
ORDER BY (u.role = 'admin') DESC, lower(u.username);
