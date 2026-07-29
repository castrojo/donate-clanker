#!/usr/bin/env bash
# donate-clanker-bootstrap.sh — idempotently install/refresh the Podman
# Quadlet unit, the selected tool drop-in, the workspace symlink/clone, and
# the env file for `ujust donate-clanker`.
#
# Safe to re-run any number of times: every step is "write only if
# different" or "resolve and re-point", never additive.
#
# Inputs (env vars, all optional):
#   TOOL         claude|copilot|goose|<name matching quadlet/tools/<name>.conf>
#                (default: auto-detect the first installed+authenticated CLI,
#                in the order defined by lib-detect.sh)
#   CLANKER_SRC  local path OR git URL for the workspace  (default: $PWD if it's a
#                git repo, else the persistent clone dir from the last run)
#   HIVE_SETUP_REPO  override for where to clone kubestellar/hive when
#                     upstream `contribute-setup` needs to run (default: v2
#                     branch of https://github.com/kubestellar/hive)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=./lib-detect.sh
source "${REPO_ROOT}/bin/lib-detect.sh"

CONF_DIR="${HOME}/.config/containers/systemd"
DROPIN_DIR="${CONF_DIR}/donate-clanker.container.d"
STATE_DIR="${HOME}/.local/state/donate-clanker"
CFG_DIR="${HOME}/.config/donate-clanker"
WORKSPACE="${STATE_DIR}/workspace"
CLONES_DIR="${STATE_DIR}/clones"
HIVE_SRC_DIR="${STATE_DIR}/hive-src"
HIVE_SETUP_REPO="${HIVE_SETUP_REPO:-https://github.com/kubestellar/hive}"

TOOL="${TOOL:-}"
CLANKER_SRC="${CLANKER_SRC:-}"

mkdir -p "${CONF_DIR}" "${DROPIN_DIR}" "${STATE_DIR}" "${CFG_DIR}" "${CLONES_DIR}"

# ── 0. Pick TOOL: explicit flag wins, else auto-detect, else fail with a
#      list of what to install. This replaces the old silent "claude"
#      default, which broke for anyone without Claude Code installed. ──
if [[ -z "$TOOL" ]]; then
  if TOOL="$(detect_first_ready_tool)"; then
    echo "TOOL not set — auto-detected: ${TOOL}"
  else
    echo "ERROR: no supported CLI is installed and authenticated on this host." >&2
    echo "Install and authenticate one of:" >&2
    for t in "${DONATE_CLANKER_TOOL_ORDER[@]}"; do
      echo "  ${t}: $(tool_install_hint "$t")" >&2
    done
    echo "Then re-run, or pass explicitly: TOOL=<name> ujust donate-clanker" >&2
    exit 1
  fi
else
  if ! tool_installed "$TOOL"; then
    echo "ERROR: TOOL=${TOOL} requested but not installed." >&2
    echo "  $(tool_install_hint "$TOOL")" >&2
    exit 1
  fi
  if ! tool_authenticated "$TOOL"; then
    echo "ERROR: TOOL=${TOOL} is installed but not authenticated." >&2
    echo "  $(tool_fixit_hint "$TOOL")" >&2
    exit 1
  fi
fi
# Record the resolved tool so the calling `just` recipe (a separate process)
# can report it back to the user without re-running detection itself.
echo -n "$TOOL" > "${STATE_DIR}/resolved-tool"

# ── 1. Ensure upstream `contribute-setup` has run (registration + gh auth +
#      CLI auth, written to ~/.config/hive/{contributor.env,gh-auth.env}).
#      We never reimplement that logic ourselves — we just make sure it has
#      run, by shelling out to kubestellar/hive's own Justfile, exactly as
#      the contribute page instructs. ──
HIVE_CONTRIBUTOR_ENV="${HOME}/.config/hive/contributor.env"
if [[ ! -f "$HIVE_CONTRIBUTOR_ENV" ]]; then
  echo "Upstream contribute-setup hasn't run yet (no ${HIVE_CONTRIBUTOR_ENV})."
  for cmd in just gh git; do
    command -v "$cmd" &>/dev/null || { echo "ERROR: '${cmd}' is required to run contribute-setup." >&2; exit 1; }
  done
  if [[ -d "${HIVE_SRC_DIR}/.git" ]]; then
    echo "Updating cached kubestellar/hive clone..."
    git -C "$HIVE_SRC_DIR" pull --ff-only --quiet || echo "WARNING: pull failed, using existing checkout"
  else
    echo "Cloning ${HIVE_SETUP_REPO} (branch v2) -> ${HIVE_SRC_DIR}..."
    git clone --quiet -b v2 "$HIVE_SETUP_REPO" "$HIVE_SRC_DIR"
  fi
  echo "Running upstream: just contribute-setup ${TOOL}"
  ( cd "$HIVE_SRC_DIR" && just contribute-setup "$TOOL" )
  if [[ ! -f "$HIVE_CONTRIBUTOR_ENV" ]]; then
    echo "ERROR: contribute-setup ran but ${HIVE_CONTRIBUTOR_ENV} still doesn't exist." >&2
    exit 1
  fi
  echo "✓ Upstream contribute-setup complete."
fi

# ── 2. Install/refresh the base quadlet unit (write only if changed) ──
install_if_changed() {
  local src="$1" dst="$2"
  if [[ ! -f "$dst" ]] || ! cmp -s "$src" "$dst"; then
    install -m 0644 "$src" "$dst"
    echo "Installed $(basename "$dst")"
  fi
}
install_if_changed "${REPO_ROOT}/quadlet/donate-clanker.container" "${CONF_DIR}/donate-clanker.container"

# ── 3. Select exactly one tool fragment (no accidental fragment merging) ──
TOOL_SRC="${REPO_ROOT}/quadlet/tools/${TOOL}.conf"
if [[ ! -f "$TOOL_SRC" ]]; then
  echo "ERROR: no tool fragment for TOOL=${TOOL} (looked for ${TOOL_SRC})" >&2
  echo "Available: $(cd "${REPO_ROOT}/quadlet/tools" && ls -- *.conf | sed 's/\.conf$//' | tr '\n' ' ')" >&2
  exit 1
fi
# Remove any previously-selected fragment so only one is ever active.
find "${DROPIN_DIR}" -maxdepth 1 -name '10-tool.conf' -delete
install -m 0644 "$TOOL_SRC" "${DROPIN_DIR}/10-tool.conf"

# ── 4. Resolve CLANKER_SRC into a stable ~/.local/state/donate-clanker/workspace ──
is_git_url() {
  [[ "$1" =~ ^(https?|ssh|git)://.* || "$1" =~ ^git@.* ]]
}

if [[ -z "$CLANKER_SRC" ]]; then
  if git -C "$PWD" rev-parse --is-inside-work-tree &>/dev/null; then
    CLANKER_SRC="$PWD"
    echo "CLANKER_SRC not set — defaulting to current repo: ${CLANKER_SRC}"
  elif [[ -L "$WORKSPACE" || -d "$WORKSPACE" ]]; then
    echo "CLANKER_SRC not set — reusing existing workspace at ${WORKSPACE}"
    CLANKER_SRC=""  # leave workspace untouched below
  else
    echo "ERROR: CLANKER_SRC not set and no prior workspace exists." >&2
    echo "  Run from a git repo, or: CLANKER_SRC=<path-or-git-url> ujust donate-clanker" >&2
    exit 1
  fi
fi

if [[ -n "$CLANKER_SRC" ]]; then
  if is_git_url "$CLANKER_SRC"; then
    slug="$(basename "${CLANKER_SRC%.git}")"
    clone_dir="${CLONES_DIR}/${slug}"
    if [[ -d "${clone_dir}/.git" ]]; then
      echo "Updating existing clone: ${clone_dir}"
      git -C "$clone_dir" pull --ff-only --quiet || echo "WARNING: pull failed, using existing checkout"
    else
      echo "Cloning ${CLANKER_SRC} -> ${clone_dir}"
      git clone --quiet "$CLANKER_SRC" "$clone_dir"
    fi
    target="$clone_dir"
  else
    target="$(realpath "$CLANKER_SRC")"
    if [[ ! -d "$target" ]]; then
      echo "ERROR: CLANKER_SRC path does not exist: ${target}" >&2
      exit 1
    fi
  fi
  rm -f "$WORKSPACE"        # symlink or stale file; clone dirs live under $CLONES_DIR, never overwritten
  ln -s "$target" "$WORKSPACE"
  echo "Workspace -> ${target}"
fi

# ── 5. Env file for the quadlet (created once; never overwritten) ──
ENV_FILE="${CFG_DIR}/contributor.env"
if [[ ! -f "$ENV_FILE" ]]; then
  cat > "$ENV_FILE" <<'EOF'
# donate-clanker container environment.
# Populated once; edit freely — the bootstrap script never overwrites this file.
# HIVE_HUB=wss://your-hive.example.com/contribute
EOF
  echo "Created ${ENV_FILE} (edit HIVE_HUB if needed)"
fi
touch "${CFG_DIR}/secrets.env"  # optional, gitignored-by-convention secrets overlay
chmod 600 "${CFG_DIR}/secrets.env"

# goose has no login flow — its provider/model selection lives in the host
# environment. Mirror just the two vars it needs into secrets.env (never the
# whole environment — see quadlet/tools/goose.conf for why).
if [[ "$TOOL" == "goose" ]]; then
  grep -v -E '^(GOOSE_PROVIDER|GOOSE_MODEL)=' "${CFG_DIR}/secrets.env" > "${CFG_DIR}/secrets.env.tmp" 2>/dev/null || true
  mv "${CFG_DIR}/secrets.env.tmp" "${CFG_DIR}/secrets.env"
  [[ -n "${GOOSE_PROVIDER:-}" ]] && echo "GOOSE_PROVIDER=${GOOSE_PROVIDER}" >> "${CFG_DIR}/secrets.env"
  [[ -n "${GOOSE_MODEL:-}" ]] && echo "GOOSE_MODEL=${GOOSE_MODEL}" >> "${CFG_DIR}/secrets.env"
  chmod 600 "${CFG_DIR}/secrets.env"
fi

# ── 6. Reload the user systemd instance so the generator picks up changes ──
systemctl --user daemon-reload
