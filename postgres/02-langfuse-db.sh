#!/bin/bash
set -e
# Create a dedicated database for Langfuse so it doesn't share schema with the memory service.
# For existing deployments run manually:
#   docker exec bitnet-postgres psql -U $POSTGRES_USER -c "CREATE DATABASE langfuse;"
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" \
  -c "CREATE DATABASE langfuse;"
