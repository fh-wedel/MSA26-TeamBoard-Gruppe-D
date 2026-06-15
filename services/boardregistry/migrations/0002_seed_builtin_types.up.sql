-- Built-in board types migrated from the former in-process boardplugins registry.
-- Each default column carries an explicit semantic status so the task service no
-- longer has to guess the status from the column name.

INSERT INTO board_types (id, type, display_name, icon, default_columns, default_config, config_schema, built_in)
VALUES
(
    gen_random_uuid(), 'kanban', 'Kanban Board', '📋',
    '[
        {"name":"To Do","position":0,"status":"open"},
        {"name":"In Progress","position":1,"wip_limit":3,"status":"in_progress"},
        {"name":"Done","position":2,"status":"done"}
    ]'::jsonb,
    '{}'::jsonb,
    '{}'::jsonb,
    TRUE
),
(
    gen_random_uuid(), 'scrum', 'Scrum Board', '🏃',
    '[
        {"name":"Backlog","position":0,"status":"open"},
        {"name":"Sprint","position":1,"status":"open"},
        {"name":"In Progress","position":2,"status":"in_progress"},
        {"name":"Review","position":3,"status":"in_progress"},
        {"name":"Done","position":4,"status":"done"}
    ]'::jsonb,
    '{"sprint_length_days":14}'::jsonb,
    '{"type":"object","properties":{"sprint_length_days":{"type":"integer","minimum":1,"maximum":90}},"additionalProperties":false}'::jsonb,
    TRUE
),
(
    gen_random_uuid(), 'calendar', 'Calendar', '📅',
    '[]'::jsonb,
    '{"week_start":"monday"}'::jsonb,
    '{"type":"object","properties":{"week_start":{"type":"string","enum":["monday","sunday"]}},"additionalProperties":false}'::jsonb,
    TRUE
);
