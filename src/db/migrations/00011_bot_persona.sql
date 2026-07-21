-- +goose Up

-- Bot difficulty stamp: the engine.Personas key ("pawn".."queen") of the bot
-- seat's persona for a bot game. NULL for human games and for bot games
-- archived before the persona ladder existed — those all played at full Queen
-- strength, so the display layer resolves NULL to the Queen.
ALTER TABLE games
    ADD COLUMN bot_persona TEXT;

-- +goose Down
ALTER TABLE games
    DROP COLUMN IF EXISTS bot_persona;
