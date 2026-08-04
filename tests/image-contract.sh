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
  'ARG GOOSE_CHANNEL=canary' \
  'FROM ${FSDK_RUNNER_IMAGE}' \
  'ARG GOOSE_REFRESH=0' \
  'ARG SKILLS_COMMIT=' \
  'https://nodejs.org/dist/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${node_arch}.tar.xz' \
  'https://github.com/cli/cli/releases/download/v${GH_VERSION}/gh_${GH_VERSION}_linux_${gh_arch}.tar.gz' \
  'https://github.com/tmux/tmux-builds/releases/download/v${TMUX_VERSION}/tmux-${TMUX_VERSION}-linux-${tmux_arch}.tar.gz' \
  'https://github.com/aaif-goose/goose/releases/download/${GOOSE_CHANNEL}/goose-${goose_arch}-unknown-linux-gnu.tar.gz' \
  'RUN --mount=type=secret,id=github_token' \
  'GH_TOKEN="$(cat /run/secrets/github_token)"' \
  'gh attestation verify "$workdir/goose.tar.gz" --repo aaif-goose/goose --signer-workflow aaif-goose/goose/.github/workflows/canary.yml' \
  'tf.extractall(root, filter="data")' \
  'npm --prefix /opt/hive install --ignore-scripts ws@8.21.1;' \
  'https://raw.githubusercontent.com/kubestellar/hive/${HIVE_COMMIT}/bin/contributor-agent.sh' \
  'https://raw.githubusercontent.com/kubestellar/hive/${HIVE_COMMIT}/bin/contributor-relay.sh' \
  'https://raw.githubusercontent.com/kubestellar/hive/${HIVE_COMMIT}/config/backends.conf' \
  '/usr/local/bin/goose run --help >/dev/null' \
  'image/config/goose.yaml /opt/bluefin/goose/config/config.yaml' \
  'COPY --chmod=0755 image/git-hooks/ /opt/bluefin/git-hooks/' \
  'COPY --chmod=0755 image/hive-entrypoint.d/ /etc/hive/entrypoint.d/' \
  'COPY --chmod=0755 image/bin/cmp /usr/local/bin/cmp' \
  'COPY --chmod=0755 image/bin/find /usr/local/bin/find' \
  'COPY --chmod=0755 image/bin/bluefin-review /usr/local/bin/bluefin-review' \
  'COPY image/tmux.conf /etc/tmux.conf' \
  'image/terminfo/xterm-256color.src /tmp/xterm-256color.src' \
  'tic -x -o /usr/share/terminfo /tmp/xterm-256color.src' \
  'image/terminfo/tmux-256color.src /tmp/tmux-256color.src' \
  'tic -x -o /usr/share/terminfo /tmp/tmux-256color.src' \
  'https://raw.githubusercontent.com/projectbluefin/common/${SKILLS_COMMIT}/docs/skills/index.json' \
  '--raw-base "https://raw.githubusercontent.com/projectbluefin/common/${SKILLS_COMMIT}/"' \
  '--out /home/dev/.agents/skills' \
  'COPY --chmod=0755 scripts/generate-skills.py /usr/local/libexec/review-generate-skills' \
  'COPY --chmod=0755 image/entrypoint.sh /usr/local/bin/review-entrypoint' \
  'USER dev' \
  'WORKDIR /home/dev' \
  'ENTRYPOINT ["/usr/local/bin/review-entrypoint"]'

# Host setup and the image relay exchange the same contributor protocol, so
# their pinned Hive revisions must remain exactly aligned.
launcher_hive_pin="$(sed -n 's/^hive_commit := "\([0-9a-f]\{40\}\)"$/\1/p' justfile)"
image_hive_pin="$(sed -n 's/^ARG HIVE_COMMIT=\([0-9a-f]\{40\}\)$/\1/p' image/Containerfile)"
if [[ -z "$launcher_hive_pin" || "$launcher_hive_pin" != "$image_hive_pin" ]]; then
  echo "::error::launcher and image Hive pins must match"
  fail=1
fi

# No host engine sockets or unrelated runtime glue may enter the image.
forbid image/Containerfile \
  '/var/run/docker.sock' \
  '/run/podman/podman.sock' \
  'https://nodejs.org/dist/latest-v24.x/' \
  'https://github.com/aaif-goose/goose/releases/latest/download/' \
  'https://raw.githubusercontent.com/projectbluefin/common/main/' \
  'npm --prefix /opt/hive audit' \
  '# renovate: datasource=github-releases depName=aaif-goose/goose' \
  'ramalama' \
  'models.json' \
  'agent-contract.json'

# shellcheck disable=SC2016 # Literal source assertion, not shell expansion.
require image/hive-entrypoint.d/hosted-knowledge.sh \
  'hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io' \
  '/api/v1/knowledge' \
  'Authorization: Bearer ${GH_TOKEN}'

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
# ~/.config/goose/config.yaml on every start. It must not pin a provider,
# model, or extension that Hive manages: the launcher passes provider and model
# through from the contributor's own account.
require image/config/goose.yaml \
  'GOOSE_MODE: auto' \
  'GOOSE_MAX_TOOL_RESPONSE_SIZE:'
forbid image/config/goose.yaml \
  'context7'

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

require image/config/local-agent-policy.md \
  'Use installed global Agent Skills when their descriptions match the task' \
  'docs/skills/index.json' \
  'inspect local repository evidence first'
forbid image/config/local-agent-policy.md \
  'context7'

# GOOSE_PATH_ROOT is the whole reason our config survives Hive's rewrite, and
# Hive's knowledge export lands on CLAUDE.md, which Goose ignores by default.
# shellcheck disable=SC2016 # Literal source assertions, not shell expansions.
require image/entrypoint.sh \
  'export GOOSE_PATH_ROOT=' \
  "note() { printf 'review: %s\\n' \"\$1\" >&2; }" \
  '[ "$GOOSE_PROVIDER" != github_copilot ]' \
  'review supports GitHub Copilot only.' \
  'export GOOSE_PROVIDER=github_copilot' \
  'GOOSE_MODEL="gpt-5.6-luna"' \
  'GOOSE_THINKING_EFFORT="${GOOSE_THINKING_EFFORT:-high}"' \
  'GOOSE_DISABLE_KEYRING=1' \
  'CONTEXT_FILE_NAMES' \
  'CLAUDE.md' \
  'GOOSE_MOIM_MESSAGE_FILE' \
  '/opt/bluefin/local-agent-policy.md' \
  'core.hooksPath /opt/bluefin/git-hooks' \
  'shopt -s nullglob' \
  'validation_tools=(bats shellcheck systemd-analyze pre-commit just podman)' \
  'validation tools unavailable:' \
  'tmux_fallback_term=xterm-256color' \
  'infocmp "${TERM:-}"' \
  'TERM=${TERM:-<unset>} has no terminfo; using ${tmux_fallback_term}' \
  '/usr/local/bin/contributor-agent.sh "$@" &' \
  'tmux has-session -t contributor' \
  'tmux readiness diagnostics' \
  'tmux attach-session -t contributor' \
  'trap cleanup EXIT HUP INT TERM' \
  "note 'tmux detached; the agent remains foreground in this terminal. Press Ctrl-C or close this terminal to stop it.'" \
  'wait "$agent_pid"' \
  'tmux kill-session -t contributor'
forbid image/entrypoint.sh \
  'context7' \
  'mcp.context7.com'

require README.md \
  'Goose canary snapshot' \
  'not byte-reproducible' \
  "--build-arg GOOSE_REFRESH=\"\$(date +%s)\""

require .github/workflows/validate.yml \
  'attestations: read' \
  'just --justfile justfile --list' \
  "' justfile > recipe_bodies.sh" \
  'docker build --secret id=github_token,env=GITHUB_TOKEN' \
  '-f image/Containerfile -t review:test .' \
  '["/usr/local/bin/review-entrypoint"]'

require .github/workflows/publish-compat-image.yml \
  'attestations: read' \
  'IMAGE: ghcr.io/projectbluefin/review' \
  'tags: review:smoke' \
  '["/usr/local/bin/review-entrypoint"]' \
  'secrets: |' \
  "github_token=\${{ secrets.GITHUB_TOKEN }}"

# ':-/config}' and ':-/workspace}' are mount points Hive never used.
forbid image/entrypoint.sh \
  '/var/run/docker.sock' \
  '/run/podman/podman.sock' \
  ':-/config}' \
  ':-/workspace}' \
  'tmux_term=xterm-256color'

require image/tmux.conf \
  'set -g default-terminal "tmux-256color"' \
  'set -g mouse on'

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
