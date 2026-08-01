# 61-donate-clanker.just — the file an eventual image build would ship at
# /usr/share/ublue-os/just/61-donate-clanker.just (not done yet — see below).
#
# This is the ONLY file that ships/installs. Everything donate-clanker needs
# (host preflight, Goose selection, VM lifecycle, container lifecycle) is
# embedded below as private ('_'-prefixed variables and shared shell
# functions) on purpose: a user browsing the image or this repo should find
# one just-recipe file and the commands it exposes, not a scattered bin/ of
# standalone scripts they might stumble into and run directly out of context.
#
# Public commands:
#   donate-clanker            Boot the pinned QEMU VM in the FOREGROUND and
#                             hand the terminal to the contributor agent.
#   donate-clanker-container  Run only the contributor container — no VM —
#                             for quick local development. Also foreground.
#   donate-clanker-doctor     Read-only preflight diagnostics. Starts nothing.
#
# ─────────────────────────────────────────────────────────────────────────
# FOREGROUND GUARANTEE
#
# The VM and the container ALWAYS run in the foreground of the terminal that
# launched them. There is no '&', no 'nohup', no 'setsid', no 'podman run
# -d', and no '--detach' on any launch path in this file, and there never
# should be. Two reasons, both non-negotiable:
#
#   1. A human must be able to steer the agent. donate-clanker is an
#      attended tool: you watch the session, you interrupt it, you type into
#      it. A backgrounded agent is an agent nobody is supervising.
#   2. Ctrl-C must stop it. Signals have to reach the QEMU process or the
#      container directly, so the run dies with the terminal. No daemon, no
#      systemd unit, no orphaned state to reap later.
#
# It follows that there is NO stop command, and there must never be one.
# Shipping 'ujust donate-clanker-stop' would be an admission that a run can
# outlive the terminal that started it — the exact property this file exists
# to prevent. Ctrl-C is the stop button. A run that needs a second command to
# end it is a daemon wearing a disguise, and this repository does not ship
# daemons. Cleanup is therefore a startup concern, never a user-facing verb:
# a launch reclaims whatever a previous run left behind.
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
# fixed list of files, NOT a glob. Getting this file under
# /usr/share/ublue-os/just/ so plain 'ujust donate-clanker' works
# system-wide means baking it into a custom image build (out of scope here
# — see README "Scope").
#
# For local/personal use right now, run this file directly with plain
# 'just --justfile', or import it from your own personal Justfile — see
# README "Installing this into your own setup" for the copy-paste commands.
# Persistent state is limited to launcher configuration; the VM runner
# receives only its per-run control/overlay directory, never a workspace or
# host home/configuration mount.

# Goose is the only agent backend. There is no local inference, no model
# profile catalogue, and no multi-CLI auto-detection: one backend means one
# readiness check and one fix-it message when it fails.
tool_order := "goose"

# TOOL is read from the environment so 'TOOL=goose ujust donate-clanker'
# works as documented — 'just' recipe parameters are positional, not
# KEY=VALUE, so it cannot be a plain recipe parameter. Any value other than
# 'goose' is a hard error rather than a silent fallback.
tool_env := env("TOOL", "")
hive_repo_url := "https://github.com/kubestellar/hive"
# origin/v2 via `git ls-remote --heads https://github.com/kubestellar/hive v2`
# on 2026-07-29.
hive_commit := "e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e"
vm_runner_image := env("DONATE_CLANKER_VM_RUNNER_IMAGE", "")
vm_raw_image := env("DONATE_CLANKER_VM_RAW", "")
vm_version := env("DONATE_CLANKER_VM_VERSION", "25.08.14")
# The fsdk-derived contributor image. Used by
# donate-clanker-container; the VM path gets the same image from inside the
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
# DONATE_CLANKER_CONTRIBUTOR_IMAGE overrides this when you need a specific
# 'sha-' tag or digest.
contributor_image := env("DONATE_CLANKER_CONTRIBUTOR_IMAGE", "ghcr.io/projectbluefin/donate-clanker:stable")

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
  [[ "${DONATE_CLANKER_TEST_ATTACH_TTY:-}" == "1" ]] || { [[ -t 0 ]] && [[ -t 1 ]] && [[ -t 2 ]]; }
}
print_missing_hive_setup_guidance() {
  local path="$1" reason="$2" tool="$3" commit="$4"
  echo "ERROR: missing Hive setup at ${path}; ${reason}." >&2
  echo "  Re-run donate-clanker from an interactive terminal, or pre-seed it yourself from kubestellar/hive @ ${commit} by running \`just contribute-setup ${tool}\` in an interactive checkout (set DONATE_CLANKER_HIVE_COMMIT to another full commit if needed)" >&2
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
  echo "ERROR: TOOL=${requested} is not supported — donate-clanker runs Goose only." >&2
  echo "  Unset TOOL, or pass TOOL=goose." >&2
  return 1
}
preflight_agent() {
  # Exactly one ERROR line per failure, each with the command that fixes it.
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
  # registry. Both probes are read-only, so donate-clanker-doctor can call
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
  echo "  Pick a published tag with DONATE_CLANKER_CONTRIBUTOR_IMAGE=ghcr.io/projectbluefin/donate-clanker:stable," >&2
  echo "  or build it yourself: podman build -f image/Containerfile -t donate-clanker:dev . && DONATE_CLANKER_CONTRIBUTOR_IMAGE=donate-clanker:dev ujust donate-clanker-container" >&2
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
  # DONATE_CLANKER_GH_TOKEN comes first so a contributor can hand the agent a
  # purpose-made, narrowly scoped PAT instead of their desktop login, which
  # typically carries admin:org, workflow and delete:packages.
  GH_TOKEN_VALUE="${DONATE_CLANKER_GH_TOKEN:-${GH_TOKEN:-}}"
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
  echo "  Narrow that with: DONATE_CLANKER_GH_TOKEN=<scoped PAT> (public_repo or repo is enough to fork and open a PR)."
  return 0
}
report_missing_gh_token() {
  echo "! no GitHub token found; the agent has no GitHub identity." >&2
  echo "  It cannot fork, clone, push or open a pull request, and will stop on" >&2
  echo "  'To get started with GitHub CLI, please run: gh auth login' — which it" >&2
  echo "  is not allowed to run. Every assigned task will die on arrival." >&2
  echo "  Fix it with: gh auth login --web --hostname github.com --scopes repo,read:org" >&2
  echo "  Or export DONATE_CLANKER_GH_TOKEN with a scoped PAT." >&2
  return 0
}
gum_can_prompt() {
  command -v gum &>/dev/null || return 1
  [[ "${DONATE_CLANKER_TEST_GUM_TTY:-}" == "1" ]] || [[ -t 0 ]]
}
load_last_selections() {
  # Only the Goose provider/model survive between runs — there is nothing
  # else left to remember now that Goose is the only backend.
  LAST_GOOSE_PROVIDER="" LAST_GOOSE_MODEL=""
  [[ -f "${LAST_FILE:-}" ]] && source "$LAST_FILE"
  return 0
}
goose_provider_label() {
  case "$1" in
    anthropic)  echo "Anthropic" ;;
    openai)     echo "OpenAI" ;;
    google)     echo "Google" ;;
    ollama)     echo "Ollama" ;;
    openrouter) echo "OpenRouter" ;;
    *)          echo "GitHub Copilot" ;;
  esac
}
goose_provider_id() {
  case "$1" in
    "GitHub Copilot") echo "github_copilot" ;;
    Anthropic)        echo "anthropic" ;;
    OpenAI)           echo "openai" ;;
    Google)           echo "google" ;;
    Ollama)           echo "ollama" ;;
    OpenRouter)       echo "openrouter" ;;
    *)                echo "$1" ;;
  esac
}
resolve_goose_selection() {
  # Env wins, then the interactive pickers, then last run's choice, then
  # GitHub Copilot. Sets GOOSE_PROVIDER/GOOSE_MODEL for the caller.
  GOOSE_PROVIDER="${GOOSE_PROVIDER:-}"
  GOOSE_MODEL="${GOOSE_MODEL:-}"
  load_last_selections
  if gum_can_prompt; then
    if [[ -z "$GOOSE_PROVIDER" ]]; then
      GOOSE_PROVIDER="$(printf '%s\n' 'GitHub Copilot' Anthropic OpenAI Google Ollama OpenRouter | gum choose --selected="$(goose_provider_label "$LAST_GOOSE_PROVIDER")" --header "Goose provider (defaults to GitHub Copilot):")"
      GOOSE_PROVIDER="$(goose_provider_id "$GOOSE_PROVIDER")"
    fi
    if [[ -z "$GOOSE_MODEL" ]]; then
      GOOSE_MODEL="$(gum input --value "${LAST_GOOSE_MODEL}" --placeholder "e.g. claude-3-5-sonnet — blank for goose's default" --header "Goose model (optional):")"
    fi
  fi
  [[ -n "$GOOSE_PROVIDER" ]] || GOOSE_PROVIDER="${LAST_GOOSE_PROVIDER:-github_copilot}"
  return 0
}
persist_goose_selection() {
  # last-selections.env is the picker memory; secrets.env holds the values
  # that are handed to the guest, rewritten from scratch each run so a stale
  # provider or model can never leak into a later launch.
  mkdir -p "$CFG_DIR"
  {
    printf 'LAST_GOOSE_PROVIDER=%q\n' "$GOOSE_PROVIDER"
    printf 'LAST_GOOSE_MODEL=%q\n' "$GOOSE_MODEL"
  } > "$LAST_FILE"

  touch "${CFG_DIR}/secrets.env"
  chmod 600 "${CFG_DIR}/secrets.env"
  grep -v -E '^(GOOSE_PROVIDER|GOOSE_MODEL|AGENT_MODEL)=' "${CFG_DIR}/secrets.env" > "${CFG_DIR}/secrets.env.tmp" 2>/dev/null || true
  mv "${CFG_DIR}/secrets.env.tmp" "${CFG_DIR}/secrets.env"
  [[ -n "$GOOSE_PROVIDER" ]] && echo "GOOSE_PROVIDER=${GOOSE_PROVIDER}" >> "${CFG_DIR}/secrets.env"
  [[ -n "$GOOSE_MODEL" ]] && echo "GOOSE_MODEL=${GOOSE_MODEL}" >> "${CFG_DIR}/secrets.env"
  chmod 600 "${CFG_DIR}/secrets.env"
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
    echo "ERROR: DONATE_CLANKER_HIVE_COMMIT must be a full 40-character commit SHA; branch names like v2 are not allowed." >&2
    return 1
  }
  if [[ -d "${HIVE_SRC_DIR}/.git" ]]; then
    existing_origin="$(git -C "$HIVE_SRC_DIR" remote get-url origin 2>/dev/null || true)"
    [[ -n "$existing_origin" ]] || {
      echo "ERROR: ${HIVE_SRC_DIR} is missing an origin remote; move it aside or delete it so donate-clanker can recreate the pinned checkout." >&2
      return 1
    }
    if [[ "$(normalize_git_remote "$existing_origin")" != "$(normalize_git_remote "$HIVE_REPO_URL")" ]]; then
      echo "ERROR: ${HIVE_SRC_DIR} points at ${existing_origin}, expected ${HIVE_REPO_URL}." >&2
      echo "  Move it aside or delete it so donate-clanker can recreate the pinned checkout." >&2
      return 1
    fi
    [[ -z "$(git -C "$HIVE_SRC_DIR" status --porcelain 2>/dev/null)" ]] || {
      echo "ERROR: ${HIVE_SRC_DIR} has local changes; refusing to execute an unverified Hive checkout." >&2
      echo "  Use a clean checkout or delete it so donate-clanker can recreate the pinned source." >&2
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
  if [[ "${DONATE_CLANKER_NON_INTERACTIVE:-}" == "true" ]]; then
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
    echo "  Install Podman, then re-run donate-clanker." >&2
    return 1
  }
  command -v python3 &>/dev/null || {
    echo "ERROR: python3 is required to open the one-shot VM bootstrap channel." >&2
    return 1
  }
  [[ -e /dev/kvm && -r /dev/kvm && -w /dev/kvm ]] || {
    echo "ERROR: /dev/kvm is unavailable or not usable by this user." >&2
    echo "  Enable KVM access, then re-run donate-clanker (see: donate-clanker-doctor)." >&2
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
  roots+=(/usr/share /usr/lib /var/home/"${USER}"/.local/share/donate-clanker/firmware)

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
fetch_vm_raw() {
  local state_dir="$1" version="$2" arch raw url
  arch="$(vm_host_arch)" || return 1
  command -v curl &>/dev/null || { echo "ERROR: curl is required to fetch the VM artifact." >&2; return 1; }
  command -v zstd &>/dev/null || { echo "ERROR: zstd is required to decompress the VM artifact." >&2; return 1; }
  raw="${state_dir}/donate-clanker-vm-${version}-${arch}.raw"
  local zst="${raw}.zst" checksum="${raw}.sha256"
  local base_url="https://github.com/projectbluefin/fsdk-containers/releases/download/v${version}"
  mkdir -p "$state_dir"
  echo "Fetching donate-clanker VM ${version} for ${arch}..."
  curl -fL --retry 3 --output "${zst}.partial" "${base_url}/$(basename "$zst")" || {
    rm -f "${zst}.partial"
    echo "ERROR: VM release asset is not published yet: ${base_url}/$(basename "$zst")" >&2
    return 1
  }
  mv "${zst}.partial" "$zst"
  curl -fL --retry 3 --output "${checksum}.partial" "${base_url}/$(basename "$checksum")" || {
    rm -f "$zst" "${zst}.partial" "$checksum" "${checksum}.partial"
    echo "ERROR: VM release checksum sidecar is not published yet: ${base_url}/$(basename "$checksum")" >&2
    return 1
  }
  mv "${checksum}.partial" "$checksum"
  echo "Decompressing VM image..."
  zstd -d "$zst" -o "${raw}.partial" --force || {
    rm -f "$zst" "$checksum" "${raw}.partial"
    echo "ERROR: VM decompression failed." >&2
    return 1
  }
  mv "${raw}.partial" "$raw"
  rm -f "$zst"
  (cd "$state_dir" && sha256sum -c "$(basename "$checksum")") || {
    rm -f "$raw" "$checksum"
    echo "ERROR: downloaded VM checksum failed." >&2
    return 1
  }
  printf '%s\n' "$raw"
}
'''

# Start donating clanker cycles to the hive — foreground, Ctrl-C to stop.
# Goose is the only backend; TOOL=goose is accepted, anything else is an
# error. The VM boots in the foreground and the terminal belongs to the
# agent until you stop it.
# Usage: ujust donate-clanker
#        TOOL=goose ujust donate-clanker
donate-clanker:
    #!/usr/bin/env bash
    set -euo pipefail
    {{shared_functions}}
    TOOL="{{tool_env}}"

    STATE_DIR="${HOME}/.local/state/donate-clanker"
    CFG_DIR="${HOME}/.config/donate-clanker"
    LAST_FILE="${CFG_DIR}/last-selections.env"
    HIVE_SRC_DIR="${STATE_DIR}/hive-src"
    HIVE_REPO_URL="{{hive_repo_url}}"
    HIVE_COMMIT="${DONATE_CLANKER_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    mkdir -p "${STATE_DIR}" "${CFG_DIR}"

    # ── 0. Preflight: one backend, one readiness check, one fix-it line. ──
    require_goose_backend "$TOOL"
    preflight_agent

    # ── 1. Provider/model, remembered between runs. ──
    resolve_goose_selection
    persist_goose_selection

    # The guest's agent needs the same Copilot credential the container path
    # gets, or it stalls on a device code nobody is there to type. It travels
    # inside the bootstrap envelope over the one-shot socket, never as a runner
    # environment variable or command-line argument.
    COPILOT_TOKEN=""
    if [[ "${GOOSE_PROVIDER:-}" == "github_copilot" ]]; then
      resolve_copilot_token
      if [[ -n "${COPILOT_TOKEN:-}" ]]; then
        echo "✓ Copilot credential passed to the agent."
      else
        report_missing_copilot_credential
      fi
    fi

    # ── 2. Ensure upstream contribute-setup has run. ──
    ensure_hive_contributor_env

    RUN_ID="$(date +%s)-$$"
    RUN_DIR="${TMPDIR:-/tmp}/donate-clanker-$$"
    mkdir -p "$RUN_DIR"
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

    # QEMU opens this socket from the runner's per-run bind mount. The
    # version-2 envelope is read from a pipe and sent once; no secret is
    # present in the runner environment, command line, or a host-created env
    # file. This one-shot server is the ONLY background job in this file —
    # it is host-local, is reaped by cleanup_bootstrap, and is not the VM.
    umask 077
    # Timeouts: the guest must boot Linux and start its consumer before it
    # can connect, which takes far longer than a few seconds on a cold boot.
    # DONATE_CLANKER_BOOTSTRAP_TIMEOUT overrides the accept timeout.
    printf '%s\0%s\0%s\0%s\0%s\0%s\0%s\0' \
      "$HIVE_ENDPOINT" "$HIVE_REGISTRATION_TOKEN" "goose" "$RUN_ID" "$GOOSE_PROVIDER" "$GOOSE_MODEL" "${COPILOT_TOKEN:-}" |
      python3 -c 'import json,os,socket,sys; path=sys.argv[1]; values=sys.stdin.buffer.read().split(b"\0"); len(values) < 7 and sys.exit("bootstrap input is incomplete"); payload={"version":2,"hive_endpoint":values[0].decode(),"registration_token":values[1].decode(),"backend":values[2].decode(),"run_id":values[3].decode(), **({"goose_provider":values[4].decode()} if values[4] else {}), **({"goose_model":values[5].decode()} if values[5] else {}), **({"provider_secret":values[6].decode()} if values[6] else {})}; os.path.exists(path) and os.unlink(path); server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM); server.bind(path); os.chmod(path,0o600); server.listen(1); server.settimeout(float(os.environ.get("DONATE_CLANKER_BOOTSTRAP_TIMEOUT","180"))); conn,_=server.accept(); conn.settimeout(30); conn.sendall((json.dumps(payload,separators=(",",":"))+"\n").encode()); ack=json.loads(conn.recv(65536).splitlines()[0]); ack == {"version":2,"type":"control_ack"} or sys.exit("invalid bootstrap acknowledgement"); conn.close(); server.close(); os.unlink(path)' "$BOOTSTRAP_SOCKET" &
    BOOTSTRAP_PID=$!

    # ── 3. Run the self-contained QEMU VM in the FOREGROUND. The guest
    #        clones assignments itself; only the per-run control/overlay
    #        directory crosses this boundary. ──
    VM_RAW="{{vm_raw_image}}"
    # An explicit raw disk wins. Otherwise, an explicitly configured runner
    # image skips the local artifact lookup and fetch path.
    if [[ -z "$VM_RAW" && -z "{{vm_runner_image}}" ]]; then
      VM_ARCH="$(vm_host_arch)"
      shopt -s nullglob
      LOCAL_VM_CANDIDATES=("${STATE_DIR}"/*-"${VM_ARCH}".raw)
      shopt -u nullglob
      [[ "${#LOCAL_VM_CANDIDATES[@]}" -eq 1 ]] && VM_RAW="${LOCAL_VM_CANDIDATES[0]}"
    fi
    if [[ -z "$VM_RAW" && -z "{{vm_runner_image}}" && "${DONATE_CLANKER_TEST_SKIP_VM_FETCH:-}" != 1 ]]; then
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
      # Boot a disposable copy-on-write overlay, never the verified master
      # image. Writing directly to the master mutates it, which breaks its
      # checksum on the next run and silently destroys the pristine artifact.
      # The overlay lives in the per-run directory and dies with the run, which
      # is what makes each VM genuinely disposable.
      VM_OVERLAY="${RUN_DIR}/overlay.qcow2"
      if command -v qemu-img &>/dev/null; then
        qemu-img create -q -f qcow2 -F raw -b "$VM_RAW" "$VM_OVERLAY" >/dev/null || {
          echo "ERROR: could not create the per-run VM overlay." >&2
          exit 1
        }
        VM_DISK_ARGS=(-drive "file=${VM_OVERLAY},format=qcow2,if=virtio")
      else
        # Without qemu-img we cannot protect the master, so refuse rather than
        # corrupt it.
        echo "ERROR: qemu-img is required to create a disposable VM overlay." >&2
        echo "  Install QEMU with: brew install qemu" >&2
        exit 1
      fi

      case "$VM_ARCH" in
        x86_64) VM_MACHINE=q35; SERIAL_DEVICE=virtio-serial-pci ;;
        aarch64) VM_MACHINE=virt; SERIAL_DEVICE=virtio-serial-device ;;
      esac
      echo "✓ booting local donate-clanker VM (4 vCPU, 8 GiB RAM)."
      echo "  Foreground by design: Ctrl-C stops it."

      # Split edk2/OVMF firmware must be attached as a pflash pair, not via
      # -bios. Unit 0 is the read-only CODE image; unit 1 is a per-run WRITABLE
      # copy of the VARS template, because the guest writes UEFI variables and
      # the template lives in a read-only prefix.
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
        # Single-blob firmware (e.g. OVMF.fd) still works with -bios.
        FIRMWARE_ARGS=(-bios "$FIRMWARE")
      fi

      # -nographic keeps the guest console on this terminal. No '&', no
      # daemonize: the VM is the last foreground command and its exit status
      # becomes ours.
      set +e
      "qemu-system-${VM_ARCH}" \
        -enable-kvm -machine "$VM_MACHINE" -cpu host -smp 4 -m 8192 \
        "${FIRMWARE_ARGS[@]}" \
        "${VM_DISK_ARGS[@]}" \
        -nic user,model=virtio \
        -chardev "socket,id=control,path=${BOOTSTRAP_SOCKET}" \
        -device "$SERIAL_DEVICE" \
        -device "virtserialport,chardev=control,name=org.projectbluefin.donate-clanker.bootstrap" \
        -nographic
      status=$?
      set -e
      exit "$status"
    fi
    [[ -n "{{vm_runner_image}}" ]] || {
      echo "ERROR: donate-clanker VM runner is not configured." >&2
      echo "  Set DONATE_CLANKER_VM_RUNNER_IMAGE to the signed immutable runner image reference." >&2
      exit 1
    }
    ensure_vm_runner
    require_no_running_instance donate-clanker-vm
    # --rm --interactive --tty and never --detach: the runner container owns
    # this terminal for as long as it lives.
    CONTAINER_ARGS=(
      podman run --rm --interactive --tty --replace --name donate-clanker-vm
      --device /dev/kvm
      --mount "type=bind,src=${RUN_DIR},dst=/run/donate-clanker,rw"
      --env "DONATE_CLANKER_BOOTSTRAP_SOCKET=/run/donate-clanker/${BOOTSTRAP_SOCKET_NAME}"
      --env "DONATE_CLANKER_RUN_ID=${RUN_ID}"
      --env "DONATE_CLANKER_VM=1"
      --env "AGENT_BACKEND=goose"
    )
    # Hive ignores AGENT_MODEL when the backend is Goose, so the model is
    # passed as GOOSE_MODEL — the variable Goose itself reads.
    [[ -n "$GOOSE_PROVIDER" ]] && CONTAINER_ARGS+=(--env "GOOSE_PROVIDER=${GOOSE_PROVIDER}")
    [[ -n "$GOOSE_MODEL" ]] && CONTAINER_ARGS+=(--env "GOOSE_MODEL=${GOOSE_MODEL}")
    CONTAINER_ARGS+=("{{vm_runner_image}}")
    echo "✓ donate-clanker is running in the pinned QEMU VM runner."
    echo "  Stop any time with Ctrl-C — that is the only way it ends."
    set +e
    "${CONTAINER_ARGS[@]}"
    status=$?
    set -e
    exit "$status"

# Run ONLY the contributor container — no VM — for quick local development.
# Same preflight and same provider/model selection as the VM path, minus the
# hardware isolation, so it is the fast loop while hacking on the image.
# Usage: ujust donate-clanker-container
donate-clanker-container:
    #!/usr/bin/env bash
    set -euo pipefail
    {{shared_functions}}
    TOOL="{{tool_env}}"

    CFG_DIR="${HOME}/.config/donate-clanker"
    LAST_FILE="${CFG_DIR}/last-selections.env"
    STATE_DIR="${HOME}/.local/state/donate-clanker"
    HIVE_SRC_DIR="${STATE_DIR}/hive-src"
    HIVE_REPO_URL="{{hive_repo_url}}"
    HIVE_COMMIT="${DONATE_CLANKER_HIVE_COMMIT:-{{hive_commit}}}"
    HIVE_COMMIT="${HIVE_COMMIT,,}"
    mkdir -p "${CFG_DIR}" "${STATE_DIR}"

    require_goose_backend "$TOOL"
    preflight_agent
    command -v podman &>/dev/null || {
      echo "ERROR: Podman is required to run the contributor container." >&2
      echo "  Install Podman, then re-run donate-clanker-container." >&2
      exit 1
    }

    resolve_goose_selection
    persist_goose_selection
    ensure_hive_contributor_env

    CONTAINER_NAME="donate-clanker-container"
    CONTRIBUTOR_IMAGE="{{contributor_image}}"
    require_no_running_instance "$CONTAINER_NAME"
    ensure_contributor_image "$CONTRIBUTOR_IMAGE"

    # Hive's own convention: the contributor config lives at
    # ~/.config/hive inside the image's home (/home/dev), read-only. There is
    # no /workspace and no /config mount — the agent clones what it needs.
    CONTAINER_ARGS=(
      podman run --rm --interactive --tty --replace --name "$CONTAINER_NAME"
      --volume "${HOME}/.config/hive:/home/dev/.config/hive:ro"
      --env "AGENT_BACKEND=goose"
    )
    # Hive deliberately ignores AGENT_MODEL for the Goose backend, which is
    # why the model is handed over as GOOSE_MODEL instead.
    [[ -n "$GOOSE_PROVIDER" ]] && CONTAINER_ARGS+=(--env "GOOSE_PROVIDER=${GOOSE_PROVIDER}")
    [[ -n "$GOOSE_MODEL" ]] && CONTAINER_ARGS+=(--env "GOOSE_MODEL=${GOOSE_MODEL}")
    # Hand over the Copilot credential so the agent can reach a model without a
    # human retyping a device code at every launch. Passed by value as an env
    # var rather than by mounting the host's Goose config: the container gets
    # exactly one secret and no view of the rest of that directory.
    if [[ "${GOOSE_PROVIDER:-}" == "github_copilot" ]]; then
      resolve_copilot_token
      if [[ -n "${COPILOT_TOKEN:-}" ]]; then
        CONTAINER_ARGS+=(--env "GITHUB_COPILOT_TOKEN=${COPILOT_TOKEN}")
        echo "✓ Copilot credential passed to the agent."
      else
        report_missing_copilot_credential
      fi
    fi
    # Hive's contributor model is fork + pull request under the contributor's
    # own identity, so the agent needs a GitHub token of its own. Passed by
    # value rather than by mounting ~/.config/gh, so the container sees one
    # credential for one host and nothing else that directory holds.
    resolve_gh_token
    if [[ -n "${GH_TOKEN_VALUE:-}" ]]; then
      CONTAINER_ARGS+=(--env "GH_TOKEN=${GH_TOKEN_VALUE}")
      report_gh_token_blast_radius "${GH_TOKEN_SOURCE}"
    else
      report_missing_gh_token
    fi
    CONTAINER_ARGS+=("$CONTRIBUTOR_IMAGE")

    echo "✓ starting the donate-clanker contributor container (no VM)."
    echo "  The entrypoint attaches to the 'contributor' tmux session for you."
    echo "  From a second terminal: podman exec -it ${CONTAINER_NAME} tmux attach -t contributor"
    echo "  Stop any time with Ctrl-C — that is the only way it ends."
    # exec: the container replaces this shell, so Ctrl-C and every other
    # signal reach it directly and its exit status is ours. Never --detach.
    exec "${CONTAINER_ARGS[@]}"

# Preflight check: is this machine actually ready for 'ujust donate-clanker'?
# Never starts the VM or the container — read-only diagnostics only.
donate-clanker-doctor:
    #!/usr/bin/env bash
    set -uo pipefail
    {{shared_functions}}
    HIVE_COMMIT="${DONATE_CLANKER_HIVE_COMMIT:-{{hive_commit}}}"
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
      echo "    (donate-clanker-container does not need it.)"
      fail=$((fail+1))
    fi
    check "gum installed (used for the provider/model pickers)" command -v gum
    # The VM path boots via UEFI, so missing firmware fails the launch late and
    # confusingly. Surface it here instead, where the fix is one brew command.
    if FIRMWARE_PATH="$(vm_firmware "$(vm_host_arch 2>/dev/null || echo x86_64)" 2>/dev/null)"; then
      echo "  ✓ UEFI firmware found: ${FIRMWARE_PATH}"
      pass=$((pass+1))
    else
      echo "  ✗ no UEFI firmware for the VM path"
      vm_firmware_hint
      echo "    (donate-clanker-container does not need it.)"
      fail=$((fail+1))
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
    # Read-only: resolves the token, prints its scopes, never its value, and
    # starts nothing. Without one the agent has no GitHub identity at all and
    # every assigned task stops at 'gh auth login'.
    resolve_gh_token
    if [[ -n "${GH_TOKEN_VALUE:-}" ]]; then
      echo "  ✓ a GitHub token is available for the agent (from ${GH_TOKEN_SOURCE}; not shown)"
      DOCTOR_GH_SCOPES="$(gh_token_scopes)"
      [[ -n "$DOCTOR_GH_SCOPES" ]] && echo "    The agent will be able to do anything this token can: ${DOCTOR_GH_SCOPES}"
      echo "    Narrow that with DONATE_CLANKER_GH_TOKEN=<scoped PAT> if that is wider than you want."
      pass=$((pass+1))
    else
      echo "  ✗ no GitHub token is available for the agent"
      echo "    It could not fork, push, or open a pull request, and would stop at 'gh auth login'."
      echo "    Run: ${GITHUB_LOGIN_COMMAND}, or export DONATE_CLANKER_GH_TOKEN."
      fail=$((fail+1))
    fi
    unset GH_TOKEN_VALUE
    echo ""

    echo "=== Agent backend (Goose only) ==="
    if tool_installed goose; then
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
    # Read-only: this only looks the credential up, never prints it, and never
    # starts anything. Both launch paths hand this exact token to the agent, so
    # a contributor can find out here — before a launch — whether the agent
    # will reach a model or sit on a device code.
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
      echo "    Override with DONATE_CLANKER_CONTRIBUTOR_IMAGE (a 'sha-' tag or digest pins a build), or build image/Containerfile locally."
      fail=$((fail+1))
    fi
    echo ""

    echo "=== Hive contributor setup ==="
    HIVE_CONTRIBUTOR_ENV="${HOME}/.config/hive/contributor.env"
    if [[ -f "$HIVE_CONTRIBUTOR_ENV" ]]; then
      echo "  ✓ ${HIVE_CONTRIBUTOR_ENV} exists"
      pass=$((pass+1))
      # Advisory only. An older contributor.env can still name a backend from
      # before this launcher was Goose-only. Hive is told the backend at launch,
      # so the stale line changes nothing — it just makes anyone reading the
      # file believe something untrue. The file is upstream's, so say so and
      # leave it alone.
      DOCTOR_BACKEND="$(hive_contributor_backend "$HIVE_CONTRIBUTOR_ENV")"
      if [[ -n "$DOCTOR_BACKEND" && "$DOCTOR_BACKEND" != "goose" ]]; then
        echo "  ! ${HIVE_CONTRIBUTOR_ENV} says AGENT_BACKEND=${DOCTOR_BACKEND}, but donate-clanker always launches goose."
        echo "    Harmless — the launcher passes AGENT_BACKEND=goose itself — but stale."
        echo "    Edit that line yourself if you want the file to match; donate-clanker will not touch it."
      fi
    else
      echo "  ✗ ${HIVE_CONTRIBUTOR_ENV} is missing"
      echo "    donate-clanker runs upstream 'just contribute-setup goose' from"
      echo "    kubestellar/hive @ ${HIVE_COMMIT:0:12} on first attended launch."
      fail=$((fail+1))
    fi
    echo ""

    echo "=== Guest repository model ==="
    echo "  ✓ assigned repositories are cloned inside the disposable guest"
    echo ""
    echo "${pass} checks passed, ${fail} failed."
    [[ "$fail" -eq 0 ]] || exit 1
