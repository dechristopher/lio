// Package backfill replays the MinIO PGN archive into the Postgres
// games/moves/positions tables, recovering history that predates the relational
// archive (arch/STATE_PERSISTENCE_SCALING.md, Layer 3). It reuses the live
// archive's octad replay + encoding (db.BuildPlies) so a replayed game hashes
// identically to a live one, and dedups on pgn_object_key so it skips games
// already recorded and is safe to re-run. Invoked once via `lio --backfill`.
package backfill

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/google/uuid"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/store"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// backfillNamespace derives a stable game_id from a PGN object key, so a
// re-run maps each object to the same UUID (a second safety net behind the
// pgn_object_key unique constraint). Any fixed UUID works.
var backfillNamespace = uuid.MustParse("6f8a1e2c-0d3b-4a5e-9c7f-1b2d3e4f5a6b")

// tagRe matches PGN tag pairs: [Key "Value"].
var tagRe = regexp.MustCompile(`\[(\w+)\s+"([^"]*)"\]`)

// Run replays every archived PGN into Postgres, skipping games already recorded
// (dedup by pgn_object_key). It needs both the object store and Postgres
// configured. Idempotent: safe to re-run or resume after an interruption.
func Run() error {
	if !store.Configured() {
		return fmt.Errorf("backfill: object store not configured (need lio_obj_*)")
	}
	if !db.Ready() {
		return fmt.Errorf("backfill: postgres not configured (need lio_pg_dsn)")
	}

	ctx := context.Background()
	keys, err := store.PGNBucket.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("backfill: list keys: %w", err)
	}
	util.Info(str.CDB, "backfill: %d archived PGNs to scan", len(keys))

	var inserted, skipped, failed int
	for i, key := range keys {
		data, err := store.PGNBucket.GetObject(key)
		if err != nil {
			util.Error(str.CDB, "backfill: get %s error=%s", key, err.Error())
			failed++
			continue
		}
		rec, plies, err := parsePGN(key, data)
		if err != nil {
			util.Error(str.CDB, "backfill: parse %s error=%s", key, err.Error())
			failed++
			continue
		}
		ok, err := db.ArchiveGameIfNew(ctx, rec, plies)
		if err != nil {
			util.Error(str.CDB, "backfill: archive %s error=%s", key, err.Error())
			failed++
			continue
		}
		if ok {
			inserted++
		} else {
			skipped++
		}
		if (i+1)%500 == 0 {
			util.Info(str.CDB, "backfill: %d/%d (inserted=%d skipped=%d failed=%d)",
				i+1, len(keys), inserted, skipped, failed)
		}
	}

	util.Info(str.CDB, "backfill: done — scanned=%d inserted=%d skipped=%d failed=%d",
		len(keys), inserted, skipped, failed)
	return nil
}

// parsePGN turns one archived PGN into a GameRecord + analytics plies. Tag
// metadata is read from the raw text; the movetext is replayed through octad
// (which honors the SetUp/FEN tag for deploy games) to derive positions, hashes,
// the packed move blob, and the board-determined method.
func parsePGN(key string, data []byte) (db.GameRecord, []db.PlyRecord, error) {
	tags := parseTags(data)

	// the Scanner decodes a game only after two blank lines; pad so a single
	// archived PGN (tags + one blank + movetext) always terminates cleanly.
	sc := octad.NewScanner(strings.NewReader(string(data) + "\n\n\n"))
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return db.GameRecord{}, nil, fmt.Errorf("decode: %w", err)
		}
		return db.GameRecord{}, nil, fmt.Errorf("no game decoded")
	}
	g := sc.Next()
	if g == nil {
		return db.GameRecord{}, nil, fmt.Errorf("nil game")
	}

	start := parsePGNTime(tags["Date"], tags["Time"])
	end := parsePGNTime(tags["EndDate"], tags["EndTime"])
	if end.IsZero() {
		end = start // pre-EndTime PGNs: end_ts approximates start (per the design)
	}

	method, reason := classify(tags["Reason"], tags["Result"], g.Method())
	// nil times: PGNs predating per-move timing carry nothing to recover
	blob, plies := db.BuildPlies(g, nil)

	var startOFEN string
	if positions := g.Positions(); len(positions) > 0 {
		startOFEN = positions[0].String()
	}

	rec := db.GameRecord{
		GameID:       uuid.NewSHA1(backfillNamespace, []byte(key)).String(),
		StartTs:      start,
		EndTs:        end,
		WhiteUID:     tags["White"],
		BlackUID:     tags["Black"],
		VariantName:  tags["Variant"],
		VariantGroup: tags["Group"],
		Outcome:      resultOrOutcome(tags["Result"], g),
		Method:       method,
		Reason:       reason,
		StartingOFEN: startOFEN,
		Moves:        blob,
		PGNObjectKey: key,
		// unavailable from a single PGN — see the backfill design notes:
		// room/creator/race/scores are per-room/per-match context, not per-game.
	}
	return rec, plies, nil
}

// parseTags reads all [Key "Value"] tag pairs from the raw PGN text.
func parseTags(data []byte) map[string]string {
	tags := map[string]string{}
	for _, m := range tagRe.FindAllStringSubmatch(string(data), -1) {
		tags[m[1]] = strings.TrimSpace(m[2])
	}
	return tags
}

// parsePGNTime reconstructs a timestamp from the PGN Date + Time tags. The tags
// carry no timezone, so it parses in the process's local zone — the backfill
// runs in the same container timezone that wrote them.
func parsePGNTime(date, clock string) time.Time {
	if date == "" || clock == "" {
		return time.Time{}
	}
	t, err := time.ParseInLocation("2006.01.02 15:04:05", date+" "+clock, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

// resultOrOutcome prefers the recorded Result tag (which captures declared
// outcomes the replay can't reproduce), falling back to the replayed board
// outcome.
func resultOrOutcome(result string, g *octad.Game) string {
	switch result {
	case "1-0", "0-1", "1/2-1/2":
		return result
	}
	return g.Outcome().String()
}

// classify derives the method enum and short reason code for a backfilled game.
// The replay is authoritative for board outcomes (checkmate, stalemate,
// repetition, ...); declared outcomes (resignation, agreed draw) and flag wins
// aren't in the movetext, so they're recovered from the human Reason tag. The
// short reason code matches the live gameOverReasonLocked vocabulary so
// backfilled rows are queryable alongside live ones.
func classify(reasonTag, result string, replayed octad.Method) (int16, string) {
	r := strings.ToUpper(reasonTag)

	var reason string
	switch {
	case strings.Contains(r, "OUT OF TIME"):
		reason = "time"
	case strings.Contains(r, "CHECKMATE"):
		reason = "checkmate"
	case strings.Contains(r, "RESIGNED"):
		reason = "resignation"
	case strings.Contains(r, "STALEMATE"):
		reason = "stalemate"
	case strings.Contains(r, "INSUFFICIENT"):
		reason = "insufficient"
	case strings.Contains(r, "REPETITION"):
		reason = "repetition"
	case strings.Contains(r, "25 MOVE"):
		reason = "moverule"
	case strings.Contains(r, "AGREEMENT"):
		reason = "agreement"
	}

	// method: replay first (board truth); recover declared outcomes from reason.
	method := replayed
	if method == octad.NoMethod {
		switch reason {
		case "checkmate":
			method = octad.Checkmate
		case "resignation":
			method = octad.Resignation
		case "stalemate":
			method = octad.Stalemate
		case "insufficient":
			method = octad.InsufficientMaterial
		case "repetition":
			method = octad.ThreefoldRepetition
		case "moverule":
			method = octad.TwentyFiveMoveRule
		case "agreement":
			method = octad.DrawOffer
			// "time": no Method enum for a flag — stays NoMethod, matching live.
		}
	}

	// unrecognized/absent reason tag: derive a code from the method + result.
	if reason == "" {
		reason = reasonFromMethod(method, result)
	}
	return int16(method), reason
}

// reasonFromMethod maps a method enum to the live short reason code, using the
// result to guess declared outcomes when no board method applies.
func reasonFromMethod(m octad.Method, result string) string {
	switch m {
	case octad.Checkmate:
		return "checkmate"
	case octad.Resignation:
		return "resignation"
	case octad.Stalemate:
		return "stalemate"
	case octad.InsufficientMaterial:
		return "insufficient"
	case octad.ThreefoldRepetition:
		return "repetition"
	case octad.TwentyFiveMoveRule:
		return "moverule"
	case octad.DrawOffer:
		return "agreement"
	}
	switch result {
	case "1/2-1/2":
		return "agreement"
	case "1-0", "0-1":
		return "resignation"
	}
	return ""
}
