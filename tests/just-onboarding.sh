#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scratch="${repo_root}/.just-onboarding-scratch-$$-$(date +%s%N)"
trap 'rm -rf "$scratch"' EXIT
fake_bin="$scratch/bin"
home="$scratch/home"
cfg_dir="$home/.config/donate-clanker"
gum_log="$scratch/gum.log"
runner_log="$scratch/runner.log"
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
cat >"$fake_bin/podman" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${RUNNER_LOG:?}"
exit 97
EOF
chmod +x "$fake_bin"/*

run_launcher() {
  env \
    -u TOOL -u DONATE_CLANKER_VM_RUNNER_IMAGE -u DONATE_CLANKER_HIVE_COMMIT \
    -u AGENT_MODEL -u GOOSE_PROVIDER -u GOOSE_MODEL \
    HOME="$home" PATH="$fake_bin:$PATH" \
    GUM_LOG="$gum_log" RUNNER_LOG="$runner_log" \
    DONATE_CLANKER_VM_RUNNER_IMAGE="ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
    "$@" \
    "$real_just" --justfile "$repo_root/just/61-donate-clanker.just" donate-clanker
}

assert_contains() { grep -Fq -- "$1" <<<"$2"; }
assert_file_contains() { grep -Fq -- "$1" "$2"; }
assert_file_not_contains() { ! grep -Fq -- "$1" "$2"; }

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
# Goose for provider and model. The process reaches the QEMU VM runner boundary.
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
: >"$runner_log"
set +e
interactive="$(run_launcher GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 GUM_TOOL_RESPONSE=goose GUM_PROVIDER_RESPONSE=openai GUM_INPUT_RESPONSE=claude-3-5-sonnet 2>&1)"
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
assert_file_contains "run --rm --interactive --tty --name donate-clanker-vm --device /dev/kvm --mount type=bind,src=${home}/.local/state/donate-clanker/runs/" "$runner_log"
assert_file_contains "--env DONATE_CLANKER_BOOTSTRAP_SOCKET=/run/donate-clanker/bootstrap-" "$runner_log"
assert_file_contains "--env DONATE_CLANKER_VM=1" "$runner_log"
assert_file_contains "ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:" "$runner_log"
assert_file_not_contains "--env-file" "$runner_log"
assert_file_not_contains "${home}/.config/hive/contributor.env" "$runner_log"

# One ready tool is selected without a chooser.
rm -f "$home/.config/goose/config.yaml"
: >"$gum_log"
set +e
single="$(run_launcher 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'TOOL not set — auto-detected: claude' "$single"
test ! -s "$gum_log"

# More than one ready tool without an attended gum prompt fails with an
# actionable explicit override instead of silently choosing one.
write_goose_config
set +e
multiple_without_gum="$(run_launcher GH_READY=1 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains "multiple CLIs are ready (claude copilot goose) and 'gum' isn't available to prompt." "$multiple_without_gum"
assert_contains 'TOOL=<name> ujust donate-clanker' "$multiple_without_gum"

# Explicit legacy selection retains its independent remembered model state.
: >"$gum_log"
set +e
legacy="$(run_launcher TOOL=claude DONATE_CLANKER_TEST_GUM_TTY=1 GUM_INPUT_RESPONSE=claude-sonnet-test 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_file_contains 'input --value' "$gum_log"
assert_file_contains 'LAST_MODEL_CLAUDE=claude-sonnet-test' "$cfg_dir/last-selections.env"
assert_file_contains 'AGENT_MODEL=claude-sonnet-test' "$cfg_dir/secrets.env"

assert_file_not_contains 'Lima' "$runner_log"
