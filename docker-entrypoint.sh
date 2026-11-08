#!/bin/sh
# Starts the API. Set RUN_MIGRATIONS=true to apply any new database
# migrations first (safe to leave on: goose skips ones already applied).
set -e

if [ "$RUN_MIGRATIONS" = "true" ]; then
  echo "Applying database migrations..."
  goose -dir /app/migrations postgres "$DATABASE_URL" up
fi

exec "$@"
