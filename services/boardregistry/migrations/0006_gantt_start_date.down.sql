UPDATE board_types
SET presentation = jsonb_set(presentation, '{view_config,start_field}', '"created_at"'::jsonb)
WHERE type = 'gantt';
