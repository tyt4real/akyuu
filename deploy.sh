#!/bin/bash
# Production deployment script for akyuu remote server with external PostgreSQL
# Run on the remote server after cloning the repo

set -e

echo "=== akyuu Production Deployment (External PostgreSQL) ==="

# Check for .env file
if [ ! -f .env ]; then
    echo "ERROR: .env file not found. Copy .env.prod.example to .env and fill in values."
    exit 1
fi

# Load env vars
source .env

# Verify required env vars
if [ -z "$POSTGRES_PASSWORD" ]; then
    echo "ERROR: POSTGRES_PASSWORD not set in .env"
    exit 1
fi

if [ -z "$MODAL_TOKEN_ID" ] || [ -z "$MODAL_TOKEN_SECRET" ]; then
    echo "ERROR: MODAL_TOKEN_ID and MODAL_TOKEN_SECRET must be set in .env"
    exit 1
fi

echo "Environment variables verified."

# Verify external PostgreSQL is accessible
echo "Checking external PostgreSQL connection..."
if ! pg_isready -h host.docker.internal -p 5433 -U akyuu -d akyuu > /dev/null 2>&1; then
    echo "WARNING: Cannot connect to PostgreSQL at host.docker.internal:5433"
    echo "Make sure PostgreSQL is running and accessible from Docker containers."
    echo "Continuing anyway..."
fi

echo "Environment variables verified."

# Pull latest images
echo "Pulling Docker images..."
docker compose -f docker-compose.prod.yml pull

# Create necessary directories
mkdir -p storage config/sites

# Start services
echo "Starting services..."
docker compose -f docker-compose.prod.yml up -d

# Show status
docker compose -f docker-compose.prod.yml ps

echo ""
echo "=== Deployment Complete ==="
echo "API available at: http://localhost:8080"
echo "Logs: docker compose -f docker-compose.prod.yml logs -f [service]"
echo ""
echo "To run embedding backlog on Modal:"
echo "  docker compose -f docker-compose.prod.yml run --rm archiver \\"
echo "    go run ./cmd/evalbench-runner -config /akyuu/config/config.yaml -modal -k 20"
echo ""
echo "To run evalbench on Modal:"
echo "  docker compose -f docker-compose.prod.yml run --rm archiver \\"
echo "    go run ./cmd/evalbench-runner -config /akyuu/config/config.yaml -modal -k 20"