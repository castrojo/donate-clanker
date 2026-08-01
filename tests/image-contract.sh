#!/usr/bin/env bash
# Contract checks for image/ — the half of this repo that actually ships.
#
# These are substring assertions over files, so they are grep, not a test
# framework. They previously lived in image/image_test.go and a bespoke CI
# step, which between them required a whole Go toolchain to check that some
# strings appear in a Containerfile.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

fail=0

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

# forbid <file> <substring>... — no substring may appear.
forbid() {
  local file="$1" unwanted
  shift
  for unwanted in "$@"; do
    grep -qiF -- "$unwanted" "$file" && {
      echo "::error file=${file}::must not contain: ${unwanted}"
      fail=1
    }
  done
  return 0
}

# Digest-pinned, not tagged: the FSDK shell-enabled base is an external image,
# so assert the pinned prefix only and let explicit bumps stay green.
# SC2016: every argument here is a literal to grep for, never an expansion.
# shellcheck disable=SC2016
require image/Containerfile \
  'ARG FSDK_RUNNER_IMAGE=ghcr.io/projectbluefin/lab-runner@sha256:' \
  'ARG HIVE_COMMIT=e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e' \
  'ARG GOOSE_VERSION=' \
  'FROM ${FSDK_RUNNER_IMAGE}' \
  'ARG GOOSE_REFRESH=0' \
  'ARG SKILLS_COMMIT=' \
  'https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz' \
  'https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${gh_arch}.tar.gz' \
  'https://github.com/tmux/tmux-builds/releases/download/v${TMUX_VERSION}/tmux-${TMUX_VERSION}-linux-${tmux_arch}.tar.gz' \
  'https://github.com/aaif-goose/goose/releases/download/v${GOOSE_VERSION}/goose-${goose_arch}-unknown-linux-gnu.tar.gz' \
  'goose_sha=' \
  'goose.tar.gz" | sha256sum -c -;' \
  'npm --prefix /opt/hive install --ignore-scripts ws@8.21.1;' \
  'npm --prefix /opt/hive audit --audit-level=high;' \
  'https://raw.githubusercontent.com/kubestellar/hive/${HIVE_COMMIT}/bin/contributor-agent.sh' \
  'https://raw.githubusercontent.com/kubestellar/hive/${HIVE_COMMIT}/bin/contributor-relay.sh' \
  'https://raw.githubusercontent.com/kubestellar/hive/${HIVE_COMMIT}/config/backends.conf' \
  '/usr/local/bin/goose run --help >/dev/null' \
  'image/config/goose.yaml /opt/bluefin/goose/config/config.yaml' \
  'COPY --chmod=0755 image/git-hooks/ /opt/bluefin/git-hooks/' \
  'https://raw.githubusercontent.com/projectbluefin/common/${SKILLS_COMMIT}/docs/skills/index.json' \
  '--raw-base "https://raw.githubusercontent.com/projectbluefin/common/${SKILLS_COMMIT}/"' \
  '--out /home/dev/.agents/skills' \
  'COPY --chmod=0755 image/entrypoint.sh /usr/local/bin/donate-clanker-entrypoint' \
  'USER dev' \
  'WORKDIR /home/dev' \
  'ENTRYPOINT ["/usr/local/bin/donate-clanker-entrypoint"]'

# No host engine sockets or unrelated runtime glue may enter the image.
forbid image/Containerfile \
  '/var/run/docker.sock' \
  '/run/podman/podman.sock' \
  'https://nodejs.org/dist/latest-v24.x/' \
  'https://github.com/aaif-goose/goose/releases/latest/download/' \
  'https://raw.githubusercontent.com/projectbluefin/common/main/' \
  '# renovate: datasource=github-releases depName=aaif-goose/goose' \
  'ramalama' \
  'models.json' \
  'agent-contract.json'

# The skill generator writes into the image as root. Its source must stay
# commit-pinned and manifest-controlled path components must not escape its
# output root.
require scripts/generate-skills.py \
  'DEFAULT_COMMON_COMMIT =' \
  'SKILL_ID_PATTERN =' \
  'invalid id' \
  'invalid entry_point'
forbid scripts/generate-skills.py \
  'projectbluefin/common/main/'

# The controlled config exists only because Hive overwrites
# ~/.config/goose/config.yaml on every start. It must not pin a provider or a
# model: the launcher passes those through from the contributor's own account.
require image/config/goose.yaml \
  'bundled: false' \
  'type: streamable_http' \
  'name: context7' \
  'enabled: true' \
  'timeout: 30' \
  'uri: "https://mcp.context7.com/mcp"' \
  'GOOSE_MODE: smart_approve' \
  'GOOSE_MAX_TOOL_RESPONSE_SIZE:'

# Comments stripped first, so the prose explaining a setting is never mistaken
# for the setting.
goose_config_body="$(sed 's/#.*//' image/config/goose.yaml)"
for unwanted in GOOSE_PROVIDER GOOSE_MODEL 127.0.0.1:8000 api_key; do
  case "$goose_config_body" in
  *"$unwanted"*)
    echo "::error file=image/config/goose.yaml::must not pin: ${unwanted}"
    fail=1
    ;;
  esac
done

# Agents must not block on Context7: use it opportunistically, fall back local.
require image/config/local-agent-policy.md \
  'inspect local repository evidence first' \
  'Context7 only when current external documentation is useful' \
  'continue with local evidence when Context7 is unavailable'

# GOOSE_PATH_ROOT is the whole reason our config survives Hive's rewrite, and
# Hive's knowledge export lands on CLAUDE.md, which Goose ignores by default.
require image/entrypoint.sh \
  'export GOOSE_PATH_ROOT=' \
  'GOOSE_DISABLE_KEYRING=1' \
  'CONTEXT_FILE_NAMES' \
  'CLAUDE.md' \
  'core.hooksPath /opt/bluefin/git-hooks' \
  'mcp.context7.com' \
  '/usr/local/bin/contributor-agent.sh "$@" &' \
  'tmux has-session -t contributor' \
  'tmux attach-session -t contributor' \
  'tmux kill-session -t contributor'

# ':-/config}' and ':-/workspace}' are mount points Hive never used.
forbid image/entrypoint.sh \
  '/var/run/docker.sock' \
  '/run/podman/podman.sock' \
  ':-/config}' \
  ':-/workspace}'

# Hooks run in every repository via a global core.hooksPath, so they must never
# claim to be enforcement: --no-verify bypasses all of them.
for hook in pre-commit commit-msg post-checkout; do
  head -c 2 "image/git-hooks/${hook}" | grep -q '#!' || {
    echo "::error file=image/git-hooks/${hook}::missing shebang"
    fail=1
  }
  [[ "$hook" == "post-checkout" ]] && continue
  require "image/git-hooks/${hook}" 'no-verify'
done

require image/git-hooks/post-checkout \
  'info/exclude' \
  '.agents/skills/' \
  'docs/skills/index.json'

[[ "$fail" -eq 0 ]] && echo "✓ image contract holds."
exit "$fail"
