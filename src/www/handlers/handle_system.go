package handlers

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/cache"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/presence"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/settings"
	"github.com/dechristopher/lio/store"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/sysinfo"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/view"
)

// The /system console page (arch/ADMIN_MODERATION.md Phase 3) and its sibling
// /moderation queue.
//
// A visitor without the role gets the ordinary 404, not a 403: a privilege
// boundary should not be an oracle for its own existence, and the same rule the
// /api/mod routes follow.

// queueShown bounds the open-report queue; resolvedShown the recent decisions
// beneath it. The queue is worked down rather than paged — if it ever needs a
// pager, that is a signal about volume worth noticing rather than papering over.
const (
	queueShown    = 50
	resolvedShown = 20
	// liveRoomsShown bounds the live list. A busy site is a good problem, but a
	// page listing every room becomes unreadable exactly when it matters most;
	// the remainder is counted rather than dropped silently.
	liveRoomsShown = 40
	// feedbackShown bounds the feedback inbox. Unlike the report queue it is not
	// worked to empty — read messages stay as history — so the section shows a
	// recent window and counts the rest.
	feedbackShown = 30
	// broadcastsShown bounds the sent-broadcast list. Recent history, not an
	// archive: what an operator needs to see is what is live and what just
	// ended, and the audit log holds the permanent record of every one.
	broadcastsShown = 20
)

// The console's three pages (arch/ADMIN_MODERATION.md, *The console is three
// pages*). Each is a real route so that a tab is a URL and — the reason that
// shows up in the logs — each page runs only its own reads. The console's loads
// are not cheap: the overview probes three backends, and a moderator reading
// the audit log should pay for none of it.

// SystemHandler renders the overview: the live picture, what is currently
// overriding a default, the switches that set them, and the process itself.
//
// Moderator-gated, with the admin cards unrendered inside. The live picture and
// the controls are one page because they answer the same question during an
// incident; the privilege boundary is not the page in any case, since every
// /api/mod route re-checks the caller's role independently.
func SystemHandler(c fiber.Ctx) error {
	acct, m, ok := systemPage(c, view.TabOverview)
	if !ok {
		return systemNotFound(c)
	}
	m.Live = liveOps()
	// Everything below is admin-only on the page, so a moderator's load does no
	// I/O it would then throw away — including the instance panel's three
	// backend probes.
	if acct.Role.CanAdmin() {
		current := settings.Current()
		m.Settings = current
		m.Active = view.ActiveNoticesOf(current)
		m.Broadcasts = sentBroadcasts()
		m.Stats = instanceStats()
	}
	return view.Render(c, fiber.StatusOK, view.System(view.SystemMeta(), m))
}

// SystemPeopleHandler renders the community page: the staff overview, the
// feedback inbox and the message composer.
func SystemPeopleHandler(c fiber.Ctx) error {
	_, m, ok := systemPage(c, view.TabPeople)
	if !ok {
		return systemNotFound(c)
	}
	m.Feedback = feedbackInbox()
	// Detailed: this is the staff-facing render, so it carries the appointment
	// trail the public /staff page does not.
	m.Staff = staffList(true)
	return view.Render(c, fiber.StatusOK, view.System(view.SystemMeta(), m))
}

// SystemLogHandler renders the audit feed, which has the page to itself.
func SystemLogHandler(c fiber.Ctx) error {
	_, m, ok := systemPage(c, view.TabLog)
	if !ok {
		return systemNotFound(c)
	}
	m.Feed = auditFeed(c)
	return view.Render(c, fiber.StatusOK, view.System(view.SystemMeta(), m))
}

// systemPage resolves the caller and starts the model for one tab. It answers
// ok=false for anyone who may not moderate at all; a page with a stricter gate
// applies it on top.
func systemPage(c fiber.Ctx, tab view.SystemTab) (*user.Account, view.SystemModel, bool) {
	acct := user.GetAccount(c)
	if acct == nil || !acct.Role.CanModerate() {
		return nil, view.SystemModel{}, false
	}
	return acct, view.SystemModel{Tab: tab, IsAdmin: acct.Role.CanAdmin()}, true
}

// systemNotFound is the console's refusal. A visitor without the role gets the
// ordinary 404, not a 403: a privilege boundary should not be an oracle for its
// own existence.
func systemNotFound(c fiber.Ctx) error {
	return view.Render(c, fiber.StatusNotFound, view.NotFound(view.PageMeta("404")))
}

// StaffHandler renders the public staff page: who runs the site and who
// moderates it.
//
// Open to everybody, unlike everything else in this file. Moderation here is
// not anonymous — the audit log says so among staff, and this says the same
// thing outward, so a player who has been sanctioned knows whose tools they
// were. It carries no appointment trail and no sanction marker: those come from
// the audit log, which is staff-only.
func StaffHandler(c fiber.Ctx) error {
	return view.Render(c, fiber.StatusOK, view.Staff(view.StaffMeta(), staffList(false)))
}

// staffList loads the staff overview. A read failure renders an empty list
// rather than failing the page: on /system the switches above it are the reason
// somebody opened the console, and on the public page the rest of the page
// still says what the site is.
func staffList(detailed bool) view.StaffList {
	members, err := db.Staff()
	if err != nil {
		util.Error(str.CDB, "staff list failed error=%s", err.Error())
		return view.StaffList{Detailed: detailed}
	}
	return view.StaffListOf(members, detailed)
}

// sentBroadcasts loads the console's broadcast history with each message's
// answers folded in (arch/NOTIFICATIONS.md). Degrades to an empty list on a
// read failure, like every other section of the page.
func sentBroadcasts() []view.BroadcastView {
	rows, err := db.ListBroadcasts(broadcastsShown)
	if err != nil {
		util.Error(str.CDB, "broadcast list failed error=%s", err.Error())
		return nil
	}
	out := make([]view.BroadcastView, 0, len(rows))
	for _, b := range rows {
		out = append(out, view.BroadcastViewOf(b))
	}
	return out
}

// SystemStatsHandler serves the instance panel on its own, for its self-poll.
// Admin-gated like the panel it refreshes: the fragment is a route in its own
// right and re-checks the role rather than trusting that its only caller is the
// page that already did.
func SystemStatsHandler(c fiber.Ctx) error {
	acct := user.GetAccount(c)
	if acct == nil || !acct.Role.CanAdmin() {
		return systemNotFound(c)
	}
	// a sample is stale the instant it is taken; a cached copy would show an
	// operator a process state that no longer holds
	c.Set("Cache-Control", "no-store")
	return view.Render(c, fiber.StatusOK, view.SystemStatsBody(instanceStats()))
}

// instanceStats samples the process and each backing service. Every probe has
// its own short deadline and reports failure as a value rather than an error,
// so a backend being down renders as a red pill on an otherwise working page —
// which is precisely the moment this panel needs to render.
func instanceStats() view.SystemStats {
	return view.SystemStatsOf(sysinfo.Sample(), db.GetStats(), cache.GetStats(), store.GetStats())
}

// SystemActionsHandler serves the audit feed on its own, for the filter form
// and pager's htmx swaps. Same gate as the page: the fragment is a route in its
// own right, so it re-checks the role rather than trusting that its only caller
// is the page that already did.
func SystemActionsHandler(c fiber.Ctx) error {
	acct := user.GetAccount(c)
	if acct == nil || !acct.Role.CanModerate() {
		return systemNotFound(c)
	}
	// a filtered feed is live data; a cached copy would show a moderator a
	// state of the log that no longer holds
	c.Set("Cache-Control", "no-store")
	return view.Render(c, fiber.StatusOK, view.AuditFeedBody(auditFeed(c)))
}

// auditFeed reads the filter/page query params and builds one rendered page of
// the audit log. Shared by the full page and the fragment so both interpret a
// URL identically.
func auditFeed(c fiber.Ctx) view.AuditFeed {
	q := view.ModActionQuery{
		Query:  strings.TrimSpace(c.Query("q")),
		Action: view.NormalizeAction(c.Query("action")),
		Page:   1,
	}
	if n, err := strconv.Atoi(c.Query("page")); err == nil && n > 1 {
		q.Page = n
	}

	feed := view.AuditFeed{
		Query:       q.Query,
		Action:      q.Action,
		ActionKinds: view.ModActionKinds,
		Page:        q.Page,
		Pages:       1,
		Filtered:    q.Filtered(),
	}

	filter := db.ModActionFilter{Action: q.Action, Query: q.Query}
	total, err := db.CountModActions(filter)
	if err != nil {
		util.Error(str.CDB, "audit count failed error=%s", err.Error())
		return feed
	}
	feed.Total = total
	if total > 0 {
		feed.Pages = int((total + view.AuditPageSize - 1) / view.AuditPageSize)
	}
	// a page past the end (a stale link, or entries removed from under it)
	// clamps rather than showing an empty list with a pager insisting there is
	// more above it
	if feed.Page > feed.Pages {
		feed.Page = feed.Pages
		q.Page = feed.Pages
	}

	offset := int32((feed.Page - 1) * view.AuditPageSize)
	actions, err := db.ListModActions(filter, view.AuditPageSize, offset)
	if err != nil {
		util.Error(str.CDB, "audit list failed error=%s", err.Error())
		return feed
	}
	for _, a := range actions {
		feed.Actions = append(feed.Actions, view.ModFeedEntry{
			When:      view.RelativeDay(a.CreatedAt),
			WhenExact: a.CreatedAt.UTC().Format("2006-01-02 15:04:05 MST"),
			Actor:     a.Actor,
			Action:    a.Action,
			Target:    a.Target,
			Reason:    a.Reason,
			Details:   view.DetailChipsOf(a.Detail),
		})
	}

	if feed.Page > 1 {
		prev := q
		prev.Page = feed.Page - 1
		feed.PrevURL = view.AuditURL(prev)
	}
	if feed.Page < feed.Pages {
		next := q
		next.Page = feed.Page + 1
		feed.NextURL = view.AuditURL(next)
	}
	return feed
}

// ModerationHandler renders the report queue: what players have flagged, and
// what has recently been decided. The queue itself carries no sanction
// controls — every row links to the reported account's page, where the record
// is visible before anyone acts.
func ModerationHandler(c fiber.Ctx) error {
	acct := user.GetAccount(c)
	if acct == nil || !acct.Role.CanModerate() {
		return view.Render(c, fiber.StatusNotFound, view.NotFound(view.PageMeta("404")))
	}

	m := view.ModerationModel{}
	if total, err := db.CountOpenReports(); err == nil {
		m.OpenCount = total
	}
	if open, err := db.OpenReports(queueShown, 0); err == nil {
		for _, r := range open {
			m.Open = append(m.Open, reportView(r, false))
		}
	} else {
		util.Error(str.CDB, "report queue load failed error=%s", err.Error())
	}
	if closed, err := db.ClosedReports(resolvedShown); err == nil {
		for _, r := range closed {
			m.Closed = append(m.Closed, reportView(r, true))
		}
	}

	return view.Render(c, fiber.StatusOK, view.Moderation(view.ModerationMeta(), m))
}

// liveOps assembles the operational picture from what the site already tracks
// for the home page. No new instrumentation: HomeListing walks the room map
// under each room's own lock, and presence walks the socket directory.
func liveOps() view.LiveOps {
	live, challenges, stats, seated := room.HomeListing()
	ops := view.LiveOps{
		// the console wants the headcount only; no roster is rendered here, so
		// it asks for no named members and no ordering key.
		//
		// Live, not Total: the home page lists everybody active in the last
		// quarter of an hour, but an operator reading this row is asking how
		// many connections are open right now, and a window would blunt exactly
		// the signal they came for.
		Online:         presence.Online(seated, 0, nil).Live,
		LiveGames:      stats.LiveGames,
		OpenChallenges: stats.OpenChallenges,
	}

	for _, g := range live {
		if len(ops.Rooms) >= liveRoomsShown {
			ops.Truncated++
			continue
		}
		ops.Rooms = append(ops.Rooms, view.LiveRoomView{
			RoomID:  g.RoomID,
			URL:     "/" + g.RoomID,
			Variant: g.Variant.Name + " " + string(g.Variant.Group),
			Moves:   strconv.Itoa(g.Moves) + " moves",
			VsBot:   g.VsBot,
			Kind:    "playing",
		})
	}
	for _, ch := range challenges {
		if len(ops.Rooms) >= liveRoomsShown {
			ops.Truncated++
			continue
		}
		ops.Rooms = append(ops.Rooms, view.LiveRoomView{
			RoomID:  ch.RoomID,
			URL:     "/" + ch.RoomID,
			Variant: ch.Variant.Name + " " + string(ch.Variant.Group),
			Moves:   "waiting",
			Kind:    "challenge",
		})
	}
	return ops
}

// feedbackInbox loads the /system feedback section: a recent window of what
// players have sent, plus the counts that let the section say what it is not
// showing. A read failure degrades to an empty section rather than failing the
// page — the console's job during an incident is the switches above it, and
// those must still render when this query does not.
func feedbackInbox() view.FeedbackInbox {
	inbox := view.FeedbackInbox{Unread: db.UnreadFeedback()}
	if total, err := db.CountFeedback(); err == nil {
		inbox.Total = total
	}
	items, err := db.ListFeedback(feedbackShown, 0)
	if err != nil {
		util.Error(str.CDB, "feedback inbox load failed error=%s", err.Error())
		return inbox
	}
	for _, f := range items {
		inbox.Items = append(inbox.Items, view.FeedbackView{
			ID:        strconv.FormatInt(f.ID, 10),
			When:      view.RelativeDay(f.Created),
			WhenExact: f.Created.UTC().Format("2006-01-02 15:04:05 MST"),
			Kind:      f.Kind,
			Class:     view.FeedbackKindClass(f.Kind),
			Label:     view.FeedbackKindLabel(f.Kind),
			Body:      f.Body,
			Author:    f.Author,
			Path:      f.Path,
			Unread:    f.Unread(),
			Reader:    f.Reader,
		})
	}
	inbox.Shown = len(inbox.Items)
	return inbox
}

// reportView renders one report for the queue.
func reportView(r db.Report, resolved bool) view.ReportView {
	v := view.ReportView{
		ID:        strconv.FormatInt(r.ID, 10),
		When:      view.RelativeDay(r.Created),
		WhenExact: r.Created.UTC().Format("2006-01-02 15:04:05 MST"),
		Category:  r.Category,
		Class:     view.ReportCategoryClass(r.Category),
		Help:      view.ReportCategoryHelp(r.Category),
		Note:      r.Note,
		Reporter:  r.Reporter,
		Target:    r.Target,
	}
	if r.GameID != "" {
		v.GameURL = "/game/" + r.GameID
	}
	if resolved {
		v.Resolved = view.RelativeDay(r.Resolved)
		v.WhenExact = r.Resolved.UTC().Format("2006-01-02 15:04:05 MST")
		v.Resolver = r.Resolver
		v.Resolution = r.Resolution
	}
	return v
}
