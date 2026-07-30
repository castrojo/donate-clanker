# 61-donate-clanker.just — the file an eventual image build would ship at
# /usr/share/ublue-os/just/61-donate-clanker.just (not done yet — see below).
#
# This is the ONLY file that ships/installs. Everything donate-clanker needs
# (VM lifecycle, host CLI detection) is embedded
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
# commands. Persistent state is limited to launcher configuration; the VM
# runner receives no host filesystem mounts.

# Deterministic detection order for every supported CLI. The launcher selects
# the only ready tool silently, or asks the user to choose when several are
# ready.
tool_order := "claude copilot goose codex"

# TOOL is read from the environment so 'TOOL=copilot ujust donate-clanker'
# works as documented — 'just' recipe parameters are positional, not
# KEY=VALUE, so it cannot be a plain recipe parameter.
tool_env := env("TOOL", "")
hive_repo_url := "https://github.com/kubestellar/hive"
# origin/v2 via `git ls-remote --heads https://github.com/kubestellar/hive v2`
# on 2026-07-29.
hive_commit := "e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e"
vm_runner_image := env("DONATE_CLANKER_VM_RUNNER_IMAGE", "")

# Shared bash, 'eval''d at the top of every recipe script that needs it:
# host CLI detection + the selected tool's container mounts. Keeping this in
# one place instead of duplicating it per-recipe is the only concession to
# DRY here — it never leaves the Justfile as a file of its own.
shared_functions := '''
GITHUB_LOGIN_COMMAND="gh auth login --web --hostname github.com --scopes repo,read:org"

github_auth_ready() {
  command -v gh &>/dev/null && gh auth status --hostname github.com &>/dev/null
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
    goose)   [[ -n "${GOOSE_PROVIDER:-}" || -f "${HOME}/.config/goose/config.yaml" ]] ;;
    codex)   [[ -n "${OPENAI_API_KEY:-}" || -d "${HOME}/.codex" ]] ;;
    *)       return 1 ;;
  esac
}
tool_fixit_hint() {
  case "$1" in
    claude)  echo "Run: claude   (then type /login and follow the prompts)" ;;
    copilot) echo "Run: ${GITHUB_LOGIN_COMMAND}" ;;
    goose)   echo "Run: goose configure   (or: export GOOSE_PROVIDER=... GOOSE_MODEL=...)" ;;
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
is_bluefin_dx() {
  [[ "${DONATE_CLANKER_TEST_BLUEFIN_DX:-}" == "1" ]] && return 0
  [[ -r /usr/share/ublue-os/image-info ]] &&
    grep -Eq '^IMAGE_NAME="?bluefin-dx"?$' /usr/share/ublue-os/image-info
}
ensure_vm_runner() {
  command -v podman &>/dev/null || {
    echo "ERROR: Podman is required to run the pinned QEMU VM runner." >&2
    echo "  Install Podman, then re-run donate-clanker." >&2
    return 1
  }
  [[ -e /dev/kvm && -r /dev/kvm && -w /dev/kvm ]] || {
    echo "ERROR: /dev/kvm is unavailable or not usable by this user." >&2
    echo "  Enable KVM access, then re-run donate-clanker (see: donate-clanker-doctor)." >&2
    return 1
  }
}
gum_can_prompt() {
  command -v gum &>/dev/null || return 1
  [[ "${DONATE_CLANKER_TEST_GUM_TTY:-}" == "1" ]] || [[ -t 0 ]]
}
'''

# Start donating clanker cycles to the hive — foreground, Ctrl-C to stop.
# TOOL defaults to the supported auto-detect path: goose, but only when its
# ordered readiness checks pass. Legacy backends remain available only via
# explicit TOOL=<name>.
# Usage: ujust donate-clanker
#        TOOL=goose ujust donate-clanker
donate-clanker:
    #!/usr/bin/env bash
    set -euo pipefail
    {{shared_functions}}
    TOOL_ORDER="{{tool_order}}"
    TOOL="{{tool_env}}"

    STATE_DIR="${HOME}/.local/state/donate-clanker"
    CFG_DIR="${HOME}/.config/donate-clanker"
    HIVE_SRC_DIR="${STATE_DIR}/hive-src"
    HIVE_REPO_URL="{{hive_repo_url}}"
    HIVE_COMMIT="${DONATE_CLANKER_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    mkdir -p "${STATE_DIR}" "${CFG_DIR}"

    # Remembers the last TOOL/provider/model choices made through the interactive
    # pickers below, so re-running donate-clanker offers them back as the
    # pre-selected default instead of starting blank every time.
    LAST_FILE="${CFG_DIR}/last-selections.env"
    LAST_TOOL="" LAST_MODEL_CLAUDE="" LAST_MODEL_COPILOT="" LAST_MODEL_CODEX="" LAST_GOOSE_PROVIDER="" LAST_GOOSE_MODEL=""
    [[ -f "$LAST_FILE" ]] && source "$LAST_FILE"

    # ── 0. Pick TOOL: auto-detect; if exactly one CLI is ready, use it
    #      silently; if several are ready, let the user pick with gum.
    #      An explicit TOOL= always wins and skips detection entirely. ──
    if [[ -z "$TOOL" ]]; then
      READY=()
      for t in ${TOOL_ORDER}; do
        tool_installed "$t" && tool_authenticated "$t" && READY+=("$t")
      done
      case "${#READY[@]}" in
        0)
          echo "ERROR: no supported AI CLI is both installed and authenticated." >&2
          for t in ${TOOL_ORDER}; do echo "  ${t}: $(tool_install_hint "$t")" >&2; done
          echo "Then re-run, or pass explicitly: TOOL=<name> ujust donate-clanker" >&2
          exit 1
          ;;
        1)
          TOOL="${READY[0]}"
          echo "TOOL not set — auto-detected: ${TOOL}"
          ;;
        *)
          if gum_can_prompt; then
            TOOL="$(printf '%s\n' "${READY[@]}" | gum choose --selected="${LAST_TOOL}" --header "Multiple AI CLIs are ready — pick one:")"
            [[ -n "$TOOL" ]] || { echo "ERROR: no selection made." >&2; exit 1; }
          else
            echo "ERROR: multiple CLIs are ready (${READY[*]}) and 'gum' isn't available to prompt." >&2
            echo "  Install gum, or pass explicitly: TOOL=<name> ujust donate-clanker" >&2
            exit 1
          fi
          ;;
      esac
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

    # ── 0b. Optional provider/model, only prompted when not already given
    #        via env and a terminal + gum are available. Blank = use the
    #        tool's own default. Copilot gets a live model list fetched
    #        from the same API its CLI/extensions use. ──
    AGENT_MODEL="${AGENT_MODEL:-}"
    GOOSE_PROVIDER="${GOOSE_PROVIDER:-}"
    GOOSE_MODEL="${GOOSE_MODEL:-}"
    LAST_MODEL_FOR_TOOL=""
    case "$TOOL" in
      claude)  LAST_MODEL_FOR_TOOL="$LAST_MODEL_CLAUDE" ;;
      copilot) LAST_MODEL_FOR_TOOL="$LAST_MODEL_COPILOT" ;;
      codex)   LAST_MODEL_FOR_TOOL="$LAST_MODEL_CODEX" ;;
    esac
    if gum_can_prompt; then
      if [[ "$TOOL" == "goose" && -z "$GOOSE_PROVIDER" ]]; then
        GOOSE_PROVIDER="$(printf '%s\n' anthropic openai google ollama openrouter | gum choose --selected="${LAST_GOOSE_PROVIDER}" --header "Goose provider (Esc to leave unset — needs 'goose configure' instead):")"
      fi
      if [[ "$TOOL" == "goose" ]]; then
        [[ -z "$GOOSE_MODEL" ]] && GOOSE_MODEL="$(gum input --value "${LAST_GOOSE_MODEL}" --placeholder "e.g. claude-3-5-sonnet — blank for goose's default" --header "Goose model (optional):")"
      elif [[ "$TOOL" == "copilot" && -z "$AGENT_MODEL" ]]; then
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
      goose)   LAST_GOOSE_PROVIDER="$GOOSE_PROVIDER"; LAST_GOOSE_MODEL="$GOOSE_MODEL" ;;
    esac
    {
      printf 'LAST_TOOL=%q\n' "$LAST_TOOL"
      printf 'LAST_MODEL_CLAUDE=%q\n' "$LAST_MODEL_CLAUDE"
      printf 'LAST_MODEL_COPILOT=%q\n' "$LAST_MODEL_COPILOT"
      printf 'LAST_MODEL_CODEX=%q\n' "$LAST_MODEL_CODEX"
      printf 'LAST_GOOSE_PROVIDER=%q\n' "$LAST_GOOSE_PROVIDER"
      printf 'LAST_GOOSE_MODEL=%q\n' "$LAST_GOOSE_MODEL"
    } > "$LAST_FILE"

    # Re-derive launcher-managed values from this run instead of retaining
    # stale choices. The selected values reach the guest only as explicit
    # container environment variables below.
    touch "${CFG_DIR}/secrets.env"
    chmod 600 "${CFG_DIR}/secrets.env"
    grep -v -E '^(GOOSE_PROVIDER|GOOSE_MODEL|AGENT_MODEL)=' "${CFG_DIR}/secrets.env" > "${CFG_DIR}/secrets.env.tmp" 2>/dev/null || true
    mv "${CFG_DIR}/secrets.env.tmp" "${CFG_DIR}/secrets.env"
    [[ -n "$AGENT_MODEL" ]] && echo "AGENT_MODEL=${AGENT_MODEL}" >> "${CFG_DIR}/secrets.env"
    if [[ "$TOOL" == "goose" ]]; then
      [[ -n "$GOOSE_PROVIDER" ]] && echo "GOOSE_PROVIDER=${GOOSE_PROVIDER}" >> "${CFG_DIR}/secrets.env"
      [[ -n "$GOOSE_MODEL" ]] && echo "GOOSE_MODEL=${GOOSE_MODEL}" >> "${CFG_DIR}/secrets.env"
    fi
    chmod 600 "${CFG_DIR}/secrets.env"

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
    # ── 2. Run the self-contained QEMU VM in the foreground. The guest
    #        clones assignments itself; no host path or socket crosses this
    #        boundary. exec preserves terminal Ctrl-C semantics. ──
    [[ -n "{{vm_runner_image}}" ]] || {
      echo "ERROR: donate-clanker VM runner is not configured." >&2
      echo "  Set DONATE_CLANKER_VM_RUNNER_IMAGE to the signed immutable runner image reference." >&2
      exit 1
    }
    ensure_vm_runner
    CONTAINER_ARGS=(
      podman run --rm --interactive --tty --name donate-clanker-vm
      --device /dev/kvm
      --env-file "$HIVE_CONTRIBUTOR_ENV"
      --env "AGENT_BACKEND=${TOOL}"
      --env "DONATE_CLANKER_VM=1"
    )
    if [[ -n "$AGENT_MODEL" ]]; then
      CONTAINER_ARGS+=(--env "AGENT_MODEL=${AGENT_MODEL}")
    fi
    if [[ "$TOOL" == "goose" ]]; then
      [[ -n "$GOOSE_PROVIDER" ]] && CONTAINER_ARGS+=(--env "GOOSE_PROVIDER=${GOOSE_PROVIDER}")
      [[ -n "$GOOSE_MODEL" ]] && CONTAINER_ARGS+=(--env "GOOSE_MODEL=${GOOSE_MODEL}")
    fi
    CONTAINER_ARGS+=("{{vm_runner_image}}")
    echo "✓ donate-clanker is running in the pinned QEMU VM runner."
    echo "  Stop any time with Ctrl-C, or: ujust donate-clanker-stop"
    exec "${CONTAINER_ARGS[@]}"

# Preflight check: is this machine actually ready for 'ujust donate-clanker'?
# Never starts the container — read-only diagnostics only.
donate-clanker-doctor:
    #!/usr/bin/env bash
    set -uo pipefail
    {{shared_functions}}
    TOOL_ORDER="{{tool_order}}"
    HIVE_COMMIT="${DONATE_CLANKER_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    INVOKED_DIR="{{invocation_directory()}}"
    pass=0; fail=0
    check() {
      local label="$1"; shift
      if "$@" &>/dev/null; then echo "  ✓ ${label}"; pass=$((pass+1));
      else echo "  ✗ ${label}"; fail=$((fail+1)); fi
    }

    echo "=== QEMU VM runner ==="
    check "Podman installed" command -v podman
    if [[ -e /dev/kvm && -r /dev/kvm && -w /dev/kvm ]]; then
      echo "  ✓ /dev/kvm is available"
      pass=$((pass+1))
    else
      echo "  ✗ /dev/kvm is unavailable or not usable by this user"
      echo "    Enable KVM access before launching the pinned QEMU VM runner."
      fail=$((fail+1))
    fi
    check "gum installed (needed to pick between multiple ready CLIs)" command -v gum
    echo ""

    echo "=== AI CLIs (auto-detect candidates) ==="
    ready_tools=()
    for t in ${TOOL_ORDER}; do
      if tool_installed "$t"; then
        if tool_authenticated "$t"; then
          echo "  ✓ ${t}: installed + authenticated"
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
      0) echo "No CLI is both installed and authenticated yet — TOOL auto-detect would fail." ;;
      1) echo "TOOL would auto-detect to: ${ready_tools[0]}" ;;
      *) echo "Multiple CLIs are ready (${ready_tools[*]}) — you'll be prompted to pick one via gum." ;;
    esac
    echo ""

    echo "=== Guest repository model ==="
    echo "  ✓ assigned repositories are cloned inside the disposable VM"
    echo ""
    echo "${pass} checks passed, ${fail} failed."
    [[ "$fail" -eq 0 ]] || exit 1

# Safety net: remove a stale VM runner container.
donate-clanker-stop:
    #!/usr/bin/env bash
    set -euo pipefail
    if command -v podman &>/dev/null; then
      podman rm -f donate-clanker-vm >/dev/null 2>&1 || true
    fi
    echo "donate-clanker guest container stopped."
