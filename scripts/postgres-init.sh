#!/bin/bash
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE auth_db;
    CREATE DATABASE project_db;
    CREATE DATABASE task_db;
    CREATE DATABASE document_db;
    CREATE DATABASE notification_db;
    CREATE DATABASE plugin_db;
EOSQL

echo "All service databases created."
