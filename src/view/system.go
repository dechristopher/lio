package view

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/settings"
)

// The /mod console (arch/ADMIN_MODERATION.md): everything that is not about one
// specific player. Per-account moderation deliberately lives on the player page
// instead, so a moderator acts on the page they were already reading.
//
// Phase 3 fills in the site controls and the audit feed; the reports queue and
// live ops view land in later phases.

// SystemModel is the console's page state.
type SystemModel struct {
	// IsAdmin gates the site controls: they affect every visitor at once, a
	// different order of blast radius from a per-account sanction, so they sit
	// above the moderator ladder.
	IsAdmin bool
	// Settings is the live snapshot, priming the controls form.
	Settings settings.Snapshot
	// Active is everything currently overriding a default, newest concern
	// first. Empty on a site running as shipped.
	Active []ActiveNotice
	// Feed is the global audit feed: a filtered, paginated window onto the
	// permanent record. Every moderator sees every other moderator's actions.
	Feed AuditFeed
	// Live is what the site is doing right now.
	Live LiveOps
}

// LiveOps is the operational picture: who is here and what is running. It is an
// assembly of counters the site already keeps for the home page rather than new
// instrumentation — the point is to put them where an operator looks when
// something seems wrong, next to the switches that can act on it.
//
// Everything here is process-local. On a single instance that is the whole
// site; if lio ever runs more than one, this page describes the instance that
// served it, and that limitation should be stated on the page rather than
// quietly misreported.
type LiveOps struct {
	Online         int
	LiveGames      int
	OpenChallenges int
	Rooms          []LiveRoomView
	// Truncated reports rooms omitted from the list, so a busy site does not
	// silently look like a short one.
	Truncated int
}

// LiveRoomView is one running room.
type LiveRoomView struct {
	RoomID  string
	URL     string
	Variant string
	Moves   string
	VsBot   bool
	// Kind distinguishes a game in progress from a challenge still waiting.
	Kind string
}

// AuditFeed is one rendered page of the audit log, with the state needed to
// page and re-filter it.
type AuditFeed struct {
	Actions []ModFeedEntry
	// Query / Action are the live filter values, echoed back into the form so
	// paging does not silently drop them.
	Query  string
	Action string
	// Actions available in the verb dropdown.
	ActionKinds []string
	// Page is 1-based; Pages is the total (at least 1, even when empty, so the
	// pager never reads "page 1 of 0").
	Page  int
	Pages int
	Total int64
	// PrevURL / NextURL are empty at the ends of the range, which is what the
	// template keys off to disable each control.
	PrevURL string
	NextURL string
	// Filtered reports whether any filter is active, so the empty state can
	// distinguish "nothing has happened" from "nothing matches".
	Filtered bool
}

// ModFeedEntry is one row of the global audit feed. Unlike the per-player
// history on a profile it names both parties, since the feed spans accounts and
// includes site-level changes with no target at all.
//
// Every field is rendered as its own tinted, tooltipped element rather than run
// together into a sentence: an audit row is scanned, not read, and a reader
// hunting one entry among hundreds needs the verb and the parties to separate
// at a glance.
type ModFeedEntry struct {
	// When is the relative time ("2 days ago"); WhenExact the absolute
	// timestamp, shown on hover because "2 days ago" is useless for
	// reconstructing a sequence of events.
	When      string
	WhenExact string
	Actor     string
	Action    string
	Target    string // empty for a site-level action (a settings change)
	Reason    string
	// Details are the action's payload, one key/value per chip.
	Details []DetailChip
}

// DetailChip is one key/value of an action's payload, with the explanation of
// what the key means.
type DetailChip struct {
	Key   string
	Value string
	Help  string
}

// ActionClass tints an action chip by what the verb does: sanctions read as
// losses, reversals as wins, and the rest as neutral changes. The colours are
// the site's existing semantic tokens, so a ban reads the same red here as a
// lost game does on a clock.
func ActionClass(action string) string {
	switch action {
	case "ban":
		return "act-ban"
	case "unban":
		return "act-unban"
	case "role":
		return "act-role"
	case "setting":
		return "act-setting"
	default:
		// title, rename, and anything added later
		return "act-edit"
	}
}

// ActionHelp explains a verb, for the chip's tooltip.
func ActionHelp(action string) string {
	switch action {
	case "ban":
		return "Account sanctioned: signed out everywhere and barred from logging in"
	case "unban":
		return "Sanction lifted early"
	case "title":
		return "Display title assigned or cleared"
	case "role":
		return "Permission level changed"
	case "rename":
		return "Username changed by a moderator"
	case "setting":
		return "Site-wide control changed"
	}
	return "Moderation action"
}

// detailHelp explains one payload key. Keys are shared across actions where
// they mean the same thing (from/to), so this is keyed by name rather than by
// action.
func detailHelp(key string) string {
	switch key {
	case "from":
		return "Value before this change"
	case "to":
		return "Value after this change"
	case "permanent":
		return "Whether the ban has no expiry"
	case "until":
		return "When the ban lifts (UTC)"
	case "duration":
		return "Ban length as chosen"
	case "forfeited":
		return "Live games this ban ended as a forfeit"
	case "lifted":
		return "The sanction that was lifted"
	case "banReason":
		return "Reason recorded on the sanction being lifted"
	case "notice":
		return "Site banner text after this change"
	case "noticeWas":
		return "Site banner text before this change"
	case "noticeLevel":
		return "Banner styling: info or warning"
	case "maintenance":
		return "New games blocked from starting"
	case "maintenanceWas":
		return "Maintenance mode before this change"
	case "registrationOpen":
		return "Whether new accounts can be created"
	case "registrationOpenWas":
		return "Registration before this change"
	case "ratedEnabled":
		return "Whether new games count toward ratings"
	case "ratedEnabledWas":
		return "Ratings before this change"
	}
	return "Recorded with this action"
}

// DetailChipsOf turns an action's JSON payload into rendered chips, each with
// the explanation of what its key means.
//
// Keys are sorted so the same action always reads the same way — a feed whose
// rows reshuffle their own fields is one nobody scans reliably — except that
// from/to lead, in that order, because a before/after pair is the whole story
// of most entries and reads backwards any other way.
func DetailChipsOf(detail map[string]any) []DetailChip {
	if len(detail) == 0 {
		return nil
	}
	keys := make([]string, 0, len(detail))
	for k := range detail {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := detailRank(keys[i]), detailRank(keys[j])
		if ri != rj {
			return ri < rj
		}
		return keys[i] < keys[j]
	})

	out := make([]DetailChip, 0, len(keys))
	for _, k := range keys {
		out = append(out, DetailChip{
			Key:   k,
			Value: formatDetailValue(detail[k]),
			Help:  detailHelp(k),
		})
	}
	return out
}

// detailRank floats the before/after pair to the front of a row.
func detailRank(key string) int {
	switch key {
	case "from":
		return 0
	case "to":
		return 1
	}
	return 2
}

// formatDetailValue renders one JSON value. Everything arrives as the types
// encoding/json produces, so numbers are float64 and are printed without a
// spurious decimal tail.
func formatDetailValue(v any) string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return "—"
		}
		return t
	case bool:
		if t {
			return "yes"
		}
		return "no"
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return "—"
	default:
		return "?"
	}
}

// ActiveNotice is one thing currently overriding a site default, with the
// setting change that stands it down.
type ActiveNotice struct {
	Title  string
	Detail string
	// Class tints the row by severity: a paused site reads differently from a
	// banner someone left up.
	Class string
	// Setting / ClearValue are exactly what the corresponding toggle would
	// post, so standing down is the same audited operation by another route —
	// not a second, quieter way to change the site.
	Setting    string
	ClearValue string
	// ClearEffect is what standing it down does, shown in the confirmation.
	ClearEffect string
}

// ActiveNoticesOf derives the active-notices list from a settings snapshot.
// Ordered by how much a visitor is affected: a site that will not start games
// first, then one that will not take signups, then ratings, then the banner.
func ActiveNoticesOf(s settings.Snapshot) []ActiveNotice {
	var out []ActiveNotice
	if s.Maintenance {
		out = append(out, ActiveNotice{
			Title:       "Maintenance mode",
			Detail:      "New games cannot be created or joined.",
			Class:       "active-notice-warn",
			Setting:     "maintenance",
			ClearValue:  "0",
			ClearEffect: SettingEffect("maintenance", false),
		})
	}
	if !s.RegistrationOpen {
		out = append(out, ActiveNotice{
			Title:       "Registration closed",
			Detail:      "New sign-ups are refused; existing accounts still log in.",
			Class:       "active-notice-warn",
			Setting:     "registrationOpen",
			ClearValue:  "1",
			ClearEffect: SettingEffect("registrationOpen", true),
		})
	}
	if !s.RatedEnabled {
		out = append(out, ActiveNotice{
			Title:       "Rated games paused",
			Detail:      "New games are created casual.",
			Class:       "active-notice-warn",
			Setting:     "ratedEnabled",
			ClearValue:  "1",
			ClearEffect: SettingEffect("ratedEnabled", true),
		})
	}
	if s.NoticeText != "" {
		out = append(out, ActiveNotice{
			Title:       "Site notice",
			Detail:      s.NoticeText,
			Class:       "active-notice-info",
			Setting:     "notice",
			ClearValue:  "",
			ClearEffect: "The banner disappears from every page.",
		})
	}
	return out
}

// SettingEffect states what flipping one switch will actually do, in the
// direction it is being flipped. It is the sentence the confirmation modal
// shows, and it names the consequence rather than the setting — an operator
// mid-incident needs to read "new games stop starting", not "maintenance
// becomes true".
func SettingEffect(key string, turningOn bool) string {
	switch key {
	case "maintenance":
		if turningOn {
			return "New games stop being created or joined. Games already in progress play out normally."
		}
		return "New games can be created and joined again."
	case "registrationOpen":
		if turningOn {
			return "Visitors can create accounts again."
		}
		return "New sign-ups are refused. Existing accounts keep signing in."
	case "ratedEnabled":
		if turningOn {
			return "New games count toward ratings again."
		}
		return "New games are created unrated. Games in progress keep the rating they started with."
	}
	return ""
}

// ModActionKinds are the verbs the audit filter offers, in the order they are
// shown. Kept here rather than derived from the data so the dropdown is stable
// — a filter whose options appear only once something has been logged is a
// filter nobody discovers.
var ModActionKinds = []string{"ban", "unban", "title", "role", "rename", "setting"}

// AuditPageSize is how many entries one page of the feed shows.
const AuditPageSize = 50

// auditCountLabel summarizes what the pager is paging through.
func auditCountLabel(f AuditFeed) string {
	switch {
	case f.Total == 1:
		return "1 entry"
	case f.Filtered:
		return strconv.FormatInt(f.Total, 10) + " matching entries"
	default:
		return strconv.FormatInt(f.Total, 10) + " entries on record"
	}
}

// AuditURL builds a /system link for one page of the feed, preserving the
// active filters. Page 1 with no filters is the bare path, so the common case
// does not carry a query string around.
func AuditURL(f ModActionQuery) string {
	v := url.Values{}
	if f.Query != "" {
		v.Set("q", f.Query)
	}
	if f.Action != "" {
		v.Set("action", f.Action)
	}
	if f.Page > 1 {
		v.Set("page", strconv.Itoa(f.Page))
	}
	if len(v) == 0 {
		return "/system"
	}
	return "/system?" + v.Encode()
}

// fragmentURL turns a /system page link into its htmx fragment equivalent, so
// the pager's href (a real, shareable page URL) and its hx-get (the swappable
// fragment) stay derived from one construction rather than two.
func fragmentURL(pageURL string) string {
	return strings.Replace(pageURL, "/system", "/system/actions", 1)
}

// ModActionQuery is the parsed audit-feed request: the filters plus the
// 1-based page. Shared by the handler and AuditURL so a link and the query it
// produces cannot drift.
type ModActionQuery struct {
	Query  string
	Action string
	Page   int
}

// Filtered reports whether any narrowing is active.
func (q ModActionQuery) Filtered() bool {
	return q.Query != "" || q.Action != ""
}

// NormalizeAction drops an action filter that is not one of the known verbs,
// so a hand-typed query string cannot produce a silently empty feed.
func NormalizeAction(action string) string {
	for _, kind := range ModActionKinds {
		if action == kind {
			return action
		}
	}
	return ""
}

// noticeLevelClass maps a notice level to its banner styling class.
func noticeLevelClass(level string) string {
	if level == settings.LevelWarn {
		return "site-notice-warn"
	}
	return "site-notice-info"
}

// SystemMeta builds page metadata for the console. Deliberately noindex-ish in
// spirit — there is nothing here for a search engine, and the page 404s for
// anyone without the role anyway.
func SystemMeta() Meta {
	return Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       "System • " + config.SiteName(),
		OGURL:       config.SiteOrigin(),
		OGTitle:     config.SiteName(),
		OGImage:     config.SiteOrigin() + "/og/default.png",
		Description: "Site administration.",
	}
}
