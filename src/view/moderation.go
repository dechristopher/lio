package view

import (
	"strconv"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db"
)

// The /moderation queue (arch/ADMIN_MODERATION.md Phase 4): the people page, as
// opposed to /system's site-state page. It is where a moderator finds out there
// is something to look at; acting on what they find happens on the player page,
// which is one click away from every row.

// ModerationModel is the queue page's state.
type ModerationModel struct {
	Open   []ReportView
	Closed []ReportView
	// OpenCount is the whole queue, which may exceed the page shown.
	OpenCount int64
}

// ReportView is one report as rendered.
type ReportView struct {
	ID string
	// When the report was filed, and the exact stamp for the tooltip.
	When      string
	WhenExact string
	// Category is the reported behaviour; Class tints it the way audit verbs
	// are tinted, so the queue is scannable by kind.
	Category string
	Class    string
	Help     string
	Note     string
	Reporter string
	Target   string
	// GameURL links the evidence when the report named a game.
	GameURL string
	// Resolution fields, set only on closed reports.
	Resolved   string
	Resolver   string
	Resolution string
}

// ReportCategoryClass tints a report by what is being alleged. Cheating and
// sandbagging are attacks on the rating system and read as losses; stalling and
// username complaints are conduct and read as warnings; "other" stays neutral
// so it does not borrow urgency it has not earned.
func ReportCategoryClass(category string) string {
	switch category {
	case "cheating", "sandbagging":
		return "rep-severe"
	case "stalling", "username":
		return "rep-conduct"
	}
	return "rep-other"
}

// ReportCategoryHelp explains a category, for the chip's tooltip. The
// vocabulary is the site's, not the reporter's — a player picks from a list, so
// the queue needs to say what each choice was understood to mean.
func ReportCategoryHelp(category string) string {
	switch category {
	case "cheating":
		return "Suspected engine assistance"
	case "sandbagging":
		return "Deliberately losing to manipulate a rating"
	case "stalling":
		return "Running down the clock or refusing to play on"
	case "username":
		return "The account's name itself is the problem"
	case "other":
		return "Something not covered by the other reasons"
	}
	return "Reported behaviour"
}

// ReportCategoryLabel renders a category for a picker.
func ReportCategoryLabel(category string) string {
	switch category {
	case "cheating":
		return "Cheating — engine assistance"
	case "sandbagging":
		return "Sandbagging — losing on purpose"
	case "stalling":
		return "Stalling — wasting time"
	case "username":
		return "Username — the name itself"
	case "other":
		return "Something else"
	}
	return category
}

// ReportCategoriesForPicker is the ordered list the player-facing picker
// offers. It mirrors db.ReportCategories — the set the CHECK constraint
// accepts — rather than keeping a second list that can drift from it; the order
// is chosen here because "other" belongs last in a menu regardless of how the
// constraint lists it.
func ReportCategoriesForPicker() []string {
	return db.ReportCategories
}

// QueueLabel summarizes the queue's size for the page heading.
func QueueLabel(n int64) string {
	switch n {
	case 0:
		return "nothing waiting"
	case 1:
		return "1 report waiting"
	default:
		return strconv.FormatInt(n, 10) + " reports waiting"
	}
}

// ModerationMeta builds page metadata for the moderation queue.
func ModerationMeta() Meta {
	return Meta{
		Version:     config.VersionString(),
		SiteURL:     config.SiteURL(),
		Title:       "Moderation • " + config.SiteName(),
		OGURL:       config.SiteOrigin(),
		OGTitle:     config.SiteName(),
		OGImage:     config.SiteOrigin() + "/og/default.png",
		Description: "Moderation queue.",
	}
}
