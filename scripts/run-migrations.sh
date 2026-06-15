#!/bin/sh
# Idempotent migration runner. Tracks applied files in schema_migrations per DB.
set -e

PG_HOST="${PG_HOST:-postgres}"
PG_USER="${POSTGRES_USER:-teamboard}"
export PGPASSWORD="${POSTGRES_PASSWORD:-teamboard}"

run_psql() {
    db="$1"; shift
    psql -h "$PG_HOST" -U "$PG_USER" -d "$db" -qAt "$@"
}

ensure_tracking_table() {
    psql -h "$PG_HOST" -U "$PG_USER" -d "$1" \
        -c "CREATE TABLE IF NOT EXISTS schema_migrations (filename TEXT PRIMARY KEY, applied_at TIMESTAMPTZ DEFAULT NOW());" \
        >/dev/null 2>&1
}

run_for_db() {
    db="$1"
    dir="$2"
    [ -d "$dir" ] || return 0
    ensure_tracking_table "$db"
    for f in $(ls "$dir"/*.up.sql 2>/dev/null | sort); do
        fname=$(basename "$f")
        applied=$(run_psql "$db" -c "SELECT COUNT(*) FROM schema_migrations WHERE filename='$fname';")
        if [ "$applied" = "0" ]; then
            echo "  [$db] applying $fname"
            psql -h "$PG_HOST" -U "$PG_USER" -d "$db" -f "$f" >/dev/null
            psql -h "$PG_HOST" -U "$PG_USER" -d "$db" \
                -c "INSERT INTO schema_migrations(filename) VALUES('$fname');" >/dev/null
        else
            echo "  [$db] skip $fname (already applied)"
        fi
    done
}

echo "Running migrations..."
run_for_db auth_db         /migrations/auth
run_for_db project_db      /migrations/project
run_for_db task_db         /migrations/task
run_for_db document_db     /migrations/document
run_for_db notification_db /migrations/notification
run_for_db plugin_db       /migrations/plugin
echo "Migrations complete."
