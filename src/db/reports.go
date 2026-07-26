package db

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
)

// Player reports (arch/ADMIN_MODERATION.md Phase 4): the queue by which a
// moderator learns there is something to look at. Unlike the audit log, a report
// is a transient request for attention rather than a permanent record — it is
// filed, worked, and closed.

// ErrAlreadyReported is the reports_open_unique violation: this reporter
// already has an open report against this account. Surfaced as an ordinary
// answer rather than an error, because from the reporter's side "you have
// already told us" is the correct and complete response.
var ErrAlreadyReported = errors.New("already reported")

// ReportCategories are the accepted report kinds, matching the CHECK
// constraint. Exported so the handler validates against exactly the set the
// database will accept rather than a second list that can drift from it.
var ReportCategories = []string{"cheating", "sandbagging", "stalling", "username", "other"}

// ValidCategory reports whether c is one of ReportCategories.
func ValidCategory(c string) bool {
	for _, k := range ReportCategories {
		if c == k {
			return true
		}
	}
	return false
}

// Report is one row of the moderation queue, with both parties resolved.
type Report struct {
	ID         int64
	Created    time.Time
	Category   string
	Note       string
	Reporter   string
	Target     string
	TargetID   int64
	GameID     string // empty when the report names no specific game
	Resolved   time.Time
	Resolver   string
	Resolution string
}

// FileReport records a report. A second open report from the same reporter
// against the same account returns ErrAlreadyReported.
func FileReport(reporterID, targetID int64, gameID *uuid.UUID,
	category, note string) error {
	ctx, cancel := Ctx()
	defer cancel()
	_, err := gen.New(Pool).CreateReport(ctx, gen.CreateReportParams{
		ReporterUserID: reporterID,
		TargetUserID:   targetID,
		GameID:         pgUUID(gameID),
		Category:       category,
		Note:           note,
	})
	if isUniqueViolation(err) {
		return ErrAlreadyReported
	}
	return err
}

// OpenReports returns a page of the queue, oldest first.
func OpenReports(limit, offset int32) ([]Report, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListOpenReports(ctx, gen.ListOpenReportsParams{
		Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Report, 0, len(rows))
	for _, r := range rows {
		out = append(out, Report{
			ID:       r.ID,
			Created:  r.CreatedAt.Time,
			Category: r.Category,
			Note:     r.Note,
			Reporter: r.ReporterUsername,
			Target:   r.TargetUsername,
			TargetID: r.TargetUserID,
			GameID:   uuidOrEmpty(r.GameID),
		})
	}
	return out, nil
}

// ClosedReports returns the most recently resolved reports.
func ClosedReports(limit int32) ([]Report, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListClosedReports(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Report, 0, len(rows))
	for _, r := range rows {
		out = append(out, Report{
			ID:         r.ID,
			Created:    r.CreatedAt.Time,
			Category:   r.Category,
			Note:       r.Note,
			Reporter:   r.ReporterUsername,
			Target:     r.TargetUsername,
			GameID:     uuidOrEmpty(r.GameID),
			Resolved:   r.ResolvedAt.Time,
			Resolver:   strOrEmpty(r.ResolverUsername),
			Resolution: strOrEmpty(r.Resolution),
		})
	}
	return out, nil
}

// CountOpenReports returns the size of the queue.
func CountOpenReports() (int64, error) {
	if Pool == nil {
		return 0, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).CountOpenReports(ctx)
}

// OpenReportsAgainst counts the open reports naming one account.
func OpenReportsAgainst(userID int64) (int64, error) {
	if Pool == nil {
		return 0, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).CountOpenReportsForUser(ctx, userID)
}

// ResolveReport closes an open report, returning the account it named. ok=false
// (with no error) means the report was already closed — two moderators working
// the queue at once, where the second should be told rather than silently
// overwriting the first's decision.
func ResolveReport(id, resolverID int64, resolution string) (targetID int64, ok bool, err error) {
	ctx, cancel := Ctx()
	defer cancel()
	targetID, err = gen.New(Pool).ResolveReport(ctx, gen.ResolveReportParams{
		ID:         id,
		ResolvedBy: &resolverID,
		Resolution: &resolution,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return targetID, true, nil
}

// pgUUID encodes an optional game reference for storage. A report that names no
// specific game stores NULL rather than a zero uuid, so the FK stays honest.
func pgUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: *id, Valid: true}
}

// uuidOrEmpty renders a nullable game reference for display.
func uuidOrEmpty(v pgtype.UUID) string {
	if !v.Valid {
		return ""
	}
	return uuid.UUID(v.Bytes).String()
}
