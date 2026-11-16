#!/bin/bash
# Runs once, on the first start of an empty data volume. Creates one database
# per service and applies that service's migrations, so the services stay
# independent and never touch each other's tables.
set -euo pipefail

for pair in product:products cart:carts order:orders; do
  service="${pair%%:*}"
  database="${pair##*:}"

  psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "CREATE DATABASE $database"

  for file in /migrations/"$service"/*.sql; do
    echo "applying $file to $database"
    psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$database" -f "$file"
  done
done
