ALTER TABLE boards ADD CONSTRAINT boards_type_check
    CHECK (type IN ('kanban', 'scrum', 'calendar'));
