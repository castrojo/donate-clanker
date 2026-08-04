#!/usr/bin/env bash
# Contract checks for mcp-app/ — the read-only Goose Desktop presentation layer.
#
# AGENTS.md says the MCP App "must never select work, control Hive, read tmux,
# expose credentials, persist state, or poll". `readOnlyHint: true` is a
# self-declared annotation that neither the MCP SDK nor Goose enforces, so this
# script is what actually holds the property.
#
# BEHAVIOURAL assertions (mcp-app/scripts/check-stdio-contract.mjs) rebuild the
# package, spawn the built stdio server, complete a real MCP handshake, and
# assert the served tools/list, resources/list, and resources/read responses.
# It calls neither fetch-backed tool, so it needs no network.
#
# TEXTUAL assertions live in this file. They are substring and regex checks
# over mcp-app/src, because "no timer anywhere in the source" and "no write
# API anywhere in the source" are absence properties that running one happy
# path cannot demonstrate.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

src="mcp-app/src"
fail=0

# forbid_pattern <extended-regex> <explanation> — no source line may match.
forbid_pattern() {
  local pattern="$1" explanation="$2" hits
  if hits="$(grep -rnE -- "$pattern" "$src")"; then
    while IFS= read -r hit; do
      echo "::error file=${hit%%:*}::${explanation}: ${hit}"
    done <<<"$hits"
    fail=1
  fi
  return 0
}

# require <file> <substring>... — every substring must appear.
require() {
  local file="$1" want
  shift
  for want in "$@"; do
    grep -qF -- "$want" "$file" || {
      echo "::error file=${file}::missing required: ${want}"
      fail=1
    }
  done
}

# ── behavioural: the built server's real stdio surface ────────────────────
# Rebuild first so a stale dist/ can never make a regressed source pass.
if [[ -d mcp-app/node_modules ]]; then
  npm --prefix mcp-app run build >/dev/null || {
    echo "::error file=mcp-app/scripts/build.mjs::package build failed"
    fail=1
  }
fi

if [[ -f mcp-app/dist/server.js ]]; then
  node mcp-app/scripts/check-stdio-contract.mjs || fail=1
else
  echo "::error file=mcp-app/package.json::dist/server.js is missing; run npm --prefix mcp-app ci && npm --prefix mcp-app run build"
  fail=1
fi

# ── textual: no mutating HTTP ─────────────────────────────────────────────
# Every outbound request is a read. fetch() defaults to GET, so any `method:`
# that is not literally 'GET', and any request `body:` at all, is a regression.
if hits="$(grep -rnE "method[[:space:]]*:[[:space:]]*['\"]" "$src" | grep -viE "method[[:space:]]*:[[:space:]]*'GET'")"; then
  while IFS= read -r hit; do
    echo "::error file=${hit%%:*}::non-GET HTTP method in a read-only app: ${hit}"
  done <<<"$hits"
  fail=1
fi
forbid_pattern "\bbody[[:space:]]*:" "request body in a read-only app"

# The server data boundary is stdio only; an HTTP listener would make the
# presentation layer a service.
forbid_pattern "StreamableHTTPServerTransport|SSEServerTransport|createServer|\.listen\(" \
  "the MCP App must serve stdio only"

# ── textual: no timers, polling, or background lifecycle ──────────────────
forbid_pattern "\b(setInterval|setTimeout|requestAnimationFrame|queueMicrotask)\b" \
  "timers turn a snapshot panel into a background lifecycle"
forbid_pattern "\bnew (WebSocket|EventSource|Worker|SharedWorker|BroadcastChannel)\b" \
  "a persistent transport or background worker is a polling lifecycle"
forbid_pattern "navigator\.serviceWorker|registerServiceWorker" \
  "a service worker is a background lifecycle"

# ── textual: no persistence ───────────────────────────────────────────────
forbid_pattern "\b(localStorage|sessionStorage|indexedDB|openDatabase)\b" \
  "the panel must hold no state across loads"
forbid_pattern "document\.cookie" "the panel must hold no state across loads"
# The panel HTML is inlined at build time, so the shipped server needs no
# filesystem module at all — which removes every write API with it.
forbid_pattern "node:fs|require\(['\"]fs['\"]\)|from ['\"]fs" \
  "the server must import no filesystem module"
forbid_pattern "\b(writeFile|writeFileSync|appendFile|appendFileSync|mkdir|mkdirSync|rmdir|unlink|createWriteStream|copyFile|rename|truncate|chmod|chown)\b" \
  "a filesystem write API is persistence"

# ── textual: no credential value can reach the snapshot ───────────────────
# Only a boolean capability indicator crosses the server boundary: a
# credential-shaped field carrying anything other than a boolean is a value.
require mcp-app/src/contracts.ts 'githubAuthAvailable: boolean;'
if hits="$(grep -nE "(token|secret|password|authorization)" mcp-app/src/contracts.ts |
  grep -viE ":[[:space:]]*boolean;?$")"; then
  while IFS= read -r hit; do
    echo "::error file=mcp-app/src/contracts.ts::credential-shaped field in a UI contract: ${hit}"
  done <<<"$hits"
  fail=1
fi

# Goose runs on a Copilot token; it must never authorize a GitHub REST call.
forbid_pattern "GITHUB_COPILOT_TOKEN|COPILOT_API_KEY" \
  "a Copilot credential must never be read by the MCP App"

# The documented, and only, GitHub credential precedence.
require mcp-app/src/sources/github.ts \
  'environment.REVIEW_GH_TOKEN' \
  'environment.GH_TOKEN' \
  "const GITHUB_API_BASE_URL = 'https://api.github.com'"

# ── textual: the host bridge pins its outbound origin ─────────────────────
# Inbound messages are validated against window.parent; outbound messages must
# not be broadcast to an arbitrary origin.
if hits="$(grep -rnE "postMessage\(" "$src" | grep -F "'*'")"; then
  while IFS= read -r hit; do
    echo "::error file=${hit%%:*}::postMessage must not target a wildcard origin: ${hit}"
  done <<<"$hits"
  fail=1
fi
require mcp-app/src/ui/main.tsx \
  'this.targetOrigin' \
  'event.source !== this.parent'

# ── artifact: the shipped bundle carries no filesystem import ─────────────
if [[ -f mcp-app/dist/server.js ]] && grep -qF 'node:fs' mcp-app/dist/server.js; then
  echo "::error file=mcp-app/dist/server.js::built server bundle must not import node:fs"
  fail=1
fi

[[ "$fail" -eq 0 ]] && echo "✓ mcp-app read-only contract holds."
exit "$fail"
