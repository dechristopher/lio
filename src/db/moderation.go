package db

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/role"
)

// The moderation data plane (arch/ADMIN_MODERATION.md): role changes,
// sanctions, and the append-only audit log. Like the rest of the accounts data
// plane these are only ever called with a live pool — the handlers above them
// gate on auth.Enabled() first.
//
// Every mutation here has a matching LogModAction call in its handler. That
// pairing is the whole accountability model, so it lives at the handler (which
// knows the actor, the reason, and whether the action was actually allowed)
// rather than being smuggled into these wrappers.

// BanState is an account's sanction status, decoded from users.banned_until /
// ban_reason. The column encodes both cases in one value — NULL for no ban,
// 'infinity' for a permanent one — so this type unpacks that into something the
// display and enforcement layers can read without knowing the encoding.
type BanState struct {
	// Banned reports whether a sanction is currently in force. An expired ban
	// is simply not Banned: it lapses on its own, with no sweeper.
	Banned bool
	// Permanent marks the 'infinity' case; Until is meaningless when set.
	Permanent bool
	// Until is when a temporary ban lifts (zero when Permanent or unbanned).
	Until time.Time
	// Reason is the moderator's stated reason. Shown to moderators only — the
	// public "account closed" state deliberately carries no reason.
	Reason string
}

// banFrom decodes the ban columns into a BanState, resolving "is it still in
// force" against the current time (Postgres would answer the same way; doing it
// here keeps every read path consistent without a now() round trip).
func banFrom(until pgtype.Timestamptz, reason *string) BanState {
	if !until.Valid {
		return BanState{}
	}
	b := BanState{Reason: strOrEmpty(reason)}
	switch {
	case until.InfinityModifier == pgtype.Infinity:
		b.Banned, b.Permanent = true, true
	case until.InfinityModifier == pgtype.NegativeInfinity:
		// nonsensical, but fail closed to "not banned" rather than panic
		return BanState{}
	default:
		b.Until = until.Time
		b.Banned = until.Time.After(time.Now())
	}
	return b
}

// banUntil encodes a ban expiry for storage: a zero time means permanent
// ('infinity'), any other time a temporary ban.
func banUntil(until time.Time) pgtype.Timestamptz {
	if until.IsZero() {
		return pgtype.Timestamptz{InfinityModifier: pgtype.Infinity, Valid: true}
	}
	return pgtype.Timestamptz{Time: until, Valid: true}
}

// ModAction is one entry of the audit log, with the participants' usernames
// already resolved (an id is useless to a human reading the feed).
type ModAction struct {
	ID        int64
	CreatedAt time.Time
	Actor     string
	Target    string // empty for a site-level action
	Action    string
	Detail    map[string]any
	Reason    string
}

// LogModAction appends one entry to the audit log. targetID is nil for a
// site-level action (a settings change). detail carries the action's
// before/after payload and may be nil.
func LogModAction(actorID int64, targetID *int64, action string,
	detail map[string]any, reason string) error {
	raw := []byte("{}")
	if len(detail) > 0 {
		encoded, err := json.Marshal(detail)
		if err != nil {
			return err
		}
		raw = encoded
	}
	ctx, cancel := Ctx()
	defer cancel()
	_, err := gen.New(Pool).InsertModAction(ctx, gen.InsertModActionParams{
		ActorUserID:  actorID,
		TargetUserID: targetID,
		Action:       action,
		Detail:       raw,
		Reason:       reason,
	})
	return err
}

// ModActionFilter narrows the audit feed. Both fields are optional: an empty
// Action means every verb, an empty Query means no text filter.
type ModActionFilter struct {
	// Action is one verb (ban/unban/title/role/rename/setting).
	Action string
	// Query matches the reason or either party's username. One box rather than
	// separate actor/target fields: "everything involving this account" is the
	// question a moderator asks, and splitting it into two inputs makes the
	// common case take two tries.
	Query string
}

// narg maps an empty filter string to the NULL the query treats as "no filter".
func narg(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ListModActions returns a page of the global audit feed, newest first.
func ListModActions(f ModActionFilter, limit, offset int32) ([]ModAction, error) {
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListModActions(ctx, gen.ListModActionsParams{
		Limit:  limit,
		Offset: offset,
		Action: narg(f.Action),
		Q:      narg(f.Query),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ModAction, 0, len(rows))
	for _, r := range rows {
		out = append(out, ModAction{
			ID:        r.ID,
			CreatedAt: r.CreatedAt.Time,
			Actor:     r.ActorUsername,
			Target:    strOrEmpty(r.TargetUsername),
			Action:    r.Action,
			Detail:    decodeDetail(r.Detail),
			Reason:    r.Reason,
		})
	}
	return out, nil
}

// CountModActions returns how many entries match the filter, for the pager.
func CountModActions(f ModActionFilter) (int64, error) {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).CountModActions(ctx, gen.CountModActionsParams{
		Action: narg(f.Action),
		Q:      narg(f.Query),
	})
}

// ListModActionsForUser returns one account's moderation history, newest
// first — the "prior actions" block on their player page (moderators only).
func ListModActionsForUser(userID int64, limit, offset int32) ([]ModAction, error) {
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListModActionsForUser(ctx,
		gen.ListModActionsForUserParams{
			TargetUserID: &userID, Limit: limit, Offset: offset,
		})
	if err != nil {
		return nil, err
	}
	out := make([]ModAction, 0, len(rows))
	for _, r := range rows {
		out = append(out, ModAction{
			ID:        r.ID,
			CreatedAt: r.CreatedAt.Time,
			Actor:     r.ActorUsername,
			Action:    r.Action,
			Detail:    decodeDetail(r.Detail),
			Reason:    r.Reason,
		})
	}
	return out, nil
}

// decodeDetail unpacks a mod_actions.detail payload, tolerating a malformed
// one (the log is for humans; a bad blob should not break the feed).
func decodeDetail(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// SetUserRole appoints or demotes an account. The caller enforces the
// authorization rules (admin-only, no self-demotion, never the last admin).
func SetUserRole(userID int64, r role.Role) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).SetUserRole(ctx, gen.SetUserRoleParams{
		ID: userID, Role: r.String(),
	})
}

// BanUser records the sanction on the account. A zero until is a permanent
// ban. This is only the record: the caller must also revoke the account's
// sessions, drop its cached sessions, and forfeit any live game — see
// arch/ADMIN_MODERATION.md for the required order.
func BanUser(userID int64, until time.Time, reason string) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).SetUserBan(ctx, gen.SetUserBanParams{
		ID:          userID,
		BannedUntil: banUntil(until),
		BanReason:   &reason,
	})
}

// UnbanUser lifts a ban early. An expired ban needs no call.
func UnbanUser(userID int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).ClearUserBan(ctx, userID)
}

// SetUserTitle assigns (or clears, with a nil titleID) an account's display
// title by titles row id.
func SetUserTitle(userID int64, titleID *int16) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).SetUserTitle(ctx, gen.SetUserTitleParams{
		ID: userID, TitleID: titleID,
	})
}

// ForceRename applies a moderator's rename. It maps the case-insensitive
// username collision to ErrUsernameTaken exactly as registration does, and
// deliberately leaves username_changed_at alone: the player's own one-time
// rename allowance is theirs, and a sanction must not spend it.
func ForceRename(userID int64, username string) error {
	ctx, cancel := Ctx()
	defer cancel()
	err := gen.New(Pool).ForceRenameUser(ctx, gen.ForceRenameUserParams{
		ID: userID, Username: username,
	})
	if isUniqueViolation(err) {
		return ErrUsernameTaken
	}
	return err
}

// CountModActionsForUser reports how many entries an account has on record, so
// a truncated list can say whether it is showing everything.
func CountModActionsForUser(userID int64) (int64, error) {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).CountModActionsForUser(ctx, &userID)
}

// CountAdmins reports how many admin accounts exist, guarding the rule that
// the last admin cannot be demoted.
func CountAdmins() (int64, error) {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).CountAdmins(ctx)
}

// AdminGrantor returns the account that last promoted userID to admin, and
// whether such a promotion is on record at all.
//
// It reads the audit log rather than a dedicated column because the log is
// already the authority on who did what — a second copy of that fact could
// disagree with it. ok=false means the account's admin role predates the log
// or was set outside the app (the SQL bootstrap), and no one may take it away
// through the UI: the founding admin is deliberately only demotable the same
// way it was created.
func AdminGrantor(userID int64) (grantor int64, ok bool, err error) {
	ctx, cancel := Ctx()
	defer cancel()
	id, err := gen.New(Pool).AdminGrantor(ctx, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// TitleOption is one row of the mod bar's title picker.
type TitleOption struct {
	ID   int16
	Code string
	Name string
}

// ListTitles returns every assignable title, ordered by code.
func ListTitles() ([]TitleOption, error) {
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListTitles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]TitleOption, 0, len(rows))
	for _, r := range rows {
		out = append(out, TitleOption{ID: r.ID, Code: r.Code, Name: r.Name})
	}
	return out, nil
}

// errUniqueViolation is Postgres' unique_violation SQLSTATE.
const errUniqueViolation = "23505"

// isUniqueViolation reports whether err is a unique-index violation, the
// signal both registration and forced rename map to ErrUsernameTaken.
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == errUniqueViolation
}

// strOrEmpty dereferences a nullable text column into a plain string ("" for
// NULL), the shape the auth/view layers want.
func strOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
