-- +goose Up

-- games.white_score / black_score never held this game's score. They hold the
-- *cumulative match score* after it: room.go archives player.Score(), which
-- accumulates across a room's games (arch/PROFILE_STATS.md, "The score columns
-- are not per-game results"). The two coincide only for game 1 of a match, so
-- the old names invited `WHERE white_score = 1` — which silently miscounts every
-- later game of a match, and drops a 1.5 from all three of the =1/=0.5/=0
-- buckets while the total still counts it. That bug shipped and sat unnoticed in
-- every profile tally on the site.
--
-- Renaming rather than repurposing: the per-game result is already recoverable
-- from `outcome` (which is what HeadToHead, db.SeatScore and the profile
-- aggregates all use), so a genuinely per-game score column would have no
-- reader. The match total is a real value with a real consumer — the archive
-- page's match scoreboard — and now says what it is.
--
-- rooms.white_score / black_score are deliberately left alone: a room *is* a
-- match, so "score" at that grain is unambiguous.
ALTER TABLE games RENAME COLUMN white_score TO white_match_score;
ALTER TABLE games RENAME COLUMN black_score TO black_match_score;

-- +goose Down
ALTER TABLE games RENAME COLUMN black_match_score TO black_score;
ALTER TABLE games RENAME COLUMN white_match_score TO white_score;
