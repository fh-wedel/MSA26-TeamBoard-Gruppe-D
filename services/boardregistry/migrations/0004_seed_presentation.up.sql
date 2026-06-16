-- Presentation specs for the built-in types, and promote 'gantt' to a first-class
-- built-in (it previously only existed as runtime data). Each spec selects a
-- built-in frontend renderer and parametrizes it declaratively.

UPDATE board_types
SET presentation = '{
    "view":"board",
    "view_config":{"group_by":"column","show_wip":true},
    "card":{"fields":["priority","due_date","labels","comment_count","attachment_count"],"color_by":"priority"}
}'::jsonb
WHERE type IN ('kanban','scrum');

UPDATE board_types
SET presentation = '{
    "view":"calendar",
    "view_config":{"date_field":"due_date","week_start":"monday"},
    "card":{"fields":["priority","labels"],"color_by":"priority"}
}'::jsonb
WHERE type = 'calendar';

-- Gantt as a built-in timeline type. ON CONFLICT brings an existing runtime row in
-- line (sets the timeline presentation and marks it built-in).
INSERT INTO board_types (id, type, display_name, icon, default_columns, default_config, config_schema, presentation, built_in)
VALUES (
    gen_random_uuid(), 'gantt', 'Gantt (Timeline)', '📊',
    '[
        {"name":"Planned","position":0,"status":"open"},
        {"name":"In Progress","position":1,"status":"in_progress"},
        {"name":"Done","position":2,"status":"done"}
    ]'::jsonb,
    '{}'::jsonb,
    '{}'::jsonb,
    '{
        "view":"timeline",
        "view_config":{"start_field":"start_date","end_field":"due_date","group_by":"column","color_by":"priority"},
        "card":{"fields":["priority","due_date"],"color_by":"priority"}
    }'::jsonb,
    TRUE
)
ON CONFLICT (type) DO UPDATE SET
    display_name   = EXCLUDED.display_name,
    icon           = EXCLUDED.icon,
    default_columns = EXCLUDED.default_columns,
    presentation   = EXCLUDED.presentation,
    built_in       = TRUE,
    updated_at     = NOW();
