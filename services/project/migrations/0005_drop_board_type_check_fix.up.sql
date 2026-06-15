-- Fix: migration 0002 dropped the wrong constraint name (boards_type_check),
-- but the actual constraint is named boards_type, so the allowlist was never
-- removed. Board types are now resolved at runtime via the Board Registry, so the
-- hardcoded CHECK must be gone for custom types (e.g. gantt) to work.
ALTER TABLE boards DROP CONSTRAINT IF EXISTS boards_type;
