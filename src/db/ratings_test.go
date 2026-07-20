package db

import (
	"testing"
	"time"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/rating"
)

// TestRatingUpdate exercises the transactional Glicko-2 update against a real
// Postgres: a rated, decisive game between two logged-in players moves both
// ratings (winner up, loser down) and seeds their rows; the no-op cases
// (casual, aborted, same account) leave ratings untouched. Skips without
// DEV_LIO_PG_DSN.
func TestRatingUpdate(t *testing.T) {
	skipNoDB(t)

	mk := func(tag string) int64 {
		email := tag + "@example.invalid"
		id, err := CreateUser("rate"+tag+time.Now().Format("150405.000"), &email, "$argon2id$fake")
		if err != nil {
			t.Fatalf("create user %s: %v", tag, err)
		}
		t.Cleanup(func() {
			ctx, cancel := Ctx()
			defer cancel()
			_, _ = Pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id) // cascades ratings
		})
		return id
	}
	white, black := mk("w"), mk("b")
	const cat = "blitz"

	apply := func(rec GameRecord) *RatingResult {
		ctx, cancel := Ctx()
		defer cancel()
		tx, err := Pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		res, err := applyRatingUpdate(ctx, gen.New(tx), rec)
		if err != nil {
			t.Fatalf("applyRatingUpdate: %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
		return res
	}

	// both start unrated (no row → default)
	if r := RatingOrDefault(white, cat); r.R != rating.DefaultRating {
		t.Fatalf("white not default before play: %v", r)
	}

	// a rated, decisive game (white wins) — the surfaced result carries the
	// winner's gain and loser's loss, keyed by seat uid
	res := apply(GameRecord{
		Rated: true, WhiteUserID: &white, BlackUserID: &black,
		WhiteUID: "wuid", BlackUID: "buid",
		Outcome: "1-0", VariantGroup: cat,
	})
	if res == nil || res.White.Delta <= 0 || res.Black.Delta >= 0 {
		t.Fatalf("rating result: %+v (want white>0, black<0)", res)
	}
	if res.White.UID != "wuid" || res.Black.UID != "buid" {
		t.Errorf("rating result uids: %+v", res)
	}
	// both were unrated going in → rating-at-game is the provisional default
	if res.White.AtGame != "1500?" || res.Black.AtGame != "1500?" {
		t.Errorf("rating-at-game: %+v (want both 1500?)", res)
	}
	w := RatingOrDefault(white, cat)
	b := RatingOrDefault(black, cat)
	if !(w.R > rating.DefaultRating) {
		t.Errorf("winner did not gain: %.2f", w.R)
	}
	if !(b.R < rating.DefaultRating) {
		t.Errorf("loser did not drop: %.2f", b.R)
	}
	if w.Games != 1 || b.Games != 1 {
		t.Errorf("games not counted: w=%d b=%d", w.Games, b.Games)
	}

	// no-op guards: casual, aborted, same-account, anon seat all leave ratings put
	before := RatingOrDefault(white, cat)
	apply(GameRecord{Rated: false, WhiteUserID: &white, BlackUserID: &black, Outcome: "1-0", VariantGroup: cat})
	apply(GameRecord{Rated: true, WhiteUserID: &white, BlackUserID: &black, Outcome: "*", VariantGroup: cat})
	apply(GameRecord{Rated: true, WhiteUserID: &white, BlackUserID: &white, Outcome: "1-0", VariantGroup: cat})
	apply(GameRecord{Rated: true, WhiteUserID: &white, BlackUserID: nil, Outcome: "1-0", VariantGroup: cat})
	if after := RatingOrDefault(white, cat); after.R != before.R || after.Games != before.Games {
		t.Errorf("no-op game changed rating: %v -> %v", before, after)
	}

	// ListRatingsForUser returns only played categories
	list, err := ListRatingsForUser(white)
	if err != nil || len(list) != 1 || list[0].Category != cat {
		t.Fatalf("ListRatingsForUser: %+v err=%v", list, err)
	}
}
