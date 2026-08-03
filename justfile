# justfile — the local review launcher entrypoint.
#
# The system image install path is still out of scope here; this root justfile
# is the foreground launcher a checkout exposes directly.
#
# This is the ONLY file that ships/installs. Everything review needs
# (host preflight, Goose selection, VM lifecycle, container lifecycle) is
# embedded below as private ('_'-prefixed variables and shared shell
# functions) on purpose: a user browsing the image or this repo should find
# one just-recipe file and the commands it exposes, not a scattered bin/ of
# standalone scripts they might stumble into and run directly out of context.
#
# Public commands:
#   review            Boot the pinned QEMU VM in the FOREGROUND and
#                     hand the terminal to the contributor agent.
#   review-container  Run only the contributor container — no VM —
#                     for quick local development. Also foreground.
#   review-doctor     Read-only preflight diagnostics. Starts nothing.
#
# ─────────────────────────────────────────────────────────────────────────
# FOREGROUND GUARANTEE
#
# The VM and the container ALWAYS run in the foreground of the terminal that
# launched them. There is no '&', no 'nohup', no 'setsid', no 'podman run
# -d', and no '--detach' on any launch path in this file, and there never
# should be. Two reasons, both non-negotiable:
#
#   1. A human must be able to steer the agent. review is an
#      attended tool: you watch the session, you interrupt it, you type into
#      it. A backgrounded agent is an agent nobody is supervising.
#   2. Ctrl-C must stop it. Signals have to reach the QEMU process or the
#      container directly, so the run dies with the terminal. No daemon, no
#      systemd unit, no orphaned state to reap later.
#
# It follows that there is NO stop command, and there must never be one.
# Shipping 'just review-stop' would be an admission that a run can outlive the
# terminal that started it — the exact property this file exists to prevent.
# Ctrl-C is the stop button. A run that needs a second command to end it is a
# daemon wearing a disguise, and this repository does not ship daemons.
# Cleanup is therefore a startup concern, never a user-facing verb: a launch
# reclaims whatever a previous run left behind.
#
# '--replace' is how that reclaim works, and it is not a hole in the
# guarantee: --rm removes the container when it exits cleanly, but a
# hard-killed terminal, an OOM kill or a podman restart can leave the fixed
# name behind, and the next launch would otherwise die with 'the container
# name ... is already in use'. --replace takes the name back at launch time
# instead of asking the user to run a lifecycle command.
#
# Concretely: the final process of every launch path is either 'exec'd (so
# it replaces this shell and inherits its signals) or is the last foreground
# command whose exit status is propagated verbatim. The only background job
# in this file is the short-lived, host-local bootstrap socket server, which
# is reaped by an EXIT/INT/TERM trap and never outlives the run.
# tests/just-onboarding.sh enforces this by grepping the launch lines.
# ─────────────────────────────────────────────────────────────────────────
#
# Bluefin's root Justfile (/usr/share/ublue-os/just/00-entry.just) imports a
# fixed list of files, NOT a glob. Making 'just review' work system-wide from
# the image still means baking this launcher into a custom image build (out of
# scope here — see README "Scope").
#
# In this checkout, run plain 'just review' (or another recipe below) from the
# repository root. Persistent state is limited to launcher configuration; the
# VM runner receives only its per-run control/overlay directory, never a
# workspace or host home/configuration mount.
# Goose is the only agent backend. There is no local inference, no model
# profile catalogue, and no multi-CLI auto-detection: one backend means one
# readiness check and one fix-it message when it fails.
tool_order := "goose"

# TOOL is read from the environment so 'TOOL=goose just review'
# works as documented — 'just' recipe parameters are positional, not
# KEY=VALUE, so it cannot be a plain recipe parameter. Any value other than
# 'goose' is a hard error rather than a silent fallback.
tool_env := env("TOOL", "")
hive_repo_url := "https://github.com/kubestellar/hive"
# origin/v2 via `git ls-remote --heads https://github.com/kubestellar/hive v2`
# on 2026-07-29.
hive_commit := "835448c3cbef9f06d34dd3802548e1d1e16dbd2f"
copilot_default_model := "gpt-4.1"
vm_runner_image := env("REVIEW_VM_RUNNER_IMAGE", "")
vm_raw_image := env("REVIEW_VM_RAW", "")
vm_version := env("REVIEW_VM_VERSION", "25.08.14")
# The fsdk-derived contributor image. Used by
# review-container; the VM path gets the same image from inside the
# guest, so this only matters for local development.
#
# ':stable' moves on every merge to main, so the default is always what the
# repository currently says. That is the point: the people running this are
# the people changing it, and nobody should be debugging a bug that was fixed
# yesterday.
#
# ':latest' is deliberately absent from the registry — the workflow publishes
# 'sha-<commit>' on every build, 'stable' on each main or release build, and
# version tags on a 'v*.*.*' release. Asking for ':latest' dies on 'manifest
# unknown'.
# REVIEW_CONTRIBUTOR_IMAGE overrides this when you need a specific
# 'sha-' tag or digest.
contributor_image := env("REVIEW_CONTRIBUTOR_IMAGE", "ghcr.io/projectbluefin/review:stable")

# Shared bash, 'eval''d at the top of every recipe script that needs it:
# host preflight, Goose selection, and the pinned Hive checkout. Keeping
# this in one place instead of duplicating it per-recipe is the only
# concession to DRY here — it never leaves the Justfile as a file of its own.
shared_functions := '''
GITHUB_LOGIN_COMMAND="gh auth login --web --hostname github.com --scopes repo,read:org"

github_auth_ready() {
  command -v gh &>/dev/null && gh auth status --hostname github.com &>/dev/null
}
can_run_attended_hive_setup() {
  [[ "${REVIEW_TEST_ATTACH_TTY:-}" == "1" ]] || { [[ -t 0 ]] && [[ -t 1 ]] && [[ -t 2 ]]; }
}
print_missing_hive_setup_guidance() {
  local path="$1" reason="$2" tool="$3" commit="$4"
  echo "ERROR: missing Hive setup at ${path}; ${reason}." >&2
  echo "  Re-run review from an interactive terminal, or pre-seed it yourself from kubestellar/hive @ ${commit} by running \`just contribute-setup ${tool}\` in an interactive checkout (set REVIEW_HIVE_COMMIT to another full commit if needed)" >&2
}
tool_installed() {
  case "$1" in
    goose) command -v goose &>/dev/null ;;
    *)     return 1 ;;
  esac
}
tool_authenticated() {
  # An explicit GOOSE_PROVIDER counts as configured because the launcher
  # passes it straight through to the guest; otherwise Goose's own config
  # must name a provider. A GitHub login alone is deliberately NOT enough:
  # Goose still needs a provider selected before it can talk to a model.
  case "$1" in
    goose)
      [[ -n "${GOOSE_PROVIDER:-}" ]] && return 0
      local cfg="${HOME}/.config/goose/config.yaml"
      [[ -s "$cfg" ]] && grep -Eq '^[[:space:]]*(GOOSE_PROVIDER|provider):[[:space:]]*[^[:space:]#]' "$cfg"
      ;;
    *) return 1 ;;
  esac
}
require_copilot_provider() {
  case "${GOOSE_PROVIDER:-}" in
    ""|github_copilot) return 0 ;;
    *)
      echo "ERROR: GOOSE_PROVIDER=${GOOSE_PROVIDER} is not supported — review supports GitHub Copilot only." >&2
      echo "  Unset GOOSE_PROVIDER or set GOOSE_PROVIDER=github_copilot." >&2
      return 1
      ;;
  esac
}
tool_fixit_hint() {
  case "$1" in
    goose) echo "Run: goose configure, select GitHub Copilot, and complete the device flow." ;;
    *)     echo "Unknown tool: $1" ;;
  esac
}
tool_install_hint() {
  case "$1" in
    goose) echo "Install: https://github.com/block/goose/releases" ;;
    *)     echo "" ;;
  esac
}
require_goose_backend() {
  # TOOL exists only for compatibility with the documented invocation; the
  # single supported value is the single supported backend.
  local requested="${1:-}"
  [[ -z "$requested" || "$requested" == "goose" ]] && return 0
  echo "ERROR: TOOL=${requested} is not supported — review runs Goose only." >&2
  echo "  Unset TOOL, or pass TOOL=goose." >&2
  return 1
}
preflight_agent() {
  # Exactly one ERROR line per failure, each with the command that fixes it.
  require_copilot_provider || return 1
  tool_installed goose || {
    echo "ERROR: goose is not installed." >&2
    echo "  $(tool_install_hint goose)" >&2
    return 1
  }
  github_auth_ready || {
    echo "ERROR: GitHub CLI is not authenticated against github.com." >&2
    echo "  Run: ${GITHUB_LOGIN_COMMAND}" >&2
    return 1
  }
  tool_authenticated goose || {
    echo "ERROR: Goose has no usable provider configuration." >&2
    echo "  $(tool_fixit_hint goose)" >&2
    return 1
  }
}
contributor_image_available() {
  # Locally present is enough; otherwise the tag has to exist in the
  # registry. Both probes are read-only, so review-doctor can call
  # this without starting anything.
  local ref="$1"
  podman image exists "$ref" && return 0
  podman manifest inspect "$ref" &>/dev/null
}
container_has_owner() {
  # Every launch here is a foreground 'podman run', so a run that is still
  # owned has a live client process holding that terminal. When the terminal
  # dies hard the container keeps running but the client is gone -- that is
  # an orphan, and nobody is watching it.
  # Anchored at the start of the command line so a wrapper shell that merely
  # mentions the container name -- a script, an editor, this check quoted in
  # someone's terminal -- is never mistaken for the owning client.
  local name="$1"
  pgrep -f "^podman run .*--name ${name}( |\$)" >/dev/null 2>&1
}
require_no_running_instance() {
  # 'Running' alone does not mean 'in use'. Distinguish the two cases,
  # because they deserve opposite treatment:
  #
  #   owned  -- somebody is working in that terminal right now. Never touch
  #             it; hand over the attach command instead.
  #   orphan -- still running, but its terminal is gone, so no one can ever
  #             reach it or Ctrl-C it again. Reclaim it silently.
  #
  # Telling a user to run 'podman rm -f' for the orphan case would smuggle
  # the stop command back in through the door it was thrown out of. The
  # launcher cleans up after itself instead.
  local name="$1"
  [[ "$(podman inspect --format '{{.State.Running}}' "$name" 2>/dev/null || echo false)" == "true" ]] || return 0
  if ! container_has_owner "$name"; then
    echo "✓ reclaiming ${name} from a run whose terminal is gone."
    return 0
  fi
  echo "ERROR: ${name} is already running in another terminal." >&2
  echo "  Attach to the live session: podman exec -it ${name} tmux attach -t contributor" >&2
  echo "  Or press Ctrl-C in the terminal that owns it." >&2
  return 1
}
image_ref_is_moving() {
  # A digest is immutable and an 'sha-<commit>' tag is minted once per build,
  # so both always name exactly one image. Anything else can be repointed at
  # a newer build under the same name.
  case "$1" in
    *@sha256:*) return 1 ;;
    *:sha-*)    return 1 ;;
    *)          return 0 ;;
  esac
}
ensure_contributor_image() {
  # A missing tag otherwise surfaces as a bare 'manifest unknown' from
  # podman at launch time, which says nothing about what to do next.
  #
  # A moving tag is re-pulled every launch. Treating 'present locally' as
  # good enough is what silently pinned contributors to whatever copy they
  # first pulled while the tag moved on underneath them -- the launcher
  # looked healthy and ran stale code. Best-effort by design: if the registry
  # is unreachable, an existing local copy still starts, so being offline
  # degrades to 'possibly stale' rather than 'cannot work'.
  local ref="$1"
  if image_ref_is_moving "$ref"; then
    podman pull "$ref" && return 0
    if podman image exists "$ref"; then
      echo "! could not refresh ${ref}; using the local copy, which may be out of date." >&2
      return 0
    fi
  fi
  contributor_image_available "$ref" && return 0
  podman pull "$ref" && return 0
  echo "ERROR: cannot obtain the contributor image ${ref}." >&2
  echo "  Published tags are 'stable', the version tags and 'sha-<commit>' — there is no ':latest'." >&2
  echo "  Pick a published tag with REVIEW_CONTRIBUTOR_IMAGE=ghcr.io/projectbluefin/review:stable," >&2
  echo "  or build it yourself: podman build -f image/Containerfile -t review:dev . && REVIEW_CONTRIBUTOR_IMAGE=review:dev just review-container" >&2
  return 1
}
resolve_copilot_token() {
  # Goose's github_copilot provider needs the long-lived OAuth token minted by
  # the Copilot editor device flow (a "ghu_" user-to-server token). Without it
  # the container starts a fresh device flow on every launch and the pane sits
  # on "enter code XXXX-XXXX" until a human types one in.
  #
  # A `gh auth token` ("gho_") is NOT a substitute -- it is a different client
  # with different scopes, and Goose fails with "failed to get api info" when
  # handed one. Verified against the contributor image.
  #
  # On a desktop Goose keeps the real token in the login keyring, so read it
  # from there. This is best-effort by design: no keyring, no secret-tool, or
  # a locked session just means the device flow happens as before.
  COPILOT_TOKEN="${GITHUB_COPILOT_TOKEN:-}"
  [[ -n "$COPILOT_TOKEN" ]] && return 0
  command -v secret-tool &>/dev/null || return 0
  # Extracted with sed rather than a JSON parser so this stays a single line:
  # CI lifts recipe bodies out of this file and runs 'bash -n' over them, which
  # a multi-line embedded script breaks. The trailing '|| true' matters under
  # 'set -euo pipefail': a locked or empty keyring makes secret-tool exit
  # non-zero, and pipefail would otherwise abort the whole launch over a lookup
  # that is meant to be optional.
  COPILOT_TOKEN="$(secret-tool lookup service goose username secrets 2>/dev/null | sed -nE 's/.*"GITHUB_COPILOT_TOKEN"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/p' | head -1 || true)"
  return 0
}
report_missing_copilot_credential() {
  # Named so both launch paths tell the same story. A `gh auth token` is the
  # tempting substitute and the reason this message exists: it looks like a
  # GitHub credential, so a contributor reasonably assumes their gh login is
  # enough, and then the agent dies on "failed to get api info" inside a guest
  # they cannot read.
  echo "! no Copilot credential found; the agent will ask for a device code." >&2
  echo "  A 'gh auth token' is NOT a substitute — Copilot inference rejects it." >&2
  echo "  Log in once on this host with: goose configure" >&2
  echo "  Or export GITHUB_COPILOT_TOKEN before launching." >&2
  return 0
}
hive_contributor_backend() {
  # Reads AGENT_BACKEND out of Hive's own contributor.env. Read-only: the file
  # belongs to upstream setup, so it is reported on, never rewritten.
  local path="$1"
  [[ -f "$path" ]] || return 0
  awk -F= '$1 == "AGENT_BACKEND" {sub(/^[^=]*=/, ""); gsub(/["'"'"']/, ""); print; exit}' "$path"
}
resolve_gh_token() {
  # Hive's contributor model is fork + pull request under the contributor's
  # OWN GitHub identity: /usr/local/bin/gh injects the hub's App token only
  # when HIVE_CONTRIBUTOR_MODE is not "true", and we always run with it set.
  # Upstream's own `just contribute-run` therefore passes -e GH_TOKEN from
  # `gh auth token`; without it the agent picks up a task, runs `gh`, is told
  # to `gh auth login` -- which the wrapper also blocks in contributor mode --
  # and stops. Every assigned task dies on arrival.
  #
  # By value, never by mounting ~/.config/gh: the container gets exactly one
  # credential for exactly one host, and no view of any other account, of
  # ~/.config/gh/hosts.yml, or of an enterprise login that happens to sit
  # beside it. Same reasoning as the Copilot token above.
  #
  # REVIEW_GH_TOKEN comes first so a contributor can hand the agent a
  # purpose-made, narrowly scoped PAT instead of their desktop login, which
  # typically carries admin:org, workflow and delete:packages.
  GH_TOKEN_VALUE="${REVIEW_GH_TOKEN:-${GH_TOKEN:-}}"
  GH_TOKEN_SOURCE="environment"
  if [[ -z "$GH_TOKEN_VALUE" ]]; then
    GH_TOKEN_SOURCE="gh auth token"
    command -v gh &>/dev/null || { GH_TOKEN_SOURCE=""; return 0; }
    GH_TOKEN_VALUE="$(gh auth token --hostname github.com 2>/dev/null || true)"
  fi
  [[ -n "$GH_TOKEN_VALUE" ]] || GH_TOKEN_SOURCE=""
  return 0
}
gh_token_scopes() {
  # Scopes, never the token. A contributor is about to hand these powers to an
  # autonomous agent, so the launcher says out loud what it is handing over.
  command -v gh &>/dev/null || return 0
  gh auth status --hostname github.com 2>&1 | sed -nE "s/.*[Tt]oken scopes:[[:space:]]*(.+)/\1/p" | head -1 || true
  return 0
}
report_gh_token_blast_radius() {
  local source="$1" scopes
  echo "✓ GitHub identity passed to the agent as GH_TOKEN (from ${source}; value not shown)."
  scopes="$(gh_token_scopes)"
  [[ -n "$scopes" ]] && echo "  The agent can do anything this token can: ${scopes}"
  echo "  Narrow that with: REVIEW_GH_TOKEN=<scoped PAT> (public_repo or repo is enough to fork and open a PR)."
  return 0
}
report_missing_gh_token() {
  echo "! no GitHub token found; the agent has no GitHub identity." >&2
  echo "  It cannot fork, clone, push or open a pull request, and will stop on" >&2
  echo "  'To get started with GitHub CLI, please run: gh auth login' — which it" >&2
  echo "  is not allowed to run. Every assigned task will die on arrival." >&2
  echo "  Fix it with: gh auth login --web --hostname github.com --scopes repo,read:org" >&2
  echo "  Or export REVIEW_GH_TOKEN with a scoped PAT." >&2
  return 0
}
report_vm_github_identity_blocked() {
  local presence="$1"
  echo "! VM GitHub identity is blocked: the current guest cannot receive GH_TOKEN." >&2
  if [[ "$presence" == present ]]; then
    echo "  A host GitHub token was found, but adding one would not reach this VM guest." >&2
  else
    echo "  No host GitHub token was found, but adding one would not reach this VM guest." >&2
  fi
  echo "  Until then, use review-container for work that needs fork, push, or PR access." >&2
  return 0
}
gum_can_prompt() {
  command -v gum &>/dev/null || return 1
  [[ "${REVIEW_TEST_GUM_TTY:-}" == "1" ]] || [[ -t 0 ]]
}
load_last_selections() {
  # Only the remembered Copilot model survives between runs — Goose is the
  # only backend and GitHub Copilot is the only supported provider.
  LAST_GOOSE_MODEL=""
  [[ -f "${LAST_FILE:-}" ]] && source "$LAST_FILE"
  return 0
}
resolve_goose_selection() {
  # Goose is fixed to GitHub Copilot. Only the model can vary, and the env can
  # still override it for automation.
  GOOSE_PROVIDER="github_copilot"
  GOOSE_MODEL="${GOOSE_MODEL:-}"
  load_last_selections
  if gum_can_prompt && [[ -z "$GOOSE_MODEL" ]]; then
    GOOSE_MODEL="$(gum input --value "${LAST_GOOSE_MODEL}" --placeholder "e.g. gpt-4.1 — blank for the default" --header "GitHub Copilot model (default: ${COPILOT_DEFAULT_MODEL}):")"
  fi
  if [[ -z "$GOOSE_MODEL" ]]; then
    GOOSE_MODEL="${LAST_GOOSE_MODEL:-${COPILOT_DEFAULT_MODEL}}"
    if ! gum_can_prompt; then
      echo "✓ GitHub Copilot is the default; using ${GOOSE_MODEL} (gum unavailable)."
    fi
  fi
  return 0
}
persist_goose_selection() {
  # Picker memory retains only the selected Copilot model.
  mkdir -p "$CFG_DIR"
  printf 'LAST_GOOSE_MODEL=%q\n' "$GOOSE_MODEL" > "$LAST_FILE"
  return 0
}
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
    echo "ERROR: REVIEW_HIVE_COMMIT must be a full 40-character commit SHA; branch names like v2 are not allowed." >&2
    return 1
  }
  if [[ -d "${HIVE_SRC_DIR}/.git" ]]; then
    existing_origin="$(git -C "$HIVE_SRC_DIR" remote get-url origin 2>/dev/null || true)"
    [[ -n "$existing_origin" ]] || {
      echo "ERROR: ${HIVE_SRC_DIR} is missing an origin remote; move it aside or delete it so review can recreate the pinned checkout." >&2
      return 1
    }
    if [[ "$(normalize_git_remote "$existing_origin")" != "$(normalize_git_remote "$HIVE_REPO_URL")" ]]; then
      echo "ERROR: ${HIVE_SRC_DIR} points at ${existing_origin}, expected ${HIVE_REPO_URL}." >&2
      echo "  Move it aside or delete it so review can recreate the pinned checkout." >&2
      return 1
    fi
    [[ -z "$(git -C "$HIVE_SRC_DIR" status --porcelain 2>/dev/null)" ]] || {
      echo "ERROR: ${HIVE_SRC_DIR} has local changes; refusing to execute an unverified Hive checkout." >&2
      echo "  Use a clean checkout or delete it so review can recreate the pinned source." >&2
      return 1
    }
  else
    if [[ -e "$HIVE_SRC_DIR" && ! -d "$HIVE_SRC_DIR" ]]; then
      echo "ERROR: ${HIVE_SRC_DIR} exists and is not a directory." >&2
      return 1
    fi
    if [[ -d "$HIVE_SRC_DIR" && -n "$(ls -A "$HIVE_SRC_DIR" 2>/dev/null)" ]]; then
      echo "ERROR: ${HIVE_SRC_DIR} exists but is not a managed git checkout." >&2
      echo "  Move it aside or choose an empty directory before continuing." >&2
      return 1
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
    return 1
  }
}
ensure_hive_contributor_env() {
  # Upstream 'just contribute-setup goose' writes this file. It is the only
  # host state the guest genuinely needs, and Hive owns its format.
  HIVE_CONTRIBUTOR_ENV="${HOME}/.config/hive/contributor.env"
  [[ -f "$HIVE_CONTRIBUTOR_ENV" ]] && return 0
  if [[ "${REVIEW_NON_INTERACTIVE:-}" == "true" ]]; then
    print_missing_hive_setup_guidance "$HIVE_CONTRIBUTOR_ENV" "non-interactive mode cannot answer the upstream prompts" goose "$HIVE_COMMIT"
    return 1
  fi
  if ! can_run_attended_hive_setup; then
    print_missing_hive_setup_guidance "$HIVE_CONTRIBUTOR_ENV" "stdin/stdout/stderr are not attached to a terminal" goose "$HIVE_COMMIT"
    return 1
  fi
  echo "Upstream contribute-setup hasn't run yet (no ${HIVE_CONTRIBUTOR_ENV})."
  for cmd in just gh git; do
    command -v "$cmd" &>/dev/null || { echo "ERROR: '${cmd}' is required to run contribute-setup." >&2; return 1; }
  done
  prepare_pinned_hive_checkout || return 1
  echo "Running upstream pinned setup: just contribute-setup goose"
  just --working-directory "$HIVE_SRC_DIR" --justfile "$HIVE_SRC_DIR/Justfile" contribute-setup goose
  [[ -f "$HIVE_CONTRIBUTOR_ENV" ]] || { echo "ERROR: contribute-setup ran but ${HIVE_CONTRIBUTOR_ENV} still missing." >&2; return 1; }
  echo "✓ Upstream contribute-setup complete."
}
read_hive_value() {
  local key="$1"
  awk -F= -v wanted="$key" '$1 == wanted {sub(/^[^=]*=/, ""); print; exit}' "$HIVE_CONTRIBUTOR_ENV"
}
ensure_vm_runner() {
  command -v podman &>/dev/null || {
    echo "ERROR: Podman is required to run the pinned QEMU VM runner." >&2
    echo "  Install Podman, then re-run review." >&2
    return 1
  }
  command -v python3 &>/dev/null || {
    echo "ERROR: python3 is required to open the one-shot VM bootstrap channel." >&2
    return 1
  }
  [[ -e /dev/kvm && -r /dev/kvm && -w /dev/kvm ]] || {
    echo "ERROR: /dev/kvm is unavailable or not usable by this user." >&2
    echo "  Enable KVM access, then re-run review (see: review-doctor)." >&2
    return 1
  }
}
vm_host_arch() {
  case "$(uname -m)" in
    x86_64|amd64) printf 'x86_64\n' ;;
    aarch64|arm64) printf 'aarch64\n' ;;
    *) echo "ERROR: unsupported host architecture: $(uname -m)" >&2; return 1 ;;
  esac
}
vm_firmware() {
  local arch="$1" name dir firmware
  command -v find &>/dev/null || return 1

  # Search brew's prefix first. On an immutable host (Bluefin, Aurora) the
  # distro's edk2/OVMF package usually is not layered, but Homebrew's `qemu`
  # formula ships firmware alongside the emulator it will actually run --
  # so if qemu came from brew, its matching firmware is already present.
  # Also search next to whichever qemu is on PATH, for the same reason.
  local -a roots=()
  if command -v brew &>/dev/null; then
    roots+=("$(brew --prefix 2>/dev/null)/share/qemu")
  fi
  if command -v "qemu-system-${arch}" &>/dev/null; then
    roots+=("$(dirname "$(dirname "$(command -v "qemu-system-${arch}")")")/share/qemu")
  fi
  roots+=(/usr/share /usr/lib /var/home/"${USER}"/.local/share/review/firmware)

  # Both naming schemes are in the wild: distro packages use OVMF_CODE*.fd,
  # while qemu's own bundled firmware uses edk2-<arch>-code.fd.
  local -a names=()
  case "$arch" in
    x86_64)  names=(edk2-x86_64-code.fd OVMF_CODE.fd OVMF_CODE_4M.fd OVMF.fd) ;;
    aarch64) names=(edk2-aarch64-code.fd AAVMF_CODE.fd QEMU_EFI.fd) ;;
    *) return 1 ;;
  esac

  for dir in "${roots[@]}"; do
    [[ -d "$dir" ]] || continue
    for name in "${names[@]}"; do
      # -L is required: Homebrew symlinks share/qemu into ../Cellar/qemu/<v>/,
      # and find does not follow symlinks without it.
      firmware="$(find -L "$dir" -name "$name" -print -quit 2>/dev/null)"
      [[ -n "$firmware" ]] && { printf '%s\n' "$firmware"; return 0; }
    done
  done
  return 1
}
vm_firmware_vars() {
  # Split edk2/OVMF firmware is a pflash PAIR: a read-only CODE image plus a
  # writable VARS image. `-bios` cannot load these; they must be attached as
  # pflash units 0 and 1. Given the CODE path, locate its VARS template.
  # Note x86_64 uses the i386 vars image in QEMU's own firmware tree.
  local code="$1" dir base vars
  dir="$(dirname "$code")"
  base="$(basename "$code")"
  case "$base" in
    edk2-x86_64-code.fd|edk2-i386-code.fd|edk2-x86_64-secure-code.fd) vars="edk2-i386-vars.fd" ;;
    edk2-aarch64-code.fd|edk2-arm-code.fd)                            vars="edk2-arm-vars.fd" ;;
    OVMF_CODE.fd|OVMF_CODE_4M.fd)                                     vars="OVMF_VARS.fd" ;;
    AAVMF_CODE.fd)                                                    vars="AAVMF_VARS.fd" ;;
    *) return 1 ;;
  esac
  [[ -f "${dir}/${vars}" ]] && { printf '%s\n' "${dir}/${vars}"; return 0; }
  # OVMF_CODE_4M.fd pairs with OVMF_VARS_4M.fd on some distributions.
  [[ "$vars" == OVMF_VARS.fd && -f "${dir}/OVMF_VARS_4M.fd" ]] && {
    printf '%s\n' "${dir}/OVMF_VARS_4M.fd"; return 0;
  }
  return 1
}
vm_firmware_hint() {
  # brew is the one install path that works without layering or a reboot on an
  # immutable host, and its qemu formula carries the firmware with it.
  echo "  Install QEMU (which bundles UEFI firmware) with: brew install qemu" >&2
  echo "  Already have brew qemu? Re-run 'brew reinstall qemu' to restore its share/qemu firmware." >&2
  echo "  Or layer the distro package: sudo rpm-ostree install edk2-ovmf   (needs a reboot)" >&2
}
ensure_vm_host() {
  local arch qemu
  arch="$(vm_host_arch)" || return 1
  qemu="qemu-system-${arch}"
  command -v "$qemu" &>/dev/null || {
    echo "ERROR: ${qemu} is required for local VM prototyping." >&2
    return 1
  }
  [[ -e /dev/kvm && -r /dev/kvm && -w /dev/kvm ]] || {
    echo "ERROR: /dev/kvm is unavailable or not usable by this user." >&2
    return 1
  }
}
vm_raw_cache_path() {
  local state_dir="$1" version="$2" arch="$3"
  printf '%s/review-vm-%s-%s.raw
' "$state_dir" "$version" "$arch"
}
verify_vm_raw() {
  local raw="$1"
  [[ -f "$raw" && -f "${raw}.sha256" ]] || return 1
  (cd "$(dirname "$raw")" && sha256sum -c "$(basename "${raw}.sha256")") &>/dev/null
}
cached_vm_raw() {
  # A cache entry is useful only when it is the requested version and
  # architecture *and* its sidecar still verifies it. Never fall back to a
  # convenient-looking raw from another release.
  local state_dir="$1" version="$2" arch="$3" raw
  raw="$(vm_raw_cache_path "$state_dir" "$version" "$arch")"
  [[ -e "$raw" || -e "${raw}.sha256" ]] || return 1
  if verify_vm_raw "$raw"; then
    cleanup_obsolete_vm_cache "$state_dir" "$version" "$arch"
    printf '%s
' "$raw"
    return 0
  fi
  echo "! cached VM ${version} for ${arch} is incomplete or failed verification; refetching it." >&2
  rm -f "$raw" "${raw}.sha256" "${raw}.zst" "${raw}.zst.partial" "${raw}.partial" "${raw}.sha256.partial"
  return 1
}
cleanup_obsolete_vm_cache() {
  local state_dir="$1" version="$2" arch="$3" current stale current_stale
  current="$(vm_raw_cache_path "$state_dir" "$version" "$arch")"
  shopt -s nullglob
  for stale in "$state_dir"/review-vm-*-${arch}.raw     "$state_dir"/review-vm-*-${arch}.raw.sha256     "$state_dir"/review-vm-*-${arch}.raw.zst     "$state_dir"/review-vm-*-${arch}.raw.partial     "$state_dir"/review-vm-*-${arch}.raw.zst.partial     "$state_dir"/review-vm-*-${arch}.raw.sha256.partial; do
    case "$stale" in
      *.raw) current_stale="$stale" ;;
      *.raw.sha256) current_stale="${stale%.sha256}" ;;
      *.raw.zst) current_stale="${stale%.zst}" ;;
      *.raw.partial) current_stale="${stale%.partial}" ;;
      *.raw.zst.partial) current_stale="${stale%.zst.partial}" ;;
      *.raw.sha256.partial) current_stale="${stale%.sha256.partial}" ;;
    esac
    [[ "$current_stale" == "$current" ]] || rm -f "$stale"
  done
  shopt -u nullglob
}
vm_release_url() {
  local version="$1" arch="$2"
  printf 'https://github.com/projectbluefin/fsdk-containers/releases/download/v%s/review-vm-%s-%s.raw.zst
'     "$version" "$version" "$arch"
}
vm_release_asset_available() {
  command -v curl &>/dev/null || return 1
  curl -fsIL --max-time 10 "$(vm_release_url "$1" "$2")" &>/dev/null
}
fetch_vm_raw() {
  local state_dir="$1" version="$2" arch raw url checksum_url
  arch="$(vm_host_arch)" || return 1
  command -v curl &>/dev/null || { echo "ERROR: curl is required to fetch the VM artifact." >&2; return 1; }
  command -v zstd &>/dev/null || { echo "ERROR: zstd is required to decompress the VM artifact." >&2; return 1; }
  raw="$(vm_raw_cache_path "$state_dir" "$version" "$arch")"
  local zst="${raw}.zst" checksum="${raw}.sha256"
  url="$(vm_release_url "$version" "$arch")"
  checksum_url="${url%.zst}.sha256"
  mkdir -p "$state_dir"
  echo "Fetching review VM ${version} for ${arch}..." >&2
  curl -fL --retry 3 --output "${zst}.partial" "$url" || {
    rm -f "${zst}.partial"
    if [[ "$arch" == "aarch64" ]]; then
      echo "ERROR: the aarch64 VM raw asset is unavailable for release ${version}: ${url}" >&2
      echo "  Use a published REVIEW_VM_RUNNER_IMAGE until that release asset exists." >&2
    else
      echo "ERROR: VM release asset is not published yet: ${url}" >&2
    fi
    return 1
  }
  mv "${zst}.partial" "$zst"
  curl -fL --retry 3 --output "${checksum}.partial" "$checksum_url" || {
    rm -f "$zst" "${zst}.partial" "$checksum" "${checksum}.partial"
    echo "ERROR: VM release checksum sidecar is not published yet: ${checksum_url}" >&2
    return 1
  }
  mv "${checksum}.partial" "$checksum"
  python3 - "$checksum" "$(basename "$raw")" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
lines = path.read_text().splitlines()
if lines:
    checksum, *_rest = lines[0].split(maxsplit=1)
    path.write_text(f"{checksum}  {sys.argv[2]}\n")
PY
  echo "Decompressing VM image..." >&2
  zstd -d "$zst" -o "${raw}.partial" --force || {
    rm -f "$zst" "$checksum" "${raw}.partial"
    echo "ERROR: VM decompression failed." >&2
    return 1
  }
  mv "${raw}.partial" "$raw"
  rm -f "$zst"
  (cd "$state_dir" && sha256sum -c "$(basename "$checksum")" >/dev/null) || {
    rm -f "$raw" "$checksum"
    echo "ERROR: downloaded VM checksum failed." >&2
    return 1
  }
  cleanup_obsolete_vm_cache "$state_dir" "$version" "$arch"
  printf '%s
' "$raw"
}
'''

# Start review cycles on the hive — foreground, Ctrl-C to stop.
# Goose is the only backend; TOOL=goose is accepted, anything else is an
# error. The VM boots in the foreground and the terminal belongs to the
# agent until you stop it.
# Usage: just review
#        TOOL=goose just review
review:
    #!/usr/bin/env bash
    set -euo pipefail
    {{shared_functions}}
    TOOL="{{tool_env}}"
    COPILOT_DEFAULT_MODEL="{{copilot_default_model}}"

    STATE_DIR="${HOME}/.local/state/review"
    CFG_DIR="${HOME}/.config/review"
    LAST_FILE="${CFG_DIR}/last-selections.env"
    HIVE_SRC_DIR="${STATE_DIR}/hive-src"
    HIVE_REPO_URL="{{hive_repo_url}}"
    HIVE_COMMIT="${REVIEW_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    mkdir -p "${STATE_DIR}" "${CFG_DIR}"

    require_goose_backend "$TOOL"
    preflight_agent
    resolve_goose_selection
    persist_goose_selection

    COPILOT_TOKEN=""
    if [[ "${GOOSE_PROVIDER:-}" == "github_copilot" ]]; then
      resolve_copilot_token
      if [[ -n "${COPILOT_TOKEN:-}" ]]; then
        echo "✓ Copilot credential passed to the agent."
      else
        report_missing_copilot_credential
      fi
    fi
    resolve_gh_token
    if [[ -n "${GH_TOKEN_VALUE:-}" ]]; then
      report_vm_github_identity_blocked present
    else
      report_vm_github_identity_blocked absent
    fi

    ensure_hive_contributor_env

    RUN_ID="$(date +%s)-$$"
    umask 077
    RUN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/review.XXXXXX")" || {
      echo "ERROR: could not create a private VM run directory." >&2
      exit 1
    }
    chmod 700 "$RUN_DIR" || {
      rm -rf "$RUN_DIR"
      echo "ERROR: could not secure the VM run directory." >&2
      exit 1
    }
    BOOTSTRAP_SOCKET_NAME="bootstrap-${RUN_ID}.sock"
    BOOTSTRAP_SOCKET="${RUN_DIR}/${BOOTSTRAP_SOCKET_NAME}"
    BOOTSTRAP_PID=""
    cleanup_bootstrap() {
      if [[ -n "$BOOTSTRAP_PID" ]]; then
        kill "$BOOTSTRAP_PID" 2>/dev/null || true
        wait "$BOOTSTRAP_PID" 2>/dev/null || true
      fi
      rm -f "$BOOTSTRAP_SOCKET"
      rm -rf "$RUN_DIR"
    }
    trap cleanup_bootstrap EXIT INT TERM

    HIVE_ENDPOINT="$(read_hive_value HIVE_WS_URL)"
    [[ -n "$HIVE_ENDPOINT" ]] || HIVE_ENDPOINT="$(read_hive_value HIVE_HUB)"
    HIVE_REGISTRATION_TOKEN="$(read_hive_value HIVE_REGISTRATION_TOKEN)"
    [[ -n "$HIVE_ENDPOINT" && -n "$HIVE_REGISTRATION_TOKEN" ]] || {
      echo "ERROR: Hive setup is missing HIVE_HUB/HIVE_WS_URL or HIVE_REGISTRATION_TOKEN." >&2
      exit 1
    }

    start_bootstrap_server() {
      printf '%s\0%s\0%s\0%s\0%s\0%s\0%s\0' \
        "$HIVE_ENDPOINT" "$HIVE_REGISTRATION_TOKEN" "goose" "$RUN_ID" "$GOOSE_PROVIDER" "$GOOSE_MODEL" "${COPILOT_TOKEN:-}" |
        python3 -c 'import json,os,socket,sys; path=sys.argv[1]; values=sys.stdin.buffer.read().split(b"\0"); len(values) < 7 and sys.exit("bootstrap input is incomplete"); payload={"version":2,"hive_endpoint":values[0].decode(),"registration_token":values[1].decode(),"backend":values[2].decode(),"run_id":values[3].decode(), **({"goose_provider":values[4].decode()} if values[4] else {}), **({"goose_model":values[5].decode()} if values[5] else {}), **({"provider_secret":values[6].decode()} if values[6] else {})}; os.path.exists(path) and os.unlink(path); server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM); server.bind(path); os.chmod(path,0o600); server.listen(1); server.settimeout(float(os.environ.get("REVIEW_BOOTSTRAP_TIMEOUT","180"))); conn,_=server.accept(); conn.settimeout(30); conn.sendall((json.dumps(payload,separators=(",",":"))+"\n").encode()); ack_line=conn.makefile("rb").readline(65537); ack_line.endswith(b"\n") or sys.exit("bootstrap acknowledgement ended before newline"); len(ack_line) <= 65536 or sys.exit("bootstrap acknowledgement exceeds 65536 bytes"); ack=json.loads(ack_line); ack == {"version":2,"type":"control_ack"} or sys.exit("invalid bootstrap acknowledgement"); conn.close(); server.close(); os.unlink(path)' "$BOOTSTRAP_SOCKET" &
      BOOTSTRAP_PID=$!
      local ready_deadline=$((SECONDS + 5))
      while [[ ! -S "$BOOTSTRAP_SOCKET" ]]; do
        if ! kill -0 "$BOOTSTRAP_PID" 2>/dev/null; then
          wait "$BOOTSTRAP_PID" || true
          echo "ERROR: VM bootstrap server exited before binding ${BOOTSTRAP_SOCKET}." >&2
          echo "  Check the bootstrap diagnostics above, then re-run review." >&2
          exit 1
        fi
        if (( SECONDS >= ready_deadline )); then
          echo "ERROR: VM bootstrap server did not bind ${BOOTSTRAP_SOCKET} within 5 seconds." >&2
          echo "  Check that this host can create Unix sockets in ${RUN_DIR}, then re-run review." >&2
          exit 1
        fi
        sleep 0.05
      done
    }

    VM_RAW="{{vm_raw_image}}"
    if [[ -z "$VM_RAW" && -z "{{vm_runner_image}}" ]]; then
      VM_ARCH="$(vm_host_arch)"
      VM_RAW="$(cached_vm_raw "$STATE_DIR" "{{vm_version}}" "$VM_ARCH" || true)"
    fi
    if [[ -z "$VM_RAW" && -z "{{vm_runner_image}}" && "${REVIEW_TEST_SKIP_VM_FETCH:-}" != 1 ]]; then
      VM_RAW="$(fetch_vm_raw "$STATE_DIR" "{{vm_version}}")"
    fi
    if [[ -n "$VM_RAW" ]]; then
      VM_ARCH="$(vm_host_arch)"
      [[ -f "$VM_RAW" ]] || { echo "ERROR: VM raw disk not found: $VM_RAW" >&2; exit 1; }
      [[ -f "${VM_RAW}.sha256" ]] || {
        echo "ERROR: VM raw disk checksum sidecar not found: ${VM_RAW}.sha256" >&2
        exit 1
      }
      (cd "$(dirname "$VM_RAW")" && sha256sum -c "$(basename "${VM_RAW}.sha256")") || {
        echo "ERROR: VM raw disk checksum failed: $VM_RAW" >&2
        exit 1
      }
      ensure_vm_host
      FIRMWARE="$(vm_firmware "$VM_ARCH")" || {
        echo "ERROR: matching UEFI firmware for ${VM_ARCH} was not found." >&2
        vm_firmware_hint
        exit 1
      }
      VM_OVERLAY="${RUN_DIR}/overlay.qcow2"
      if command -v qemu-img &>/dev/null; then
        qemu-img create -q -f qcow2 -F raw -b "$VM_RAW" "$VM_OVERLAY" >/dev/null || {
          echo "ERROR: could not create the per-run VM overlay." >&2
          exit 1
        }
        VM_DISK_ARGS=(-drive "file=${VM_OVERLAY},format=qcow2,if=virtio")
      else
        echo "ERROR: qemu-img is required to create a disposable VM overlay." >&2
        echo "  Install QEMU with: brew install qemu" >&2
        exit 1
      fi

      start_bootstrap_server

      case "$VM_ARCH" in
        x86_64) VM_MACHINE=q35; SERIAL_DEVICE=virtio-serial-pci ;;
        aarch64) VM_MACHINE=virt; SERIAL_DEVICE=virtio-serial-device ;;
      esac
      echo "✓ booting local review VM (4 vCPU, 8 GiB RAM)."
      echo "  Foreground by design: Ctrl-C stops it."

      FIRMWARE_ARGS=()
      if VARS_TEMPLATE="$(vm_firmware_vars "$FIRMWARE" 2>/dev/null)"; then
        RUN_VARS="${RUN_DIR}/efivars.fd"
        cp -f "$VARS_TEMPLATE" "$RUN_VARS"
        chmod u+w "$RUN_VARS"
        FIRMWARE_ARGS=(
          -drive "if=pflash,format=raw,unit=0,readonly=on,file=${FIRMWARE}"
          -drive "if=pflash,format=raw,unit=1,file=${RUN_VARS}"
        )
      else
        FIRMWARE_ARGS=(-bios "$FIRMWARE")
      fi

      set +e
      "qemu-system-${VM_ARCH}"         -enable-kvm -machine "$VM_MACHINE" -cpu host -smp 4 -m 8192         "${FIRMWARE_ARGS[@]}"         "${VM_DISK_ARGS[@]}"         -nic user,model=virtio         -chardev "socket,id=control,path=${BOOTSTRAP_SOCKET}"         -device "$SERIAL_DEVICE"         -device "virtserialport,chardev=control,name=org.projectbluefin.review.bootstrap"         -nographic
      status=$?
      set -e
      exit "$status"
    fi
    [[ -n "{{vm_runner_image}}" ]] || {
      echo "ERROR: review VM runner is not configured." >&2
      echo "  Set REVIEW_VM_RUNNER_IMAGE to the signed immutable runner image reference." >&2
      exit 1
    }
    ensure_vm_runner
    require_no_running_instance review-vm
    start_bootstrap_server
    CONTAINER_ARGS=(
      podman run --rm --interactive --tty --replace --name review-vm
      --device /dev/kvm
      --mount "type=bind,src=${RUN_DIR},dst=/run/review,rw"
      --env "REVIEW_BOOTSTRAP_SOCKET=/run/review/${BOOTSTRAP_SOCKET_NAME}"
      --env "REVIEW_RUN_ID=${RUN_ID}"
      --env "REVIEW_VM=1"
      --env "AGENT_BACKEND=goose"
    )
    [[ -n "$GOOSE_PROVIDER" ]] && CONTAINER_ARGS+=(--env "GOOSE_PROVIDER=${GOOSE_PROVIDER}")
    [[ -n "$GOOSE_MODEL" ]] && CONTAINER_ARGS+=(--env "GOOSE_MODEL=${GOOSE_MODEL}")
    CONTAINER_ARGS+=("{{vm_runner_image}}")
    echo "✓ review is running in the pinned QEMU VM runner."
    echo "  Stop any time with Ctrl-C — that is the only way it ends."
    set +e
    "${CONTAINER_ARGS[@]}"
    status=$?
    set -e
    exit "$status"

# Run ONLY the contributor container — no VM — for quick local development.
# Same preflight and same provider/model selection as the VM path, minus the
# hardware isolation, so it is the fast loop while hacking on the image.
# Usage: just review-container
review-container:
    #!/usr/bin/env bash
    set -euo pipefail
    {{shared_functions}}
    TOOL="{{tool_env}}"
    COPILOT_DEFAULT_MODEL="{{copilot_default_model}}"

    CFG_DIR="${HOME}/.config/review"
    LAST_FILE="${CFG_DIR}/last-selections.env"
    STATE_DIR="${HOME}/.local/state/review"
    HIVE_SRC_DIR="${STATE_DIR}/hive-src"
    HIVE_REPO_URL="{{hive_repo_url}}"
    HIVE_COMMIT="${REVIEW_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    mkdir -p "${CFG_DIR}" "${STATE_DIR}"

    require_goose_backend "$TOOL"
    preflight_agent
    command -v podman &>/dev/null || {
      echo "ERROR: Podman is required to run the contributor container." >&2
      echo "  Install Podman, then re-run review-container." >&2
      exit 1
    }

    resolve_goose_selection
    persist_goose_selection
    ensure_hive_contributor_env

    CONTAINER_NAME="review-container"
    CONTRIBUTOR_IMAGE="{{contributor_image}}"
    require_no_running_instance "$CONTAINER_NAME"
    ensure_contributor_image "$CONTRIBUTOR_IMAGE"

    CONTAINER_ARGS=(
      podman run --rm --interactive --tty --replace --name "$CONTAINER_NAME"
      --volume "${HOME}/.config/hive:/home/dev/.config/hive:ro"
      --env "AGENT_BACKEND=goose"
    )
    [[ -n "$GOOSE_PROVIDER" ]] && CONTAINER_ARGS+=(--env "GOOSE_PROVIDER=${GOOSE_PROVIDER}")
    [[ -n "$GOOSE_MODEL" ]] && CONTAINER_ARGS+=(--env "GOOSE_MODEL=${GOOSE_MODEL}")
    if [[ "${GOOSE_PROVIDER:-}" == "github_copilot" ]]; then
      resolve_copilot_token
      if [[ -n "${COPILOT_TOKEN:-}" ]]; then
        export GITHUB_COPILOT_TOKEN="$COPILOT_TOKEN"
        CONTAINER_ARGS+=(--env GITHUB_COPILOT_TOKEN)
        echo "✓ Copilot credential passed to the agent."
      else
        report_missing_copilot_credential
      fi
    fi
    resolve_gh_token
    if [[ -n "${GH_TOKEN_VALUE:-}" ]]; then
      export GH_TOKEN="$GH_TOKEN_VALUE"
      CONTAINER_ARGS+=(--env GH_TOKEN)
      report_gh_token_blast_radius "${GH_TOKEN_SOURCE}"
    else
      report_missing_gh_token
    fi
    CONTAINER_ARGS+=("$CONTRIBUTOR_IMAGE")

    echo "✓ starting the review contributor container (no VM)."
    echo "  The entrypoint attaches to the 'contributor' tmux session for you."
    echo "  From a second terminal: podman exec -it ${CONTAINER_NAME} tmux attach -t contributor"
    echo "  Stop any time with Ctrl-C — that is the only way it ends."
    exec "${CONTAINER_ARGS[@]}"

# Preflight check: is this machine actually ready for 'just review'?
# Never starts the VM or the container — read-only diagnostics only.
review-doctor:
    #!/usr/bin/env bash
    set -uo pipefail
    {{shared_functions}}
    COPILOT_DEFAULT_MODEL="{{copilot_default_model}}"
    HIVE_COMMIT="${REVIEW_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    pass=0; fail=0
    check() {
      local label="$1"; shift
      if "$@" &>/dev/null; then echo "  ✓ ${label}"; pass=$((pass+1));
      else echo "  ✗ ${label}"; fail=$((fail+1)); fi
    }

    echo "=== Host ==="
    check "Podman installed" command -v podman
    if [[ -e /dev/kvm && -r /dev/kvm && -w /dev/kvm ]]; then
      echo "  ✓ /dev/kvm is available"
      pass=$((pass+1))
    else
      echo "  ✗ /dev/kvm is unavailable or not usable by this user"
      echo "    Enable KVM access before launching the pinned QEMU VM runner."
      echo "    (review-container does not need it.)"
      fail=$((fail+1))
    fi
    check "gum installed (used for the provider/model pickers)" command -v gum
    echo ""

    echo "=== VM startup ==="
    if VM_ARCH="$(vm_host_arch)"; then
      echo "  ✓ supported VM architecture: ${VM_ARCH}"
      pass=$((pass+1))
      check "python3 installed (VM bootstrap socket)" command -v python3
    else
      echo "  ✗ this host architecture cannot run the local VM"
      fail=$((fail+1))
      VM_ARCH=""
    fi
    if [[ -n "{{vm_raw_image}}" ]]; then
      echo "  ✓ configured VM raw disk: {{vm_raw_image}}"
      pass=$((pass+1))
      check "qemu-system-${VM_ARCH} installed" command -v "qemu-system-${VM_ARCH}"
      check "qemu-img installed (disposable VM overlays)" command -v qemu-img
      if FIRMWARE_PATH="$(vm_firmware "$VM_ARCH" 2>/dev/null)"; then
        echo "  ✓ UEFI firmware found: ${FIRMWARE_PATH}"
        pass=$((pass+1))
      else
        echo "  ✗ no UEFI firmware for ${VM_ARCH}"
        vm_firmware_hint
        fail=$((fail+1))
      fi
      if verify_vm_raw "{{vm_raw_image}}"; then
        echo "  ✓ configured VM raw disk checksum verifies"
        pass=$((pass+1))
      else
        echo "  ✗ configured VM raw disk is missing or its checksum sidecar does not verify"
        echo "    Supply {{vm_raw_image}} and {{vm_raw_image}}.sha256, or unset REVIEW_VM_RAW."
        fail=$((fail+1))
      fi
    elif [[ -n "{{vm_runner_image}}" ]]; then
      if contributor_image_available "{{vm_runner_image}}"; then
        echo "  ✓ configured VM runner image is resolvable: {{vm_runner_image}}"
        pass=$((pass+1))
      else
        echo "  ✗ configured VM runner image cannot be resolved: {{vm_runner_image}}"
        echo "    Publish or correct REVIEW_VM_RUNNER_IMAGE."
        fail=$((fail+1))
      fi
    elif [[ -n "$VM_ARCH" ]]; then
      check "qemu-system-${VM_ARCH} installed" command -v "qemu-system-${VM_ARCH}"
      check "qemu-img installed (disposable VM overlays)" command -v qemu-img
      check "curl installed (VM artifact download)" command -v curl
      check "zstd installed (VM artifact decompression)" command -v zstd
      if FIRMWARE_PATH="$(vm_firmware "$VM_ARCH" 2>/dev/null)"; then
        echo "  ✓ UEFI firmware found: ${FIRMWARE_PATH}"
        pass=$((pass+1))
      else
        echo "  ✗ no UEFI firmware for ${VM_ARCH}"
        vm_firmware_hint
        fail=$((fail+1))
      fi
      VM_RELEASE_URL="$(vm_release_url "{{vm_version}}" "$VM_ARCH")"
      if vm_release_asset_available "{{vm_version}}" "$VM_ARCH"; then
        echo "  ✓ VM release artifact is published: ${VM_RELEASE_URL}"
        pass=$((pass+1))
      elif [[ "$VM_ARCH" == "aarch64" ]]; then
        echo "  ✗ aarch64 VM release artifact is unavailable: ${VM_RELEASE_URL}"
        echo "    Configure a published REVIEW_VM_RUNNER_IMAGE until the aarch64 raw asset is released."
        fail=$((fail+1))
      else
        echo "  ✗ VM release artifact is unavailable: ${VM_RELEASE_URL}"
        echo "    Check REVIEW_VM_VERSION or configure REVIEW_VM_RUNNER_IMAGE."
        fail=$((fail+1))
      fi
    fi
    echo ""

    echo "=== GitHub ==="
    if github_auth_ready; then
      echo "  ✓ gh is authenticated against github.com"
      pass=$((pass+1))
    else
      echo "  ✗ gh is not authenticated against github.com"
      echo "    Run: ${GITHUB_LOGIN_COMMAND}"
      fail=$((fail+1))
    fi
    resolve_gh_token
    if [[ -n "${GH_TOKEN_VALUE:-}" ]]; then
      echo "  ✓ a GitHub token is available for the container-only agent (from ${GH_TOKEN_SOURCE}; not shown)"
      DOCTOR_GH_SCOPES="$(gh_token_scopes)"
      [[ -n "$DOCTOR_GH_SCOPES" ]] && echo "    The agent will be able to do anything this token can: ${DOCTOR_GH_SCOPES}"
      echo "    Narrow that with REVIEW_GH_TOKEN=<scoped PAT> if that is wider than you want."
      pass=$((pass+1))
    else
      echo "  ✗ no GitHub token is available for the container-only agent"
      echo "    It could not fork, push, or open a pull request, and would stop at 'gh auth login'."
      echo "    For container-only mode, run: ${GITHUB_LOGIN_COMMAND}, or export REVIEW_GH_TOKEN."
      fail=$((fail+1))
    fi
    echo "  ! VM GitHub identity is blocked: the current guest cannot receive GH_TOKEN."
    echo "    Required before VM mode can fork, push, or open PRs: a guest that maps a compatible bootstrap identity field."
    echo "    A host gh login or REVIEW_GH_TOKEN cannot satisfy this VM prerequisite."
    unset GH_TOKEN_VALUE
    echo ""

    echo "=== Agent backend (Goose only) ==="
    if ! require_copilot_provider; then
      fail=$((fail+1))
    elif tool_installed goose; then
      if tool_authenticated goose; then
        echo "  ✓ goose: installed + configured"
        pass=$((pass+1))
      else
        echo "  ✗ goose: installed, NOT configured — $(tool_fixit_hint goose)"
        fail=$((fail+1))
      fi
    else
      echo "  ✗ goose: not installed — $(tool_install_hint goose)"
      fail=$((fail+1))
    fi
    echo ""

    echo "=== Copilot credential ==="
    resolve_copilot_token
    if [[ -n "${COPILOT_TOKEN:-}" ]]; then
      echo "  ✓ a Copilot credential is available (not shown)"
      pass=$((pass+1))
    else
      echo "  ✗ no Copilot credential is available"
      echo "    The agent will stop at 'enter code XXXX-XXXX' and wait for a human."
      echo "    A 'gh auth token' is NOT a substitute — Copilot inference rejects it."
      echo "    Run: goose configure (pick GitHub Copilot), or export GITHUB_COPILOT_TOKEN."
      fail=$((fail+1))
    fi
    unset COPILOT_TOKEN
    echo ""

    echo "=== Contributor image ==="
    DOCTOR_CONTRIBUTOR_IMAGE="{{contributor_image}}"
    if contributor_image_available "$DOCTOR_CONTRIBUTOR_IMAGE"; then
      echo "  ✓ ${DOCTOR_CONTRIBUTOR_IMAGE} is resolvable"
      pass=$((pass+1))
    else
      echo "  ✗ ${DOCTOR_CONTRIBUTOR_IMAGE} cannot be resolved"
      echo "    Published tags are 'stable', the version tags and 'sha-<commit>'; ':latest' does not exist."
      echo "    Override with REVIEW_CONTRIBUTOR_IMAGE (a 'sha-' tag or digest pins a build), or build image/Containerfile locally."
      fail=$((fail+1))
    fi
    echo ""

    echo "=== Hive contributor setup ==="
    HIVE_CONTRIBUTOR_ENV="${HOME}/.config/hive/contributor.env"
    if [[ -f "$HIVE_CONTRIBUTOR_ENV" ]]; then
      echo "  ✓ ${HIVE_CONTRIBUTOR_ENV} exists"
      pass=$((pass+1))
      DOCTOR_BACKEND="$(hive_contributor_backend "$HIVE_CONTRIBUTOR_ENV")"
      if [[ -n "$DOCTOR_BACKEND" && "$DOCTOR_BACKEND" != "goose" ]]; then
        echo "  ! ${HIVE_CONTRIBUTOR_ENV} says AGENT_BACKEND=${DOCTOR_BACKEND}, but review always launches goose."
        echo "    Harmless — the launcher passes AGENT_BACKEND=goose itself — but stale."
        echo "    Edit that line yourself if you want the file to match; review will not touch it."
      fi
    else
      echo "  ✗ ${HIVE_CONTRIBUTOR_ENV} is missing"
      echo "    review runs upstream 'just contribute-setup goose' from"
      echo "    kubestellar/hive @ ${HIVE_COMMIT:0:12} on first attended launch."
      fail=$((fail+1))
    fi
    echo ""

    echo "=== Guest repository model ==="
    echo "  ✓ assigned repositories are cloned inside the disposable guest"
    echo ""
    echo "${pass} checks passed, ${fail} failed."
    [[ "$fail" -eq 0 ]] || exit 1
