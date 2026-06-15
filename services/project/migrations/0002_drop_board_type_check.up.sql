-- Remove the hardcoded board type allowlist from the DB.
-- Board types are resolved at runtime against the Board Registry service
-- (boardregistry_db); boards.type is a free slug validated via that registry.
-- NOTE: the constraint is named `boards_type` (see 0001_init). Both names are
-- dropped defensively; see 0005 for the fix applied to already-migrated DBs.
ALTER TABLE boards DROP CONSTRAINT IF EXISTS boards_type;
ALTER TABLE boards DROP CONSTRAINT IF EXISTS boards_type_check;
