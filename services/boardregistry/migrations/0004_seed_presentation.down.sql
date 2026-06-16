-- Reset presentation for built-ins; remove the gantt built-in seed.
UPDATE board_types SET presentation = '{}'::jsonb WHERE type IN ('kanban','scrum','calendar');
DELETE FROM board_types WHERE type = 'gantt' AND built_in = TRUE;
