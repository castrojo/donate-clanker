#!/usr/bin/env bash
# donate-clanker-bootstrap.sh — idempotently install/refresh the Podman
# Quadlet unit, the selected tool drop-in, the workspace symlink/clone, and
# the env file for `ujust donate-clanker`.
#
# Safe to re-run any number of times: every step is "write only if
# different" or "resolve and re-point", never additive.
#
# Inputs (env vars, all optional):
#   TOOL         claude|copilot|<name matching quadlet/tools/<name>.conf>  (default: claude)
#   CLANKER_SRC  local path OR git URL for the workspace  (default: $PWD if it's a
#                git repo, else the persistent clone dir from the last run)
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONF_DIR="${HOME}/.config/containers/systemd"
DROPIN_DIR="${CONF_DIR}/donate-clanker.container.d"
STATE_DIR="${HOME}/.local/state/donate-clanker"
CFG_DIR="${HOME}/.config/donate-clanker"
WORKSPACE="${STATE_DIR}/workspace"
CLONES_DIR="${STATE_DIR}/clones"

TOOL="${TOOL:-claude}"
CLANKER_SRC="${CLANKER_SRC:-}"

mkdir -p "${CONF_DIR}" "${DROPIN_DIR}" "${STATE_DIR}" "${CFG_DIR}" "${CLONES_DIR}"

# ── 1. Install/refresh the base quadlet unit (write only if changed) ──
install_if_changed() {
  local src="$1" dst="$2"
  if [[ ! -f "$dst" ]] || ! cmp -s "$src" "$dst"; then
    install -m 0644 "$src" "$dst"
    echo "Installed $(basename "$dst")"
  fi
}
install_if_changed "${REPO_ROOT}/quadlet/donate-clanker.container" "${CONF_DIR}/donate-clanker.container"

# ── 2. Select exactly one tool fragment (no accidental fragment merging) ──
TOOL_SRC="${REPO_ROOT}/quadlet/tools/${TOOL}.conf"
if [[ ! -f "$TOOL_SRC" ]]; then
  echo "ERROR: no tool fragment for TOOL=${TOOL} (looked for ${TOOL_SRC})" >&2
  echo "Available: $(cd "${REPO_ROOT}/quadlet/tools" && ls -- *.conf | sed 's/\.conf$//' | tr '\n' ' ')" >&2
  exit 1
fi
# Remove any previously-selected fragment so only one is ever active.
find "${DROPIN_DIR}" -maxdepth 1 -name '10-tool.conf' -delete
install -m 0644 "$TOOL_SRC" "${DROPIN_DIR}/10-tool.conf"

# ── 3. Resolve CLANKER_SRC into a stable ~/.local/state/donate-clanker/workspace ──
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

# ── 4. Env file for the quadlet (created once; never overwritten) ──
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

# ── 5. Reload the user systemd instance so the generator picks up changes ──
systemctl --user daemon-reload
