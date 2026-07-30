# 61-donate-clanker.just — the file an eventual image build would ship at
# /usr/share/ublue-os/just/61-donate-clanker.just (not done yet — see below).
#
# This is the ONLY file that ships/installs. Everything donate-clanker needs
# (Lima lifecycle, host CLI detection) is embedded
# below as private (_-prefixed) recipes and variables, on purpose: a user
# browsing the image or this repo should find one just-recipe file and the
# three public commands it exposes, not a scattered bin/ of standalone
# scripts they might stumble into and run directly out of context.
#
# Bluefin's root Justfile (/usr/share/ublue-os/just/00-entry.just) imports a
# fixed list of files, NOT a glob. Getting this file under
# /usr/share/ublue-os/just/ so plain 'ujust donate-clanker' works
# system-wide means baking it into a custom image build (out of scope here
# — see README "Scope").
#
# For local/personal use right now, run this file directly with plain
# 'just --justfile', or import it from your own personal Justfile — see
# README "Installing this into your own setup" for the copy-paste
# commands. All state this recipe touches lives under $HOME (Lima state,
# config, workspace) — nothing here needs root or writes elsewhere.

# Production auto-detect only covers the supported backend: GitHub + Goose
# local inference. Legacy backends remain available only via explicit
# TOOL=<name> and are not production-gated.
tool_order := "goose"
legacy_tool_order := "claude copilot codex"

# TOOL/CLANKER_SRC are read from the environment (like upstream hive's own
# Justfile does with HIVE_HUB) so 'TOOL=copilot ujust donate-clanker' works
# as documented — 'just' recipe parameters are positional, not KEY=VALUE,
# so these can't be plain recipe params.
tool_env := env("TOOL", "")
clanker_src_env := env("CLANKER_SRC", "")
hive_repo_url := "https://github.com/kubestellar/hive"
# origin/v2 via `git ls-remote --heads https://github.com/kubestellar/hive v2`
# on 2026-07-29.
hive_commit := "e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e"
compat_image := "ghcr.io/projectbluefin/donate-clanker:stable"

# Shared bash, 'eval''d at the top of every recipe script that needs it:
# host CLI detection + the selected tool's container mounts. Keeping this in
# one place instead of duplicating it per-recipe is the only concession to
# DRY here — it never leaves the Justfile as a file of its own.
shared_functions := '''
GITHUB_LOGIN_COMMAND="gh auth login --web --hostname github.com --scopes repo,read:org"

github_auth_ready() {
  command -v gh &>/dev/null && gh auth status --hostname github.com &>/dev/null
}
env_value() {
  local path="$1" key="$2" line value
  line="$(grep -E "^${key}=" "$path" | head -n1 || true)"
  [[ -n "$line" ]] || return 1
  value="${line#*=}"
  [[ -n "${value//[[:space:]]/}" ]] || return 1
  printf '%s\n' "$value"
}
validate_hive_config_file() {
  local path="$1" key hub backend
  [[ -f "$path" ]] || { echo "missing ${path}" >&2; return 1; }
  for key in HIVE_REGISTRATION_TOKEN HIVE_HUB CONTRIBUTOR_ID CONTRIBUTOR_USERNAME AGENT_BACKEND; do
    env_value "$path" "$key" >/dev/null || { echo "missing required ${key} in ${path}" >&2; return 1; }
  done
  backend="$(env_value "$path" AGENT_BACKEND)" || return 1
  [[ "$backend" == "goose" ]] || { echo "AGENT_BACKEND must be goose in ${path}" >&2; return 1; }
  hub="$(env_value "$path" HIVE_HUB)" || return 1
  [[ "$hub" =~ ^wss://.+/contribute$ ]] || { echo "HIVE_HUB must be a wss:// URL ending in /contribute in ${path}" >&2; return 1; }
}
yaml_value() {
  local path="$1" key="$2"
  awk -F':' -v key="$key" '
    /^[^[:space:]#][^:]*:/ {
      raw=$1
      gsub(/[[:space:]]+/, "", raw)
      if (raw == key) {
        sub(/^[^:]*:[[:space:]]*/, "", $0)
        sub(/[[:space:]]*#.*/, "", $0)
        gsub(/^["'"'"']|["'"'"']$/, "", $0)
        print $0
        exit
      }
    }
  ' "$path"
}
validate_goose_config_file() {
  local path="$1" key provider base_url
  [[ -f "$path" ]] || { echo "missing ${path}" >&2; return 1; }
  for key in provider base_url api_key model; do
    [[ -n "$(yaml_value "$path" "$key")" ]] || { echo "missing required ${key} in ${path}" >&2; return 1; }
  done
  provider="$(yaml_value "$path" provider)"
  [[ "$provider" == "openai" ]] || { echo "provider must be openai in ${path} for the supported local-inference path" >&2; return 1; }
  base_url="$(yaml_value "$path" base_url)"
  [[ "$base_url" =~ ^https?://[^[:space:]]+$ ]] || { echo "base_url must be a valid http(s) endpoint in ${path}" >&2; return 1; }
}
require_supported_goose() {
  local goose_config hive_config
  goose_config="${HOME}/.config/goose/config.yaml"
  hive_config="${HOME}/.config/hive/contributor.env"
  if ! github_auth_ready; then
    echo "ERROR: GitHub authentication is required for supported onboarding." >&2
    echo "  Run: ${GITHUB_LOGIN_COMMAND}" >&2
    return 1
  fi
  if ! command -v goose &>/dev/null; then
    echo "ERROR: Goose is required for supported onboarding." >&2
    echo "  $(tool_install_hint goose)" >&2
    return 1
  fi
  if ! validate_goose_config_file "$goose_config" &>/dev/null; then
    echo "ERROR: Goose local inference is not configured for the supported path." >&2
    echo "  Run: goose configure   (set provider=openai and your local endpoint, API key, and model)" >&2
    return 1
  fi
  if [[ -f "$hive_config" ]] && ! validate_hive_config_file "$hive_config" &>/dev/null; then
    echo "ERROR: Hive contributor setup is incomplete for the supported Goose path." >&2
    echo "  Re-run donate-clanker from an interactive terminal to complete pinned Hive setup." >&2
    return 1
  fi
}
can_run_attended_hive_setup() {
  [[ "${DONATE_CLANKER_TEST_ATTACH_TTY:-}" == "1" ]] || { [[ -t 0 ]] && [[ -t 1 ]] && [[ -t 2 ]]; }
}
print_missing_hive_setup_guidance() {
  local path="$1" reason="$2" tool="$3" commit="$4"
  echo "ERROR: missing Hive setup at ${path}; ${reason}." >&2
  echo "  Re-run donate-clanker from an interactive terminal, or pre-seed it yourself from kubestellar/hive @ ${commit} by running \`just contribute-setup ${tool}\` in an interactive checkout (set DONATE_CLANKER_HIVE_COMMIT to another full commit if needed)" >&2
}
tool_installed() {
  case "$1" in
    claude)  command -v claude &>/dev/null ;;
    copilot) command -v gh &>/dev/null ;;
    goose)   command -v goose &>/dev/null ;;
    codex)   command -v codex &>/dev/null ;;
    *)       return 1 ;;
  esac
}
tool_authenticated() {
  case "$1" in
    claude)  [[ -d "${HOME}/.claude" || -f "${HOME}/.claude.json" ]] ;;
    copilot) gh auth status --hostname github.com &>/dev/null ;;
    goose)   github_auth_ready && validate_goose_config_file "${HOME}/.config/goose/config.yaml" &>/dev/null ;;
    codex)   [[ -n "${OPENAI_API_KEY:-}" || -d "${HOME}/.codex" ]] ;;
    *)       return 1 ;;
  esac
}
tool_fixit_hint() {
  case "$1" in
    claude)  echo "Run: claude   (then type /login and follow the prompts)" ;;
    copilot) echo "Run: ${GITHUB_LOGIN_COMMAND}" ;;
    goose)   echo "Run: goose configure   (set provider=openai and a local base_url, api_key, and model)" ;;
    codex)   echo "Run: codex login   (or: export OPENAI_API_KEY=...)" ;;
    *)       echo "Unknown tool: $1" ;;
  esac
}
tool_install_hint() {
  case "$1" in
    claude)  echo "Install: npm i -g @anthropic-ai/claude-code" ;;
    copilot) echo "Install: see https://cli.github.com" ;;
    goose)   echo "Install: https://github.com/block/goose/releases" ;;
    codex)   echo "Install: npm i -g @openai/codex" ;;
    *)       echo "" ;;
  esac
}
tool_host_dirs() {
  case "$1" in
    claude)  printf '%s\n' "${HOME}/.claude" "${HOME}/.config/claude-code" ;;
    copilot) ;;
    goose)   printf '%s\n' "${HOME}/.config/goose" ;;
    codex)   printf '%s\n' "${HOME}/.codex" ;;
    *)       return 1 ;;
  esac
}
copilot_live_models() {
  # Queries the same model-listing endpoint the Copilot CLI/extensions use,
  # so the picker always reflects what's actually available to this
  # account today rather than a list baked into this file that goes stale.
  # Best-effort: any missing dependency or request failure just returns
  # non-zero so the caller can fall back to free-text input.
  command -v curl &>/dev/null && command -v jq &>/dev/null && command -v gh &>/dev/null || return 1
  local tok
  tok="$(gh auth token 2>/dev/null)" || return 1
  [[ -n "$tok" ]] || return 1
  curl -fsS --max-time 5 \
    -H "Authorization: Bearer ${tok}" \
    -H "Editor-Version: donate-clanker/1.0" \
    -H "Copilot-Integration-Id: vscode-chat" \
    https://api.githubcopilot.com/models 2>/dev/null \
    | jq -r '.data[]? | select(.model_picker_enabled == true and .capabilities.type == "chat") | .id' 2>/dev/null
}
tool_container_mounts() {
  case "$1" in
    claude)
      printf '%s\n' \
        "${HOME}/.claude:/home/dev/.claude:ro" \
        "${HOME}/.config/claude-code:/home/dev/.config/claude-code:ro"
      ;;
    copilot)
      ;;
    goose)
      printf '%s\n' "${HOME}/.config/goose:/home/dev/.config/goose:ro"
      ;;
    codex)
      printf '%s\n' "${HOME}/.codex:/home/dev/.codex:ro"
      ;;
    *)
      echo "ERROR: no container mounts for tool '$1'" >&2
      return 1
      ;;
  esac
}
is_bluefin_dx() {
  [[ "${DONATE_CLANKER_TEST_BLUEFIN_DX:-}" == "1" ]] && return 0
  [[ -r /usr/share/ublue-os/image-info ]] &&
    grep -Eq '^IMAGE_NAME="?bluefin-dx"?$' /usr/share/ublue-os/image-info
}
ensure_lima() {
  command -v limactl &>/dev/null && return 0
  if ! is_bluefin_dx; then
    echo "ERROR: Lima is required to run donate-clanker." >&2
    echo "  Install Lima, then re-run donate-clanker." >&2
    return 1
  fi
  command -v brew &>/dev/null || {
    echo "ERROR: Bluefin DX requires Homebrew to install Lima." >&2
    return 1
  }
  echo "Lima is not installed; installing it with Homebrew..."
  brew install lima
  hash -r
  command -v limactl &>/dev/null || {
    echo "ERROR: Homebrew installed Lima but limactl is not on PATH." >&2
    return 1
  }
}
ensure_lima_vm() {
  local instance="$1" state_dir="$2" workspace="$3" hive_dir="$4" mount_marker desired_mounts mount
  local -a lima_args
  shift 4
  mount_marker="${state_dir}/lima-mounts"
  desired_mounts="$(printf '%s\n' "${workspace}:w" "${hive_dir}:r" "$@")"
  if limactl list --format '{{.Name}}' | grep -Fxq "$instance"; then
    if [[ -f "$mount_marker" ]] && printf '%s' "$desired_mounts" | cmp -s - "$mount_marker"; then
      limactl start "$instance"
      return
    fi
    limactl delete --force "$instance"
  fi
  lima_args=(start --name "$instance" --tty=false
    --mount-only "${workspace}:w"
    --mount-only "${hive_dir}:r")
  for mount in "$@"; do
    lima_args+=(--mount-only "$mount")
  done
  lima_args+=(template:podman)
  limactl "${lima_args[@]}"
  printf '%s' "$desired_mounts" > "$mount_marker"
}
gum_can_prompt() {
  command -v gum &>/dev/null || return 1
  [[ "${DONATE_CLANKER_TEST_GUM_TTY:-}" == "1" ]] || [[ -t 0 ]]
}
tool_supports_launcher_model_override() {
  case "$1" in
    claude|copilot|codex) return 0 ;;
    *)                    return 1 ;;
  esac
}
'''

# Start donating clanker cycles to the hive — foreground, Ctrl-C to stop.
# TOOL defaults to the supported auto-detect path: goose, but only when its
# ordered readiness checks pass. Legacy backends remain available only via
# explicit TOOL=<name>.
# Usage: ujust donate-clanker
#        TOOL=goose ujust donate-clanker
#        CLANKER_SRC=~/src/some-repo ujust donate-clanker
#        CLANKER_SRC=https://github.com/org/repo ujust donate-clanker
donate-clanker:
    #!/usr/bin/env bash
    set -euo pipefail
    {{shared_functions}}
    TOOL_ORDER="{{tool_order}}"
    TOOL="{{tool_env}}"
    CLANKER_SRC="{{clanker_src_env}}"
    # NOT $PWD: just changes the recipe's working directory to wherever the
    # *justfile* lives (e.g. ~ when this is imported from a personal
    # ~/Justfile), not the directory the user actually ran 'just'/'ujust'
    # from. invocation_directory() is just's own builtin for that real cwd.
    INVOKED_DIR="{{invocation_directory()}}"

    STATE_DIR="${HOME}/.local/state/donate-clanker"
    CFG_DIR="${HOME}/.config/donate-clanker"
    WORKSPACE="${STATE_DIR}/workspace"
    CLONES_DIR="${STATE_DIR}/clones"
    HIVE_SRC_DIR="${STATE_DIR}/hive-src"
    LIMA_INSTANCE="donate-clanker"
    HIVE_REPO_URL="{{hive_repo_url}}"
    HIVE_COMMIT="${DONATE_CLANKER_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    mkdir -p "${STATE_DIR}" "${CFG_DIR}" "${CLONES_DIR}"

    # Remembers the last TOOL/model choices made through the interactive
    # pickers below, so re-running donate-clanker offers them back as the
    # pre-selected default instead of starting blank every time.
    LAST_FILE="${CFG_DIR}/last-selections.env"
    LAST_TOOL="" LAST_MODEL_CLAUDE="" LAST_MODEL_COPILOT="" LAST_MODEL_CODEX=""
    [[ -f "$LAST_FILE" ]] && source "$LAST_FILE"

    # ── 0. Pick TOOL: the supported Goose path always runs the same
    #      GitHub -> Goose binary -> Goose config -> existing Hive config
    #      gate. Auto-detect resolves to Goose only after that gate
    #      passes. Unsupported backends remain explicit, manual
    #      compatibility paths and are not production-gated. ──
    if [[ -z "$TOOL" ]]; then
      require_supported_goose || exit 1
      TOOL="goose"
      echo "TOOL not set — auto-detected supported backend: ${TOOL}"
    elif [[ "$TOOL" == "goose" ]]; then
      require_supported_goose || exit 1
    else
      if ! tool_installed "$TOOL"; then
        echo "ERROR: TOOL=${TOOL} requested but not installed." >&2
        echo "  $(tool_install_hint "$TOOL")" >&2
        exit 1
      fi
      if ! tool_authenticated "$TOOL"; then
        echo "ERROR: TOOL=${TOOL} is installed but not ready." >&2
        echo "  $(tool_fixit_hint "$TOOL")" >&2
        exit 1
      fi
    fi

    # ── 0b. Optional model, only prompted when not already given via env
    #        and a terminal + gum are available. Blank = use the tool's
    #        own default. The supported Goose path relies on goose's own
    #        local config, so only the legacy/manual backends prompt here.
    #        copilot gets a live model list fetched from the same API the
    #        Copilot CLI/extensions use, so there's never a model name to
    #        remember or mistype; other tools keep free-text entry.
    #        Whatever is chosen (including staying blank) is remembered as
    #        the next run's default. ──
    AGENT_MODEL="${AGENT_MODEL:-}"
    LAST_MODEL_FOR_TOOL=""
    if ! tool_supports_launcher_model_override "$TOOL"; then
      AGENT_MODEL=""
    fi
    case "$TOOL" in
      claude)  LAST_MODEL_FOR_TOOL="$LAST_MODEL_CLAUDE" ;;
      copilot) LAST_MODEL_FOR_TOOL="$LAST_MODEL_COPILOT" ;;
      codex)   LAST_MODEL_FOR_TOOL="$LAST_MODEL_CODEX" ;;
    esac
    if tool_supports_launcher_model_override "$TOOL" && gum_can_prompt; then
      if [[ "$TOOL" == "copilot" && -z "$AGENT_MODEL" ]]; then
        MODEL_LIST="$(copilot_live_models || true)"
        if [[ -n "$MODEL_LIST" ]]; then
          AGENT_MODEL="$(printf '%s\n' "$MODEL_LIST" | gum choose --selected="${LAST_MODEL_FOR_TOOL}" --header "Model for copilot (Esc to leave unset — use copilot's default):")"
        else
          AGENT_MODEL="$(gum input --value "${LAST_MODEL_FOR_TOOL}" --placeholder "e.g. claude-sonnet-4-6 — blank for copilot's default" --header "Model override for copilot (optional — live list unavailable):")"
        fi
      else
        [[ -z "$AGENT_MODEL" ]] && AGENT_MODEL="$(gum input --value "${LAST_MODEL_FOR_TOOL}" --placeholder "e.g. claude-sonnet-4-6 — blank for ${TOOL}'s default" --header "Model override for ${TOOL} (optional):")"
      fi
    fi

    # Persist this run's resolved choices as next run's defaults.
    LAST_TOOL="$TOOL"
    case "$TOOL" in
      claude)  LAST_MODEL_CLAUDE="$AGENT_MODEL" ;;
      copilot) LAST_MODEL_COPILOT="$AGENT_MODEL" ;;
      codex)   LAST_MODEL_CODEX="$AGENT_MODEL" ;;
    esac
    {
      printf 'LAST_TOOL=%q\n' "$LAST_TOOL"
      printf 'LAST_MODEL_CLAUDE=%q\n' "$LAST_MODEL_CLAUDE"
      printf 'LAST_MODEL_COPILOT=%q\n' "$LAST_MODEL_COPILOT"
      printf 'LAST_MODEL_CODEX=%q\n' "$LAST_MODEL_CODEX"
    } > "$LAST_FILE"

    normalize_git_remote() {
      local value="$1"
      value="${value#ssh://}"
      value="${value%.git}"
      value="${value%/}"
      if [[ "$value" =~ ^git@github\.com:(.+)$ ]]; then
        printf 'https://github.com/%s\n' "${BASH_REMATCH[1]}"
      elif [[ "$value" =~ ^git@github\.com/(.+)$ ]]; then
        printf 'https://github.com/%s\n' "${BASH_REMATCH[1]}"
      else
        printf '%s\n' "$value"
      fi
    }
    prepare_pinned_hive_checkout() {
      local existing_origin actual_commit
      [[ "$HIVE_COMMIT" =~ ^[0-9a-f]{40}$ ]] || {
        echo "ERROR: DONATE_CLANKER_HIVE_COMMIT must be a full 40-character commit SHA; branch names like v2 are not allowed." >&2
        exit 1
      }
      if [[ -d "${HIVE_SRC_DIR}/.git" ]]; then
        existing_origin="$(git -C "$HIVE_SRC_DIR" remote get-url origin 2>/dev/null || true)"
        [[ -n "$existing_origin" ]] || {
          echo "ERROR: ${HIVE_SRC_DIR} is missing an origin remote; move it aside or delete it so donate-clanker can recreate the pinned checkout." >&2
          exit 1
        }
        if [[ "$(normalize_git_remote "$existing_origin")" != "$(normalize_git_remote "$HIVE_REPO_URL")" ]]; then
          echo "ERROR: ${HIVE_SRC_DIR} points at ${existing_origin}, expected ${HIVE_REPO_URL}." >&2
          echo "  Move it aside or delete it so donate-clanker can recreate the pinned checkout." >&2
          exit 1
        fi
        [[ -z "$(git -C "$HIVE_SRC_DIR" status --porcelain 2>/dev/null)" ]] || {
          echo "ERROR: ${HIVE_SRC_DIR} has local changes; refusing to execute an unverified Hive checkout." >&2
          echo "  Use a clean checkout or delete it so donate-clanker can recreate the pinned source." >&2
          exit 1
        }
      else
        if [[ -e "$HIVE_SRC_DIR" && ! -d "$HIVE_SRC_DIR" ]]; then
          echo "ERROR: ${HIVE_SRC_DIR} exists and is not a directory." >&2
          exit 1
        fi
        if [[ -d "$HIVE_SRC_DIR" && -n "$(ls -A "$HIVE_SRC_DIR" 2>/dev/null)" ]]; then
          echo "ERROR: ${HIVE_SRC_DIR} exists but is not a managed git checkout." >&2
          echo "  Move it aside or choose an empty directory before continuing." >&2
          exit 1
        fi
        mkdir -p "$HIVE_SRC_DIR"
        git init --quiet "$HIVE_SRC_DIR"
        git -C "$HIVE_SRC_DIR" remote add origin "$HIVE_REPO_URL"
      fi

      echo "Preparing kubestellar/hive @ ${HIVE_COMMIT:0:12} -> ${HIVE_SRC_DIR}..."
      git -C "$HIVE_SRC_DIR" fetch --depth 1 origin "$HIVE_COMMIT"
      git -C "$HIVE_SRC_DIR" checkout --detach -f FETCH_HEAD
      actual_commit="$(git -C "$HIVE_SRC_DIR" rev-parse HEAD)"
      [[ "$actual_commit" == "$HIVE_COMMIT" ]] || {
        echo "ERROR: expected Hive commit ${HIVE_COMMIT}, got ${actual_commit}." >&2
        exit 1
      }
    }

    # ── 1. Ensure upstream contribute-setup has run ──
    HIVE_CONTRIBUTOR_ENV="${HOME}/.config/hive/contributor.env"
    if [[ ! -f "$HIVE_CONTRIBUTOR_ENV" ]]; then
      if [[ "${DONATE_CLANKER_NON_INTERACTIVE:-}" == "true" ]]; then
        print_missing_hive_setup_guidance "$HIVE_CONTRIBUTOR_ENV" "non-interactive mode cannot answer the upstream prompts" "$TOOL" "$HIVE_COMMIT"
        exit 1
      fi
      if ! can_run_attended_hive_setup; then
        print_missing_hive_setup_guidance "$HIVE_CONTRIBUTOR_ENV" "stdin/stdout/stderr are not attached to a terminal" "$TOOL" "$HIVE_COMMIT"
        exit 1
      fi
      echo "Upstream contribute-setup hasn't run yet (no ${HIVE_CONTRIBUTOR_ENV})."
      for cmd in just gh git; do
        command -v "$cmd" &>/dev/null || { echo "ERROR: '${cmd}' is required to run contribute-setup." >&2; exit 1; }
      done
      prepare_pinned_hive_checkout
      echo "Running upstream pinned setup: just contribute-setup ${TOOL}"
      just --working-directory "$HIVE_SRC_DIR" --justfile "$HIVE_SRC_DIR/Justfile" contribute-setup "$TOOL"
      [[ -f "$HIVE_CONTRIBUTOR_ENV" ]] || { echo "ERROR: contribute-setup ran but ${HIVE_CONTRIBUTOR_ENV} still missing." >&2; exit 1; }
      echo "✓ Upstream contribute-setup complete."
    fi
    if [[ "$TOOL" == "goose" ]] && [[ -f "$HIVE_CONTRIBUTOR_ENV" ]] && ! validate_hive_config_file "$HIVE_CONTRIBUTOR_ENV"; then
      echo "ERROR: upstream Hive setup is incomplete for the supported Goose path." >&2
      echo "  Re-run from kubestellar/hive @ ${HIVE_COMMIT}: just contribute-setup goose" >&2
      exit 1
    fi

    # ── 2. Resolve CLANKER_SRC into a stable workspace symlink ──
    is_git_url() { [[ "$1" =~ ^(https?|ssh|git)://.* || "$1" =~ ^git@.* ]]; }
    if [[ -z "$CLANKER_SRC" ]]; then
      if git -C "$INVOKED_DIR" rev-parse --is-inside-work-tree &>/dev/null; then
        CLANKER_SRC="$INVOKED_DIR"
        echo "CLANKER_SRC not set — defaulting to current repo: ${CLANKER_SRC}"
      elif [[ -L "$WORKSPACE" || -d "$WORKSPACE" ]]; then
        echo "CLANKER_SRC not set — reusing existing workspace at ${WORKSPACE}"
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
          git -C "$clone_dir" pull --ff-only --quiet || echo "WARNING: pull failed, using existing checkout"
        else
          git clone --quiet "$CLANKER_SRC" "$clone_dir"
        fi
        target="$clone_dir"
      else
        target="$(realpath "$CLANKER_SRC")"
        [[ -d "$target" ]] || { echo "ERROR: CLANKER_SRC path does not exist: ${target}" >&2; exit 1; }
      fi
      rm -f "$WORKSPACE"
      ln -s "$target" "$WORKSPACE"
      echo "Workspace -> ${target}"
    fi

    # ── 3. Create or reuse the isolated Lima guest, then attach directly
    #        to its foreground contributor container. ──
    mapfile -t TOOL_HOST_DIRS < <(tool_host_dirs "$TOOL")
    for d in "${TOOL_HOST_DIRS[@]}"; do
      [[ -n "$d" ]] && mkdir -p "$d"
    done
    WORKSPACE_TARGET="$(realpath "$WORKSPACE")"
    ensure_lima
    ensure_lima_vm "$LIMA_INSTANCE" "$STATE_DIR" "$WORKSPACE_TARGET" "${HOME}/.config/hive" \
      "${TOOL_HOST_DIRS[@]/%/:r}"

    CONTAINER_ARGS=(
      podman run --pull=always --rm --interactive --tty
      --userns=keep-id
      --name donate-clanker
      --workdir /workspace
      --volume "${HOME}/.config/hive:/config:ro"
      --volume "${WORKSPACE_TARGET}:/workspace"
      --env "AGENT_BACKEND=${TOOL}"
    )
    while IFS= read -r mount; do
      [[ -n "$mount" ]] && CONTAINER_ARGS+=(--volume "$mount")
    done < <(tool_container_mounts "$TOOL")
    if tool_supports_launcher_model_override "$TOOL" && [[ -n "$AGENT_MODEL" ]]; then
      CONTAINER_ARGS+=(--env "AGENT_MODEL=${AGENT_MODEL}")
    fi
    CONTAINER_ARGS+=("{{compat_image}}")

    echo "✓ donate-clanker is running in Lima VM ${LIMA_INSTANCE}."
    echo "  Stop any time with Ctrl-C, or: ujust donate-clanker-stop"
    exec limactl shell "$LIMA_INSTANCE" -- "${CONTAINER_ARGS[@]}"

# Preflight check: is this machine actually ready for 'ujust donate-clanker'?
# Never starts the container — read-only diagnostics only.
donate-clanker-doctor:
    #!/usr/bin/env bash
    set -uo pipefail
    {{shared_functions}}
    TOOL_ORDER="{{tool_order}}"
    LEGACY_TOOL_ORDER="{{legacy_tool_order}}"
    HIVE_COMMIT="${DONATE_CLANKER_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    INVOKED_DIR="{{invocation_directory()}}"
    pass=0; fail=0
    check() {
      local label="$1"; shift
      if "$@" &>/dev/null; then echo "  ✓ ${label}"; pass=$((pass+1));
      else echo "  ✗ ${label}"; fail=$((fail+1)); fi
    }

    echo "=== Lima runtime ==="
    if command -v limactl &>/dev/null; then
      check "limactl installed" command -v limactl
      check "Lima responds (limactl list)" limactl list
    elif is_bluefin_dx; then
      echo "  ✓ Bluefin DX will install Lima with Homebrew on first launch"
      pass=$((pass+1))
    else
      echo "  ✗ limactl is not installed (install Lima, then retry)"
      fail=$((fail+1))
    fi
    check "gum installed (optional interactive legacy model prompts)" command -v gum
    echo ""

    echo "=== Supported production backend: GitHub + Goose ==="
    check "gh installed" command -v gh
    check "gh authenticated for github.com" github_auth_ready
    check "goose installed" command -v goose
    check "goose local config (~/.config/goose/config.yaml)" validate_goose_config_file "${HOME}/.config/goose/config.yaml"
    check "contribute-setup completed and valid (~/.config/hive/contributor.env)" validate_hive_config_file "${HOME}/.config/hive/contributor.env"
    echo "  Fix GitHub auth with: ${GITHUB_LOGIN_COMMAND}"
    echo "  Fix Goose local config with: goose configure   (provider=openai, local base_url/api_key/model)"
    echo "  Fix Hive setup by re-running donate-clanker (auto-fetches kubestellar/hive @ ${HIVE_COMMIT}),"
    echo "    or manually run just contribute-setup goose from that exact commit."
    echo ""

    echo "=== Supported auto-detect ==="
    ready_tools=()
    for t in ${TOOL_ORDER}; do
      if tool_installed "$t"; then
        if tool_authenticated "$t"; then
          echo "  ✓ ${t}: installed + GitHub-authenticated + local Goose config valid"
          ready_tools+=("$t")
        else
          echo "  ~ ${t}: installed, NOT ready — $(tool_fixit_hint "$t")"
        fi
      else
        echo "  ✗ ${t}: not installed — $(tool_install_hint "$t")"
      fi
    done
    echo ""

    case "${#ready_tools[@]}" in
      0) echo "Goose is not fully ready yet — TOOL auto-detect would fail." ;;
      1) echo "TOOL would auto-detect to the supported backend: ${ready_tools[0]}" ;;
      *) echo "Unexpected: multiple supported backends are ready (${ready_tools[*]})." ;;
    esac
    echo ""

    echo "=== Legacy compatibility backends (manual only; unsupported for production) ==="
    for t in ${LEGACY_TOOL_ORDER}; do
      if tool_installed "$t"; then
        if tool_authenticated "$t"; then
          echo "  ~ ${t}: installed + ready (manual TOOL=${t} only)"
        else
          echo "  ~ ${t}: installed, NOT ready — $(tool_fixit_hint "$t")"
        fi
      else
        echo "  ✗ ${t}: not installed — $(tool_install_hint "$t")"
      fi
    done
    echo ""

    echo "=== Source (CLANKER_SRC) ==="
    WORKSPACE="${HOME}/.local/state/donate-clanker/workspace"
    if [[ -n "${CLANKER_SRC:-}" ]]; then
      echo "  CLANKER_SRC is set: ${CLANKER_SRC}"
    elif git -C "$INVOKED_DIR" rev-parse --is-inside-work-tree &>/dev/null; then
      echo "  CLANKER_SRC not set — current directory is a git repo, would default to: ${INVOKED_DIR}"
    elif [[ -L "$WORKSPACE" ]]; then
      echo "  CLANKER_SRC not set — would reuse existing workspace: $(readlink -f "$WORKSPACE" 2>/dev/null || echo "$WORKSPACE")"
    else
      echo "  ✗ CLANKER_SRC not set, current directory isn't a git repo, and no prior workspace exists."
      echo "    First run will fail unless you either 'cd' into the repo you want to donate against,"
      echo "    or run: CLANKER_SRC=<path-or-git-url> ujust donate-clanker"
    fi
    echo ""
    echo "${pass} checks passed, ${fail} failed."
    [[ "$fail" -eq 0 ]] || exit 1

# Safety net: remove a stale guest container without destroying the reusable
# Lima VM.
donate-clanker-stop:
    #!/usr/bin/env bash
    set -euo pipefail
    if command -v limactl &>/dev/null && limactl list --format '{{{{.Name}}}}' | grep -Fxq donate-clanker; then
      limactl shell donate-clanker -- podman rm -f donate-clanker >/dev/null 2>&1 || true
    fi
    echo "donate-clanker guest container stopped."
