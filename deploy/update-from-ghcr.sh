#!/usr/bin/env bash
set -Eeuo pipefail

DEPLOY_PATH="${DEPLOY_PATH:-/opt/sub2api}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-sub2api}"
TOKENROUTER_IMAGE="${TOKENROUTER_IMAGE:-ghcr.io/tokenflux/tokenrouter:latest}"
HEALTHCHECK_URL="${HEALTHCHECK_URL:-}"
GHCR_USERNAME="${GHCR_USERNAME:-}"
GHCR_TOKEN="${GHCR_TOKEN:-}"

log() {
  printf '[%s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"
}

fail() {
  log "ERROR: $*"
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "missing required command: $1"
}

upsert_env() {
  local key="$1"
  local value="$2"
  local file=".env"

  touch "$file"
  chmod 600 "$file"
  if grep -q "^${key}=" "$file"; then
    sed -i "s|^${key}=.*|${key}=${value}|" "$file"
  else
    printf '%s=%s\n' "$key" "$value" >>"$file"
  fi
}

read_env_value() {
  local key="$1"
  local file=".env"

  if [ -f "$file" ]; then
    grep -E "^${key}=" "$file" | tail -n 1 | cut -d= -f2- || true
  fi
}

ensure_compose_image_variable() {
  local file="$1"
  local tmp_file
  local replacement='    image: ${TOKENROUTER_IMAGE:-ghcr.io/tokenflux/tokenrouter:latest}'

  if grep -q 'TOKENROUTER_IMAGE' "$file"; then
    return 0
  fi

  tmp_file="$(mktemp)"
  awk -v replacement="$replacement" '
    /^  sub2api:[[:space:]]*$/ {
      in_sub2api = 1
      print
      next
    }
    in_sub2api && /^  [^[:space:]#][^:]*:[[:space:]]*$/ {
      in_sub2api = 0
    }
    in_sub2api && /^[[:space:]]+image:[[:space:]]*/ {
      print replacement
      changed = 1
      next
    }
    { print }
    END {
      if (!changed) {
        exit 42
      }
    }
  ' "$file" >"$tmp_file" || {
    rm -f "$tmp_file"
    fail "failed to update sub2api image line in ${file}"
  }

  cp "$file" "${file}.bak.$(date +%Y%m%d%H%M%S)"
  mv "$tmp_file" "$file"
}

wait_for_health() {
  local url="$1"
  local attempt

  [ -n "$url" ] || return 0
  require_command curl

  for attempt in $(seq 1 30); do
    if curl -fsS --max-time 5 "$url" >/dev/null; then
      log "health check passed: $url"
      return 0
    fi
    sleep 3
  done

  fail "health check failed after 30 attempts: $url"
}

main() {
  require_command docker

  mkdir -p "$DEPLOY_PATH"
  cd "$DEPLOY_PATH"

  [ -f "$COMPOSE_FILE" ] || fail "compose file not found: ${DEPLOY_PATH}/${COMPOSE_FILE}"
  [ -f ".env" ] || fail ".env not found: ${DEPLOY_PATH}/.env"

  ensure_compose_image_variable "$COMPOSE_FILE"
  upsert_env TOKENROUTER_IMAGE "$TOKENROUTER_IMAGE"

  if [ -z "$HEALTHCHECK_URL" ]; then
    server_port="$(read_env_value SERVER_PORT)"
    server_port="${server_port:-8080}"
    HEALTHCHECK_URL="http://127.0.0.1:${server_port}/health"
  fi

  if [ -n "$GHCR_USERNAME" ] && [ -n "$GHCR_TOKEN" ]; then
    docker_config="$(mktemp -d)"
    export DOCKER_CONFIG="$docker_config"
    trap 'rm -rf "$docker_config"' EXIT
    log "logging in to ghcr.io for this deployment"
    printf '%s' "$GHCR_TOKEN" | docker login ghcr.io -u "$GHCR_USERNAME" --password-stdin >/dev/null
  fi

  log "pulling image: ${TOKENROUTER_IMAGE}"
  docker compose -f "$COMPOSE_FILE" --project-name "$COMPOSE_PROJECT_NAME" pull sub2api

  log "starting services"
  docker compose -f "$COMPOSE_FILE" --project-name "$COMPOSE_PROJECT_NAME" up -d --remove-orphans

  log "current service status"
  docker compose -f "$COMPOSE_FILE" --project-name "$COMPOSE_PROJECT_NAME" ps

  wait_for_health "$HEALTHCHECK_URL"

  log "pruning unused images older than 7 days"
  docker image prune -f --filter "until=168h" >/dev/null || true

  log "deployment completed"
}

main "$@"
