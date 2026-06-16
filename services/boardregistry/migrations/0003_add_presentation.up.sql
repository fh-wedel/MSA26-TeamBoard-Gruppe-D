-- Presentation spec: declarative rendering hints (which built-in view renderer to
-- use and how to parametrize it) carried by each board-type definition. Validated
-- against a host-defined meta-schema in the domain layer, not author-defined.
ALTER TABLE board_types ADD COLUMN presentation JSONB NOT NULL DEFAULT '{}'::jsonb;
