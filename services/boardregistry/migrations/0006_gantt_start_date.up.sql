-- Point the gantt timeline at the new task start_date field (was created_at, which
-- users can't control). Updates the already-seeded gantt row.
UPDATE board_types
SET presentation = jsonb_set(presentation, '{view_config,start_field}', '"start_date"'::jsonb)
WHERE type = 'gantt';
