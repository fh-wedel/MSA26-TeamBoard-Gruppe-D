-- Color timeline bars by task priority (importance) instead of status, so urgency
-- is visible at a glance on the Gantt chart. Updates the already-seeded gantt row.
UPDATE board_types
SET presentation = jsonb_set(
        jsonb_set(presentation, '{view_config,color_by}', '"priority"'::jsonb),
        '{card,color_by}', '"priority"'::jsonb)
WHERE type = 'gantt';
