#!/bin/bash
set -e
# Enable pgvector extension so OpenWebUI can use PostgreSQL as a vector store for RAG.
# This runs once on first database initialization (empty volume).
# For existing deployments run manually:
#   docker exec bitnet-postgres psql -U $POSTGRES_USER -d $POSTGRES_DB -c "CREATE EXTENSION IF NOT EXISTS vector;"
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  -c "CREATE EXTENSION IF NOT EXISTS vector;"
