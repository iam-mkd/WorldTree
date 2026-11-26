#!/usr/bin/env bash
# Emulates the worktrunk post-start hook body. worktrunk would template these via
# {{ branch | hash_port }}; its algorithm is undocumented, so this uses cksum mod 10000 + 10000.
set -euo pipefail
branch="$1"; repo="notes"
hp() { echo $(( $(printf '%s' "$1" | cksum | cut -d' ' -f1) % 10000 + 10000 )); }
cat > .env <<ENV
COMPOSE_PROJECT_NAME=${repo}-${branch//[^a-z0-9]/-}
COMPOSE_FILE=docker-compose.yml:docker-compose.worldtree.yml
COMPOSE_PATH_SEPARATOR=:
PG_PORT=$(hp "pg-$branch")
REDIS_PORT=$(hp "redis-$branch")
APP_PORT=$(hp "$branch")
DB_PORT=$(hp "pg-$branch")
ENV
cat .env
