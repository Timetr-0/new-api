#!/usr/bin/env bash
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."

command -v openssl >/dev/null
docker compose version >/dev/null

umask 077
touch .env
chmod 600 .env

for key in POSTGRES_PASSWORD SESSION_SECRET CRYPTO_SECRET REDIS_PASSWORD; do
  if grep -Eq "^[[:space:]]*${key}[[:space:]]*=" .env; then
    continue
  fi

  secret_value="${!key:-}"
  if [[ -z "$secret_value" ]]; then
    secret_value=$(openssl rand -hex 32)
  fi
  printf '\n%s=%s\n' "$key" "$secret_value" >> .env
done

docker compose config --quiet
docker compose build new-api
docker compose up -d --no-build
