#!/usr/bin/env bash
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."

command -v openssl >/dev/null
if ! command -v docker >/dev/null; then
  printf 'Docker is not installed. Install Docker Engine before starting New-API.\n' >&2
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  compose=(docker compose)
elif command -v docker-compose >/dev/null &&
  docker-compose version --short 2>/dev/null | grep -Eq '^v?([2-9]|[1-9][0-9]+)\.'; then
  compose=(docker-compose)
else
  cat >&2 <<'MESSAGE'
Docker Compose 2 or newer is required. Install the official Linux CLI plugin:

  compose_plugin_dir="${DOCKER_CONFIG:-$HOME/.docker}/cli-plugins"
  mkdir -p "$compose_plugin_dir"
  curl -fL "https://github.com/docker/compose/releases/download/v5.6.0/docker-compose-linux-$(uname -m)" -o "$compose_plugin_dir/docker-compose"
  chmod +x "$compose_plugin_dir/docker-compose"
  docker compose version

Then run ./bin/docker-up.sh again.
MESSAGE
  exit 1
fi

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

"${compose[@]}" config --quiet
"${compose[@]}" build new-api
"${compose[@]}" up -d --no-build
