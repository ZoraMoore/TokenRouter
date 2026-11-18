#!/usr/bin/env bash
set -euo pipefail

repo_root="${1:-}"
if [[ -z "$repo_root" ]]; then
  repo_root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
fi

if [[ -z "$repo_root" || ! -d "$repo_root/.git" ]]; then
  printf '[FAIL] repository root is missing or is not a Git worktree\n' >&2
  exit 2
fi

failures=0

pass() {
  printf '[PASS] %s\n' "$1"
}

fail() {
  printf '[FAIL] %s\n' "$1" >&2
  failures=$((failures + 1))
}

require_pattern() {
  local label="$1"
  local pattern="$2"
  local path="$3"

  if [[ ! -f "$repo_root/$path" ]]; then
    fail "$label: missing $path"
    return
  fi

  if rg -q -- "$pattern" "$repo_root/$path"; then
    pass "$label"
  else
    fail "$label: pattern not found in $path"
  fi
}

require_file() {
  local label="$1"
  local path="$2"

  if [[ -f "$repo_root/$path" ]]; then
    pass "$label"
  else
    fail "$label: missing $path"
  fi
}

require_pattern \
  'EasyPay order query uses GET helper' \
  'body, statusCode, err := e\.getRaw\(ctx, e\.apiBase\(\)\+"/api\.php", params\)' \
  'backend/internal/payment/provider/easypay.go'

require_pattern \
  'EasyPay GET helper strips URL-bearing transport errors' \
  'if urlErr, ok := err\.\(\*url\.Error\); ok && urlErr\.Err != nil' \
  'backend/internal/payment/provider/easypay.go'

require_pattern \
  'EasyPay provider transport redaction test exists' \
  'TestEasyPayQueryOrderTransportErrorDoesNotLeakCredential' \
  'backend/internal/payment/provider/easypay_test.go'

require_pattern \
  'EasyPay service draft test requires GET' \
  'require\.Equal\(t, http\.MethodGet, r\.Method\)' \
  'backend/internal/payment/postgres/configuration_scenario_test.go'

require_pattern \
  'Docker frontend build heap remains 4096 MiB' \
  'ENV NODE_OPTIONS=--max-old-space-size=4096' \
  'deploy/Dockerfile'

require_pattern \
  'Standalone docs site is excluded from the main image context' \
  '^website-docs/$' \
  '.dockerignore'

require_file 'Standalone documentation site package exists' 'website-docs/package.json'
require_file 'Standalone documentation site config exists' 'website-docs/.vitepress/config.mts'

# TLS 二开必须与支付、文档和构建约束一同保留；静态标记不能替代对应回归测试。
require_pattern 'TLS profile target and approval gate exists' 'func .*IsOutboundReadyForTarget' 'backend/internal/egress/tls_profile.go'
require_pattern 'TLS profile explicit approval exists' 'func .*SetApproval' 'backend/internal/egress/tls_profile_service.go'
require_pattern 'Authenticated ingress request association exists' 'SetAuthenticatedIngressRequestObserver' 'backend/internal/app/egress.go'
require_pattern 'Authenticated ingress hook follows API key authentication' 'ObserveAuthenticatedIngressRequest' 'backend/internal/apikey/httpapi/authenticate.go'
require_pattern 'Google API key authentication retains ingress hook' 'ObserveAuthenticatedIngressRequest' 'backend/internal/apikey/httpapi/authenticate.go'
require_file 'Authenticated ingress Redis quota store exists' 'backend/internal/egress/rediscache/authenticated_ingress_quota.go'
require_file 'Linux TLS observer exists' 'backend/cmd/tls-ingress-observer/capture_linux.go'
require_pattern 'Custom image includes TLS observer' 'COPY --from=backend-builder /app/tls-ingress-observer /app/tls-ingress-observer' 'deploy/Dockerfile'
require_pattern 'Compose retains TLS observer' '^  tls-ingress-observer:' 'deploy/docker-compose.yml'
require_file 'Trusted Nginx ingress header template exists' 'deploy/nginx-authenticated-ingress-tls.conf.example'
require_file 'Public internal route block template exists' 'deploy/nginx-authenticated-ingress-public-block.conf.example'
require_file 'Applied TLS verification migration remains present' 'backend/migrations/259_add_tls_fingerprint_profile_verification.sql'
require_file 'Independent image billing migration exists' 'backend/migrations/298_preserve_custom_image_billing.sql'
require_pattern 'Historical OpenAI platform uses constant default' "ADD COLUMN IF NOT EXISTS platform VARCHAR\\(64\\) NOT NULL DEFAULT 'openai'" 'backend/migrations/278_platform_independent_groups.sql'
require_pattern 'Future platform default remains unknown' "ALTER COLUMN platform SET DEFAULT 'unknown'" 'backend/migrations/278_platform_independent_groups.sql'
require_pattern 'Platform backfill skips matching rows' 'ul.platform IS DISTINCT FROM g.platform' 'backend/migrations/278_platform_independent_groups.sql'
require_pattern 'Image settlement preserves independent plan rates' 'cmd.DisablePlanGroupRateMultiplier = true' 'backend/internal/gateway/completion/settlement.go'
require_pattern 'Public image quotes retain independent rates' 'settings.ImageRateIndependent' 'backend/internal/billing/public_quote.go'
require_pattern 'Embedded frontend bypasses internal TLS routes' 'strings.HasPrefix\(trimmed, "/internal/"\)' 'backend/internal/web/embed_on.go'
require_pattern 'Vite avoids a duplicate production typecheck' 'enableBuild: false' 'frontend/vite.config.ts'

version_file="$repo_root/backend/cmd/server/VERSION"
if [[ -f "$version_file" ]] && rg -q '^[0-9]+\.[0-9]+\.[0-9]+$' "$version_file"; then
  pass "Application VERSION is semantic: $(tr -d '\r\n' < "$version_file")"
else
  fail 'Application VERSION is missing or is not MAJOR.MINOR.PATCH'
fi

if git -C "$repo_root" diff --check; then
  pass 'Git diff has no whitespace errors'
else
  fail 'Git diff contains whitespace errors'
fi

if (( failures > 0 )); then
  printf '[RESULT] %d customization check(s) failed\n' "$failures" >&2
  exit 1
fi

printf '[RESULT] customization contract preserved\n'
