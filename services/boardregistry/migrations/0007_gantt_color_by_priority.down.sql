UPDATE board_types
SET presentation = jsonb_set(
        jsonb_set(presentation, '{view_config,color_by}', '"status"'::jsonb),
        '{card,color_by}', '"status"'::jsonb)
WHERE type = 'gantt';
