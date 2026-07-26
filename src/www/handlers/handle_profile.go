package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/view"
)

// The public player page at /@/<username> (arch/ADMIN_MODERATION.md). It is the
// site's first surface tying a name to a history, and the host for the
// moderation bar — so a moderator never leaves the page they were already
// looking at to act on it.
//
// Note the route registration order in www.go: this must be wired before the
// /:id and /:id/:num room wildcards, or "@" is captured as a room id and the
// username as a game ordinal (the same trap /game/:uuid documents).

// recentGamesShown bounds the recent-games list. Deep history lives in the
// archive; this is a glance, not a paginated log.
const recentGamesShown = 20

// modHistoryShown bounds the per-account moderation history in the mod bar.
const modHistoryShown = 20

// ProfileHandler renders a player page by username (case-insensitive, like
// every other username lookup). Unknown names 404 through the normal page, so a
// typo looks like a missing page rather than an error.
func ProfileHandler(c fiber.Ctx) error {
	username := strings.TrimSpace(c.Params("username"))
	if username == "" || !auth.Enabled() {
		return view.Render(c, fiber.StatusNotFound, view.NotFound(view.PageMeta("404")))
	}

	rec, found, err := db.GetUserByUsername(username)
	if err != nil {
		return view.Render(c, fiber.StatusInternalServerError,
			view.NotFound(view.PageMeta("404")))
	}
	if !found {
		return view.Render(c, fiber.StatusNotFound, view.NotFound(view.PageMeta("404")))
	}

	m := view.ProfileModel{
		UserID:   rec.ID,
		Username: rec.Username,
		Title:    rec.Title,
		Joined:   view.JoinedMonth(rec.CreatedAt),
		Closed:   rec.Ban.Banned,
	}

	// A closed account publishes nothing: no ratings, no record, no games. The
	// data is all still there (the archive keeps their games, and their name
	// still renders on an opponent's history) — it is this page that stops
	// making a claim about them.
	if !m.Closed {
		fillProfileStats(&m, rec.ID)
		fillViewerH2H(c, &m, rec.ID)
	}

	fillModBar(c, &m, rec)

	return view.Render(c, fiber.StatusOK, view.Profile(view.ProfileMeta(m), m))
}

// fillProfileStats loads the public statistics block. Each read degrades to
// empty on error rather than failing the page: a profile missing its bot record
// is worth more to a visitor than a 500.
func fillProfileStats(m *view.ProfileModel, userID int64) {
	if ratings, err := db.ListRatingsForUser(userID); err == nil {
		for _, r := range ratings {
			m.Ratings = append(m.Ratings, view.RatingView{
				Category: r.Category,
				Rating:   r.Rating.Display(),
				Games:    r.Rating.Games,
			})
		}
	}
	if total, err := db.TotalsForUser(userID); err == nil {
		m.Total = view.NewRecordView(total)
	}
	if variants, err := db.VariantRecordsForUser(userID); err == nil {
		for _, v := range variants {
			m.Variants = append(m.Variants, view.VariantRecordView{
				Name:       v.Name,
				Group:      v.Group,
				RecordView: view.NewRecordView(v.Record),
			})
		}
	}
	if bots, err := db.BotRecordsForUser(userID); err == nil {
		for _, b := range bots {
			m.Bots = append(m.Bots, view.BotRecordView{
				// an empty persona key is a pre-ladder game: resolve it to the
				// Queen exactly as the archive and clocks do
				Persona:    view.BotSeatLabel(b.Persona),
				Glyph:      view.BotSeatGlyph(b.Persona),
				RecordView: view.NewRecordView(b.Record),
			})
		}
	}
	if games, err := db.ListGamesForUser(userID, recentGamesShown, 0); err == nil {
		for _, g := range games {
			m.Games = append(m.Games, profileGameView(g))
		}
	}
}

// profileGameView renders one archived game for the recent-games list.
func profileGameView(g db.ProfileGame) view.ProfileGameView {
	result, class := view.ResultClass(g.Score)
	opponent := g.OpponentName
	switch {
	case g.OpponentIsBot:
		opponent = "BOT " + view.BotSeatGlyph(g.BotPersona) + " " + view.BotSeatLabel(g.BotPersona)
	case opponent == "":
		opponent = "Anonymous"
	}
	mode := "Casual"
	if g.Rated {
		mode = "Rated"
	}
	return view.ProfileGameView{
		URL:      archiveURL(g),
		When:     relativeDay(g.Start),
		Variant:  g.VariantName + " " + g.VariantGroup,
		Mode:     mode,
		Result:   result,
		Class:    class,
		Opponent: opponent,
		OppTitle: g.OpponentTitle,
	}
}

// archiveURL links a game to its permalink: the canonical /<room>/<n> when the
// game belongs to a room, else the room-less /game/<uuid> form.
func archiveURL(g db.ProfileGame) string {
	if g.RoomID != "" && g.GameIndex > 0 {
		return "/" + g.RoomID + "/" + strconv.Itoa(int(g.GameIndex))
	}
	return "/game/" + g.GameID.String()
}

// fillViewerH2H adds the viewing account's own record against this one, shown
// only when they are logged in, are not looking at themselves, and have
// actually met.
func fillViewerH2H(c fiber.Ctx, m *view.ProfileModel, userID int64) {
	acct := user.GetAccount(c)
	if acct == nil || acct.ID == userID {
		return
	}
	viewerID, targetID := acct.ID, userID
	h2h := db.HeadToHead(&viewerID, &targetID)
	if h2h.Games == 0 {
		return
	}
	m.H2HShow = true
	m.H2H = view.FormatPoints(h2h.AScore) + " – " + view.FormatPoints(h2h.BScore)
}

// fillModBar populates the moderator-only controls, but only for a viewer who
// may actually act on this account. It mirrors the API's rules in
// www/handlers/api/mod (self-administration for admins, peer admins only for
// the granting admin) so the page never offers an action the server would
// refuse — and the API refuses it anyway if the page is wrong.
func fillModBar(c fiber.Ctx, m *view.ProfileModel, rec db.UserRecord) {
	acct := user.GetAccount(c)
	if acct == nil {
		return
	}
	self := acct.ID == rec.ID
	switch {
	case self:
		// an admin may administer their own account; anyone else gets no bar
		if !acct.Role.CanAdmin() {
			return
		}
	case !acct.Role.CanActOn(rec.Role):
		return
	case rec.Role.CanAdmin() && !adminGrantedBy(acct.ID, rec.ID):
		// a peer admin is off limits unless this viewer promoted them
		return
	}

	m.ShowMod = true
	m.Mod = view.ModBarView{
		IsSelf:   self,
		Username: rec.Username,
		// no self-demotion, and no ban on your own account
		CanSetRole:  acct.Role.CanAdmin() && !self,
		CanBan:      !self,
		CurrentRole: rec.Role.String(),
		Banned:      rec.Ban.Banned,
		BanReason:   rec.Ban.Reason,
		BanUntil:    banUntilLabel(rec.Ban),
		Roles:       view.AssignableRoles(),
	}
	if titles, err := db.ListTitles(); err == nil {
		for _, t := range titles {
			id := strconv.Itoa(int(t.ID))
			m.Mod.Titles = append(m.Mod.Titles, view.TitleOptionView{
				ID: id, Code: t.Code, Name: t.Name,
			})
			if t.Code == rec.Title.Code {
				m.Mod.CurrentTitleID = id
			}
		}
	}
	if actions, err := db.ListModActionsForUser(rec.ID, modHistoryShown, 0); err == nil {
		for _, a := range actions {
			m.Mod.Actions = append(m.Mod.Actions, view.ModFeedEntry{
				When:      relativeDay(a.CreatedAt),
				WhenExact: a.CreatedAt.UTC().Format("2006-01-02 15:04:05 MST"),
				Actor:     a.Actor,
				Action:    a.Action,
				Reason:    a.Reason,
				Details:   view.DetailChipsOf(a.Detail),
			})
		}
	}
	if total, err := db.CountModActionsForUser(rec.ID); err == nil {
		m.Mod.ActionsTotal = total
	}
	if reports, err := db.OpenReportsAgainst(rec.ID); err == nil {
		m.Mod.OpenReports = reports
	}
	// The full record lives on /system, filtered to this name — which also
	// surfaces actions this account *took*, not just ones taken against it.
	// That is the broader question a moderator is usually asking, and it avoids
	// a second paginator here that would drift from the real one.
	if len(m.Mod.Actions) > 0 {
		m.Mod.HistoryURL = view.AuditURL(view.ModActionQuery{Query: rec.Username, Page: 1})
	}
}

// adminGrantedBy reports whether viewerID is on record as having promoted
// targetID to admin. Mirrors the API's grantedBy; a lookup failure denies, so
// the control simply does not render.
func adminGrantedBy(viewerID, targetID int64) bool {
	grantor, ok, err := db.AdminGrantor(targetID)
	return err == nil && ok && grantor == viewerID
}

// banUntilLabel phrases a sanction's remaining term for the moderator view.
func banUntilLabel(b db.BanState) string {
	if !b.Banned {
		return ""
	}
	if b.Permanent {
		return "permanently"
	}
	return "until " + b.Until.Format("Jan 2, 2006")
}

// relativeDay renders a coarse "N days ago" for archived timestamps, falling
// back to a date past a week. Day-grained on purpose: an exact time adds
// nothing to a games list and publishes more than a public page should.
func relativeDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return strconv.Itoa(h) + " hours ago"
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return strconv.Itoa(days) + " days ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}
