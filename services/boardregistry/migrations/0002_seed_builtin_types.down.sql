DELETE FROM board_types WHERE type IN ('kanban', 'scrum', 'calendar') AND built_in = TRUE;
