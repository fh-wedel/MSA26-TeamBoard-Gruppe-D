-- Creates one schema per microservice. Each service uses SQLAlchemy
-- create_all() against its own schema, no cross-schema FKs.
CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS projects;
CREATE SCHEMA IF NOT EXISTS tasks;
CREATE SCHEMA IF NOT EXISTS documents;
