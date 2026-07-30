#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scratch="${repo_root}/.just-onboarding-scratch-$$-$(date +%s%N)"
trap 'rm -rf "$scratch"' EXIT
fake_bin="$scratch/bin"
home="$scratch/home"
cfg_dir="$home/.config/donate-clanker"
gum_log="$scratch/gum.log"
lima_log="$scratch/lima.log"
brew_log="$scratch/brew.log"
real_just="$(command -v just)"
mkdir -p "$fake_bin" "$home/.config/goose" "$home/.config/hive" "$cfg_dir"

cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
[[ "${GH_READY:-}" == "1" ]] || exit 1
EOF
cat >"$fake_bin/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$fake_bin/goose" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$fake_bin/gum" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${GUM_LOG:?}"
case "${1:-}" in
  input) printf '%s\n' "${GUM_INPUT_RESPONSE:-}" ;;
  choose)
    case "$*" in
      *"Multiple AI CLIs"*) printf '%s\n' "${GUM_TOOL_RESPONSE:-}" ;;
      *"Goose provider"*) printf '%s\n' "${GUM_PROVIDER_RESPONSE:-}" ;;
      *) printf '%s\n' "${GUM_CHOOSE_RESPONSE:-}" ;;
    esac
    ;;
  *) exit 1 ;;
esac
EOF
cat >"$fake_bin/git" <<'EOF'
#!/usr/bin/env bash
exit 97
EOF
cat >"$fake_bin/limactl-template" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${LIMA_LOG:?}"
case "${1:-}" in
  list)
    [[ "${LIMA_INSTANCE_EXISTS:-}" == "1" ]] && printf '%s\n' donate-clanker
    ;;
  shell) exit 97 ;;
esac
EOF
cp "$fake_bin/limactl-template" "$fake_bin/limactl"
cat >"$fake_bin/brew" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${BREW_LOG:?}"
if [[ "${1:-}" == "install" && "${2:-}" == "lima" ]]; then
  cp "${FAKE_BIN:?}/limactl-template" "${FAKE_BIN}/limactl"
  chmod +x "${FAKE_BIN}/limactl"
fi
EOF
chmod +x "$fake_bin"/*

run_launcher() {
  env \
    -u TOOL -u CLANKER_SRC -u DONATE_CLANKER_HIVE_COMMIT \
    -u AGENT_MODEL -u GOOSE_PROVIDER -u GOOSE_MODEL \
    HOME="$home" PATH="$fake_bin:$PATH" \
    GUM_LOG="$gum_log" LIMA_LOG="$lima_log" BREW_LOG="$brew_log" FAKE_BIN="$fake_bin" \
    "$@" \
    "$real_just" --justfile "$repo_root/just/61-donate-clanker.just" donate-clanker
}

assert_contains() { grep -Fq "$1" <<<"$2"; }
assert_file_contains() { grep -Fq "$1" "$2"; }
assert_file_not_contains() { ! grep -Fq "$1" "$2"; }

write_goose_config() {
  cat >"$home/.config/goose/config.yaml" <<'EOF'
provider: openai
base_url: http://127.0.0.1:11434/v1
api_key: local-test-key
model: llama3.1
EOF
}

cat >"$home/.config/hive/contributor.env" <<'EOF'
HIVE_REGISTRATION_TOKEN=super-secret-registration-token
HIVE_HUB=wss://example.invalid/contribute
CONTRIBUTOR_ID=test-contributor
CONTRIBUTOR_USERNAME=test-user
AGENT_BACKEND=goose
EOF
write_goose_config
mkdir -p "$home/.claude"

# Multiple ready tools use gum, restore the remembered default, then prompt
# Goose for provider and model. The process reaches the Lima guest boundary.
cat >"$cfg_dir/last-selections.env" <<'EOF'
LAST_TOOL=claude
LAST_GOOSE_PROVIDER=ollama
LAST_GOOSE_MODEL=llama3.1
EOF
cat >"$cfg_dir/secrets.env" <<'EOF'
GOOSE_PROVIDER=stale-provider
GOOSE_MODEL=stale-model
AGENT_MODEL=stale-model
EOF
: >"$gum_log"
: >"$lima_log"
set +e
interactive="$(run_launcher GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 GUM_TOOL_RESPONSE=goose GUM_PROVIDER_RESPONSE=openai GUM_INPUT_RESPONSE=claude-3-5-sonnet CLANKER_SRC="$repo_root" 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_file_contains 'choose --selected=claude --header Multiple AI CLIs are ready — pick one:' "$gum_log"
assert_file_contains 'choose --selected=ollama --header Goose provider' "$gum_log"
assert_file_contains 'input --value llama3.1' "$gum_log"
assert_file_contains 'LAST_TOOL=goose' "$cfg_dir/last-selections.env"
assert_file_contains 'LAST_GOOSE_PROVIDER=openai' "$cfg_dir/last-selections.env"
assert_file_contains 'LAST_GOOSE_MODEL=claude-3-5-sonnet' "$cfg_dir/last-selections.env"
assert_file_contains 'GOOSE_PROVIDER=openai' "$cfg_dir/secrets.env"
assert_file_contains 'GOOSE_MODEL=claude-3-5-sonnet' "$cfg_dir/secrets.env"
assert_file_not_contains 'stale-' "$cfg_dir/secrets.env"
test "$(stat -c '%a' "$cfg_dir/secrets.env")" = 600
assert_file_contains "start --name donate-clanker --tty=false --mount-only ${repo_root}:w --mount-only ${home}/.config/hive --mount-only ${home}/.config/goose template:podman" "$lima_log"
assert_file_contains "shell donate-clanker -- podman run --pull=always --rm --interactive --tty --userns=keep-id --name donate-clanker --workdir /workspace --volume ${home}/.config/hive:/config:ro --volume ${repo_root}:/workspace --env AGENT_BACKEND=goose --volume ${home}/.config/goose:/home/dev/.config/goose:ro --env GOOSE_PROVIDER=openai --env GOOSE_MODEL=claude-3-5-sonnet ghcr.io/projectbluefin/donate-clanker:stable" "$lima_log"

# One ready tool is selected without a chooser.
rm -f "$home/.config/goose/config.yaml"
: >"$gum_log"
set +e
single="$(run_launcher CLANKER_SRC="$repo_root" 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'TOOL not set — auto-detected: claude' "$single"
test ! -s "$gum_log"

# More than one ready tool without an attended gum prompt fails with an
# actionable explicit override instead of silently choosing one.
write_goose_config
set +e
multiple_without_gum="$(run_launcher GH_READY=1 CLANKER_SRC="$repo_root" 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains "multiple CLIs are ready (claude copilot goose) and 'gum' isn't available to prompt." "$multiple_without_gum"
assert_contains 'TOOL=<name> ujust donate-clanker' "$multiple_without_gum"

# Explicit legacy selection retains its independent remembered model state.
: >"$gum_log"
set +e
legacy="$(run_launcher TOOL=claude DONATE_CLANKER_TEST_GUM_TTY=1 GUM_INPUT_RESPONSE=claude-sonnet-test CLANKER_SRC="$repo_root" 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_file_contains 'input --value' "$gum_log"
assert_file_contains 'LAST_MODEL_CLAUDE=claude-sonnet-test' "$cfg_dir/last-selections.env"
assert_file_contains 'AGENT_MODEL=claude-sonnet-test' "$cfg_dir/secrets.env"

# The VM is reused and no host engine is invoked.
: >"$lima_log"
set +e
reused_vm="$(run_launcher TOOL=claude CLANKER_SRC="$repo_root" LIMA_INSTANCE_EXISTS=1 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_file_contains 'list --format {{.Name}}' "$lima_log"
assert_file_contains 'start donate-clanker' "$lima_log"
assert_file_not_contains 'template:podman' "$lima_log"

rm -f "$fake_bin/limactl"
: >"$lima_log"
: >"$brew_log"
set +e
bluefin_dx_install="$(run_launcher TOOL=claude CLANKER_SRC="$repo_root" DONATE_CLANKER_TEST_BLUEFIN_DX=1 PATH="$fake_bin:/usr/bin:/bin" 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'Lima is not installed; installing it with Homebrew...' "$bluefin_dx_install"
assert_file_contains 'install lima' "$brew_log"
assert_file_contains 'start --name donate-clanker' "$lima_log"
