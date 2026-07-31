package handlers

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/sync/errgroup"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/pools"
	"github.com/dechristopher/lio/room"
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

// opponentsShown bounds the most-played list. Enough to show who someone
// actually plays, short of becoming a directory.
const opponentsShown = 8

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
		RenderedAt: strconv.FormatInt(time.Now().UnixMilli(), 10),
		UserID:     rec.ID,
		Username:   rec.Username,
		Title:      rec.Title,
		Joined:     view.JoinedMonth(rec.CreatedAt),
		Closed:     rec.Ban.Banned,
		// whether they could take a challenge right now; the button is hidden
		// rather than shown-and-refused
		Busy: room.AccountBusy(rec.ID),
	}

	// A closed account publishes nothing: no ratings, no record, no games. The
	// data is all still there (the archive keeps their games, and their name
	// still renders on an opponent's history) — it is this page that stops
	// making a claim about them.
	if !m.Closed {
		fillProfileStats(&m, rec.ID)
		fillViewerH2H(c, &m, rec.ID)
		// after fillProfileStats: the counts are loaded there, and this adds the
		// control that sits beside them
		fillFollowControl(c, &m, rec.ID)
	}

	fillModBar(c, &m, rec)
	// after fillModBar: the report control stands down when the viewer already
	// has the moderation bar on this account
	fillReportControl(c, &m, rec.ID)

	return view.Render(c, fiber.StatusOK, view.Profile(view.ProfileMeta(m), m))
}

// statReadLimit bounds how many of the profile's reads are in flight at once.
// pgxpool defaults to max(4, NumCPU) connections, so an unbounded fan-out could
// have one page render hold the whole pool while other requests wait on it.
const statReadLimit = 4

// fillProfileStats loads the public statistics block. Each read degrades to
// empty on error rather than failing the page: a profile missing its bot record
// is worth more to a visitor than a 500.
//
// The reads are independent and all scan the same indexed row set, so they run
// concurrently — the page costs the slowest one rather than their sum. errgroup
// is here for SetLimit, not for error propagation: every leg swallows its own
// error to keep the degrade-quietly contract, so the group never fails. Each
// leg writes to a distinct field of m, which is what makes this safe without a
// mutex.
func fillProfileStats(m *view.ProfileModel, userID int64) {
	var g errgroup.Group
	g.SetLimit(statReadLimit)

	// Ratings and their history are one leg, not two: the curve needs the tiles
	// to exist first (each stored point is a rating carried *into* a game, so
	// only the live rating can close a series) and running them in parallel
	// would mean synchronising two writers onto the same model.
	g.Go(func() error {
		ratings, err := db.ListRatingsForUser(userID)
		if err != nil {
			return nil
		}
		for _, r := range ratings {
			m.Ratings = append(m.Ratings, view.NewRatingView(
				r.Category, r.Rating.Display(), r.Rating.Games))
		}
		view.SortRatings(m.Ratings)

		history, err := db.RatingHistoryForUser(userID)
		if err != nil {
			return nil // tiles without curves beat no ratings section at all
		}
		m.Charts = view.NewRatingCharts(history, m.Ratings, time.Now())
		for _, c := range m.Charts {
			if c.Ready {
				m.HasCharts = true
				break
			}
		}
		return nil
	})
	g.Go(func() error {
		if total, life, err := db.TotalsForUser(userID); err == nil {
			m.Total = view.NewRecordView(total)
			m.Lifetime = view.NewLifetimeView(total, life)
		}
		return nil
	})
	g.Go(func() error {
		if variants, err := db.VariantRecordsForUser(userID); err == nil {
			for _, v := range variants {
				m.Variants = append(m.Variants, view.VariantRecordView{
					Name: v.Name,
					// speed class, not the stored "deploy" group — see the note
					// in profileGameView
					Group:      pools.SpeedFor(v.Name, v.Group),
					RecordView: view.NewRecordView(v.Record),
				})
			}
		}
		return nil
	})
	g.Go(func() error {
		if bots, err := db.BotRecordsForUser(userID); err == nil {
			for _, b := range bots {
				m.Bots = append(m.Bots, view.BotRecordView{
					// an empty persona key is a pre-ladder game: resolve it to
					// the Queen exactly as the archive and clocks do
					Persona:    view.BotSeatLabel(b.Persona),
					Glyph:      view.BotSeatGlyph(b.Persona),
					RecordView: view.NewRecordView(b.Record),
					Bar:        view.NewWDLBar(b.Record),
				})
			}
		}
		return nil
	})
	// One game past the display limit is fetched but never rendered. It answers
	// exactly one question: does the oldest shown match continue past the window?
	// Without it the form strip would have to guess whether its first group is
	// complete, and a match score is not a thing to guess at.
	var olderRoomID string
	g.Go(func() error {
		games, err := db.ListGamesForUser(userID, recentGamesShown+1, 0)
		if err != nil {
			return nil
		}
		if len(games) > recentGamesShown {
			olderRoomID = games[recentGamesShown].RoomID
			games = games[:recentGamesShown]
		}
		for _, g := range games {
			m.Games = append(m.Games, profileGameView(g))
		}
		return nil
	})
	g.Go(func() error {
		if colors, err := db.ColorSplitForUser(userID); err == nil {
			m.Colors = view.NewColorSplits(colors)
		}
		return nil
	})
	g.Go(func() error {
		if endings, err := db.EndingsForUser(userID); err == nil {
			m.Endings = view.NewEndings(endings)
		}
		return nil
	})
	g.Go(func() error {
		if lengths, err := db.LengthsForUser(userID); err == nil {
			m.Lengths = view.NewLengths(lengths)
		}
		return nil
	})
	g.Go(func() error {
		if streaks, err := db.StreaksForUser(userID); err == nil {
			m.Streaks = view.NewStreakView(streaks)
		}
		return nil
	})
	g.Go(func() error {
		if forms, err := db.FormationsForUser(userID); err == nil {
			m.Formations = view.NewFormations(forms)
		}
		return nil
	})
	g.Go(func() error {
		if acts, err := db.ActivityForUser(userID); err == nil {
			m.Activity = view.NewActivityView(acts, time.Now())
		}
		return nil
	})
	g.Go(func() error {
		if opps, err := db.OpponentsForUser(userID, opponentsShown); err == nil {
			m.Opponents = view.NewOpponents(opps)
		}
		return nil
	})
	g.Go(func() error {
		if counts, err := db.FollowCountsForUser(userID); err == nil {
			m.Follow = view.NewFollowView(counts)
		}
		return nil
	})

	_ = g.Wait() // never non-nil; every leg degrades in place

	// The bot ladder is derived from the records loaded above, so it renders
	// every rung — including ones never played — rather than only what a query
	// happened to return.
	m.BotLadder = view.NewBotLadder(m.Bots)

	// The form strip is derived, not queried: it reuses the games list above, so
	// it can never disagree with the rows rendered under it. That makes it
	// ordering-dependent, hence after Wait rather than in a leg of its own.
	m.Form = view.NewFormGroups(m.Games, olderRoomID)
}

// profileGameView renders one archived game for the recent-games list.
func profileGameView(g db.ProfileGame) view.ProfileGameView {
	result, class := view.ResultClass(g.Score)
	opponent, glyph := g.OpponentName, ""
	switch {
	case g.OpponentIsBot:
		// the glyph rides separately so the template can wrap it — see
		// ProfileGameView.OppGlyph
		opponent = "BOT " + view.BotSeatLabel(g.BotPersona)
		glyph = view.BotSeatGlyph(g.BotPersona)
	case opponent == "":
		opponent = "Anonymous"
	}
	mode := "Casual"
	if g.Rated {
		mode = "Rated"
	}
	v := view.ProfileGameView{
		RoomID: g.RoomID,
		URL:    archiveURL(g),
		When:   view.RelativeDay(g.Start),
		// the speed class, never the stored group: every game here is deploy
		// (the default mode), so "deploy" labels nothing, while bullet/blitz/
		// rapid is the distinction a player reads (variant.SpeedGroup)
		Variant:  g.VariantName + " " + pools.SpeedFor(g.VariantName, g.VariantGroup),
		Reason:   g.Reason,
		Mode:     mode,
		Result:   result,
		Class:    class,
		Opponent: opponent,
		OppGlyph: glyph,
		OppTitle: g.OpponentTitle,
		Ending:   view.ReasonPhrase(g.Reason),
	}
	// ratings only mean something on a rated game
	if g.Rated {
		v.OppRating = g.OppRating
		v.Delta, v.DeltaClass = view.RatingDelta(g.Delta)
	}
	return v
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

// fillFollowControl adds the follow button and its current state, for a
// logged-in visitor looking at somebody else's account (arch/FOLLOWING.md).
//
// Only reached for an open account: a closed one publishes nothing, so the
// whole social block is absent along with the rest of the page's claims about
// it. The API refuses a follow of a banned account independently.
func fillFollowControl(c fiber.Ctx, m *view.ProfileModel, userID int64) {
	acct := user.GetAccount(c)
	if acct == nil || acct.ID == userID {
		return
	}
	m.Follow.Control = true
	m.Follow.IsFollowing = db.IsFollowing(acct.ID, userID)
}

// fillReportControl decides whether the page offers the report control: a
// logged-in visitor looking at someone else's open account.
//
// The profile is where a player ends up when they want to do something about an
// opponent but no longer have the game in front of them, so it is the surface
// that has to work without a game at all — the report simply carries no
// evidence link. A closed account is already sanctioned and a moderator holding
// the mod bar has direct tools, so neither is asked to file anything.
//
// Must run after fillModBar, which is what ShowMod is read from.
func fillReportControl(c fiber.Ctx, m *view.ProfileModel, userID int64) {
	acct := user.GetAccount(c)
	m.ShowReport = acct != nil && acct.ID != userID && !m.Closed && !m.ShowMod
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
				When:      view.RelativeDay(a.CreatedAt),
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
