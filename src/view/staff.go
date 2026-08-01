package view

import (
	"strconv"
	"time"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/title"
)

// The staff overview (arch/ADMIN_MODERATION.md, *Who holds power is not a
// secret*): who may moderate this site, grouped by what they may do.
//
// It renders on two surfaces from one model. The public page answers a
// player's question — who are the people with the tools, so a name in a
// decision means something. The /system panel answers staff's — who else holds
// power, who granted it, and when. The second is a superset of the first, so
// Detailed switches the extra fields on rather than a second model carrying
// them.

// StaffList is the site's staff, split by role.
//
// Two groups, not one list with a role column beside each name. What separates
// a moderator from an admin is what they may do, and that is the question
// somebody reading this page is asking; a column makes it a detail of a row
// they have to scan for.
type StaffList struct {
	Admins []StaffView
	Mods   []StaffView
	// Detailed is the staff-facing render: the appointment trail and the
	// sanction marker. Both are accountability among staff rather than
	// information a player can use, and the audit log they come from is not
	// public either.
	Detailed bool
}

// Empty reports whether the site has no staff at all — a fresh install before
// the first admin is bootstrapped, and the only state where the page has
// nothing to say.
func (s StaffList) Empty() bool { return len(s.Admins) == 0 && len(s.Mods) == 0 }

// Total is how many people hold a role above player.
func (s StaffList) Total() int { return len(s.Admins) + len(s.Mods) }

// StaffView is one staff account as a page renders it.
type StaffView struct {
	Username string
	Title    title.Title
	// Joined is when the account registered, relative ("3 months ago"), with
	// JoinedExact the absolute timestamp on hover. It is deliberately not the
	// appointment date: a long-standing player who became a moderator last week
	// is a different thing from an account created last week, and the public
	// page shows the one anybody can already see on the profile.
	Joined      string
	JoinedExact string
	// Granted describes the appointment, and is only ever populated on the
	// detailed render. Empty for a role set outside the app — the bootstrapped
	// first admin — which is stated rather than left blank, because a blank
	// reads as missing data.
	GrantedBy    string
	Granted      string
	GrantedExact string
	// Sanctioned marks a staff account that is itself banned. It should never be
	// true; it renders because the only thing worse than it happening is it
	// happening unnoticed.
	Sanctioned bool
}

// Bootstrapped reports an account whose role has no grantor on record. Only
// meaningful on the detailed render.
func (s StaffView) Bootstrapped() bool { return s.GrantedBy == "" }

// StaffListOf splits the staff into its two groups and renders the timestamps.
//
// detailed decides whether the appointment trail is carried at all, rather than
// carrying it and hiding it in the template. A field that is present in the
// model of a public page is one render mistake away from being on the page.
func StaffListOf(members []db.StaffMember, detailed bool) StaffList {
	out := StaffList{Detailed: detailed}
	for _, m := range members {
		v := StaffView{
			Username:    m.Username,
			Title:       title.Title{Code: m.TitleCode, Name: m.TitleName},
			Joined:      RelativeDay(m.Joined),
			JoinedExact: exactTime(m.Joined),
		}
		if detailed {
			v.GrantedBy = m.GrantedBy
			v.Granted = RelativeDay(m.GrantedAt)
			v.GrantedExact = exactTime(m.GrantedAt)
			v.Sanctioned = m.Sanctioned
		}
		if m.Role == role.Admin {
			out.Admins = append(out.Admins, v)
		} else {
			out.Mods = append(out.Mods, v)
		}
	}
	return out
}

// exactTime is the absolute timestamp shown on hover beside every relative one,
// because "3 months ago" cannot be used to reconstruct a sequence of events.
func exactTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05 MST")
}

// StaffCountLabel summarizes the list for its heading.
func StaffCountLabel(s StaffList) string {
	if s.Total() == 1 {
		return "1 person"
	}
	return strconv.Itoa(s.Total()) + " people"
}

// StaffMeta builds page metadata for the public staff page. It is a real page
// people are meant to find — a player looking up who moderates the site should
// reach it from a search as well as from the footer — so it carries the same
// social card every other public page does.
func StaffMeta() Meta {
	return Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       "Staff • " + config.SiteName(),
		OGURL:       config.SiteOrigin() + "/staff",
		OGTitle:     "Staff • " + config.SiteName(),
		OGImage:     config.SiteOrigin() + "/og/default.png",
		Description: "The people who run and moderate " + config.SiteName() + ".",
	}
}
