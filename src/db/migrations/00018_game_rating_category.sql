-- +goose Up

-- rating_category: the game's Glicko-2 category — the variant HTMLName, e.g.
-- "one-two-rapid-deploy" (arch/PROFILE_STATS.md). Since 00013 a rating is keyed
-- per exact time control, so a per-category rating history has to know which
-- category each game belonged to, and the games row could not say.
--
-- The value is already computed at archive time (room.archiveToDatabase sets
-- rec.RatingCategory = g.Variant.HTMLName) and was simply never persisted.
-- (variant_name, variant_group) cannot substitute for it: the two unlimited
-- casual variants share Name "∞" and Group "unlimited", so the pair is not
-- unique. It happens to be unique across every *rateable* variant, which is
-- what makes the backfill below exact — but that is a coincidence of the
-- current variant set, not a property to build the read path on.
--
-- NULL means unrated, or archived before this column existed.
ALTER TABLE games
    ADD COLUMN rating_category TEXT;

-- Backfill the rated rows — the only ones a rating curve reads. Both the deploy
-- and classic forms of the four curated time controls are registered as rating
-- categories (pools.ratingCategories), so both are mapped here even though the
-- create modal currently offers only the deploy form. A rated row matching
-- neither arm stays NULL and is simply absent from the curve.
UPDATE games
SET rating_category = CASE variant_group || ' ' || variant_name
                          WHEN 'deploy ¼ + 0' THEN 'quarter-zero-bullet-deploy'
                          WHEN 'deploy ½ + 1' THEN 'half-one-blitz-deploy'
                          WHEN 'deploy 1 + 2' THEN 'one-two-rapid-deploy'
                          WHEN 'deploy 3 + 5' THEN 'three-five-rapid-deploy'
                          WHEN 'bullet ¼ + 0' THEN 'quarter-zero-blitz'
                          WHEN 'blitz ½ + 1' THEN 'half-one-blitz'
                          WHEN 'rapid 1 + 2' THEN 'one-two-rapid'
                          WHEN 'rapid 3 + 5' THEN 'three-five-rapid'
    END
WHERE rated;

-- The rating curve reads one account's rated games in time order. The partial
-- index keeps it off the unrated majority (every anonymous, casual and bot
-- game), which is most of the table.
CREATE INDEX games_rating_category_idx ON games (rating_category, start_ts)
    WHERE rated AND rating_category IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS games_rating_category_idx;
ALTER TABLE games
    DROP COLUMN IF EXISTS rating_category;
