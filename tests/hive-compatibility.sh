#!/usr/bin/env bash
# Validate every compatibility conclusion against the exact Hive revision the
# launcher and image use. This is intentionally source-backed: these seams are
# upstream protocol behavior, not local reimplementations.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

pin="$(sed -n 's/^ARG HIVE_COMMIT=\([0-9a-f]\{40\}\)$/\1/p' image/Containerfile)"
[[ "$pin" =~ ^[0-9a-f]{40}$ ]] || {
  echo "::error file=image/Containerfile::HIVE_COMMIT must be a full SHA" >&2
  exit 1
}

hive_source() {
  curl --fail --location --silent --show-error \
    "https://raw.githubusercontent.com/kubestellar/hive/${pin}/$1"
}

agent="$(hive_source bin/contributor-agent.sh)"
backends="$(hive_source config/backends.conf)"

# Hive now exposes its refreshed knowledge through the filenames Goose reads
# natively, so a downstream CONTEXT_FILE_NAMES extension would be redundant.
# shellcheck disable=SC2016 # Exact pinned-source fragments, not shell syntax.
for link in \
  'ln -sf "$AGENT_MD" "${HOME}/AGENTS.md"' \
  'ln -sf "$AGENT_MD" "${HOME}/.goosehints"'; do
  grep -qF "$link" <<<"$agent" || {
    echo "::error::pinned Hive no longer creates Goose-native knowledge link: $link" >&2
    exit 1
  }
done
# shellcheck disable=SC2016 # Exact pinned-source fragment, not shell syntax.
grep -qF 'if [ ! -f "${HOME}/.config/goose/config.yaml" ]; then' <<<"$agent" || {
  echo "::error::pinned Hive no longer preserves an existing Goose config" >&2
  exit 1
}

# contributor-agent.sh sources this full upstream interface before detection;
# shrinking it locally would make review own Hive backend behavior.
grep -qF 'source /usr/local/etc/hive/backends.conf' <<<"$agent" || {
  echo "::error::pinned Hive no longer consumes the installed backends.conf" >&2
  exit 1
}
grep -qF 'KNOWN_BACKENDS="claude copilot goose codex agy bob pi aider litellm"' <<<"$backends" || {
  echo "::error::pinned Hive backend interface changed" >&2
  exit 1
}

# Exercise the hook with an inert command after it has installed its wrapper.
# The exact hosted URL is rewritten and receives a Bearer token; unrelated
# curl calls retain their original arguments.
hook_output="$(
  HIVE_HUB='wss://hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io/contribute' \
    GH_TOKEN='compatibility-test-token' \
    bash -c '
      source image/hive-entrypoint.d/hosted-knowledge.sh
      curl_binary=/bin/echo
      curl -sf "https://hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io/api/knowledge/export" -o /dev/null
    '
)"
[[ "$hook_output" == *'--header Authorization: Bearer compatibility-test-token'* ]] &&
  [[ "$hook_output" == *'/api/v1/knowledge'* ]] ||
  {
    echo "::error::hosted knowledge hook did not authenticate and rewrite the stock export" >&2
    exit 1
  }

if HIVE_HUB='wss://other.hive.example/contribute' GH_TOKEN='compatibility-test-token' \
  bash -c 'source image/hive-entrypoint.d/hosted-knowledge.sh; declare -F curl' |
  grep -q .; then
  echo "::error::hosted knowledge hook must not intercept other Hive deployments" >&2
  exit 1
fi

grep -qF 'export GOOSE_PATH_ROOT=' image/entrypoint.sh
if grep -qF 'CONTEXT_FILE_NAMES' image/entrypoint.sh; then
  echo "::error::entrypoint retains an obsolete context filename override" >&2
  exit 1
fi

echo "✓ pinned Hive compatibility seams hold."
