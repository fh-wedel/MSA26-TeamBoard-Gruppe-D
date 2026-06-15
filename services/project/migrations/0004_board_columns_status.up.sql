-- Each column carries an explicit semantic task status, sourced from the board
-- type definition. The task service uses this instead of guessing from the name.
ALTER TABLE board_columns ADD COLUMN status TEXT NOT NULL DEFAULT 'open';
