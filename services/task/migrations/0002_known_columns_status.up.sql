-- Explicit semantic status for a column, propagated from the board type via
-- board.created / column.* events. Task status is derived from this instead of
-- guessing from the column name (DeriveStatus remains a fallback).
ALTER TABLE known_columns ADD COLUMN status TEXT NOT NULL DEFAULT 'open';
