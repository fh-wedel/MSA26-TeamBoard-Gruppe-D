-- A calendar board still needs a column to hold tasks (the task service binds tasks
-- to a column and learns boards via board.created carrying its columns). The
-- calendar renderer ignores columns and lays tasks out by date, so a single bucket
-- column is enough to make calendar boards usable.
UPDATE board_types
SET default_columns = '[{"name":"Scheduled","position":0,"status":"open"}]'::jsonb
WHERE type = 'calendar';
