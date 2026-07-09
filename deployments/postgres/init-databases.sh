#!/bin/bash
# Init script — creates 4 separate databases, one per service.
# The PostgreSQL container runs this automatically on first start (empty volume).
set -e

databases=(
  "${ORDER_DB_NAME:-order_db}"
  "${INVENTORY_DB_NAME:-inventory_db}"
  "${PAYMENT_DB_NAME:-payment_db}"
  "${SAGA_DB_NAME:-saga_db}"
)

for db in "${databases[@]}"; do
  echo "Creating database: $db"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
    CREATE DATABASE "$db";
    GRANT ALL PRIVILEGES ON DATABASE "$db" TO "$POSTGRES_USER";
EOSQL
done

echo "✓ 4 databases created."
