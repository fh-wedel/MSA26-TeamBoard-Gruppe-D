-- Remove the hardcoded board type allowlist from the DB.
-- Validation is now handled by the boardplugins registry, which supports
-- arbitrary types registered via boardplugins.Register().
ALTER TABLE boards DROP CONSTRAINT IF EXISTS boards_type_check;
