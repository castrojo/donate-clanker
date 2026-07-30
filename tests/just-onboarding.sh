#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scratch="${repo_root}/.just-onboarding-scratch-$$-$(date +%s%N)"
trap 'rm -rf "$scratch"' EXIT
fake_bin="$scratch/bin"
home="$scratch/home"
cfg_dir="$home/.config/donate-clanker"
gum_log="$scratch/gum.log"
git_log="$scratch/git.log"
inner_just_log="$scratch/inner-just.log"
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
cat >"$fake_bin/git" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${GIT_LOG:?}"
exit 97
EOF
cat >"$fake_bin/gum" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "${1:-}" >> "${GUM_LOG:?}"
case "${1:-}" in
  input) printf '%s\n' "${GUM_INPUT_RESPONSE:-}" ;;
  choose) printf '%s\n' "${GUM_CHOOSE_RESPONSE:-}" ;;
  *) exit 1 ;;
esac
EOF
cat >"$fake_bin/systemctl" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
cat >"$fake_bin/just" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "${INNER_JUST_LOG:?}"
exit 98
EOF
chmod +x "$fake_bin/gh" "$fake_bin/claude" "$fake_bin/goose" "$fake_bin/git" "$fake_bin/gum" "$fake_bin/systemctl" "$fake_bin/just"

run_launcher() {
  env \
    -u TOOL \
    -u CLANKER_SRC \
    -u DONATE_CLANKER_HIVE_COMMIT \
    -u AGENT_MODEL \
    -u GOOSE_PROVIDER \
    -u GOOSE_MODEL \
    -u CLANKER_CONTAINER_RUNTIME \
    -u CLANKER_STRICT_SANDBOX \
    HOME="$home" \
    PATH="$fake_bin:$PATH" \
    GUM_LOG="$gum_log" \
    GIT_LOG="$git_log" \
    INNER_JUST_LOG="$inner_just_log" \
    "$@" \
    "$real_just" --justfile "$repo_root/just/61-donate-clanker.just" donate-clanker
}

assert_contains() {
  local needle="$1" haystack="$2"
  grep -Fq "$needle" <<<"$haystack"
}

assert_not_contains() {
  local needle="$1" haystack="$2"
  ! grep -Fq "$needle" <<<"$haystack"
}

assert_file_not_contains() {
  local needle="$1" path="$2"
  ! grep -Fq "$needle" "$path"
}

assert_file_contains() {
  local needle="$1" path="$2"
  grep -Fq "$needle" "$path"
}

write_goose_config() {
  cat >"$home/.config/goose/config.yaml" <<'EOF'
provider: openai
base_url: http://127.0.0.1:11434/v1
api_key: local-test-key
model: llama3.1
EOF
}

cat >"$cfg_dir/last-selections.env" <<'EOF'
LAST_TOOL=goose
LAST_GOOSE_PROVIDER=ollama
LAST_GOOSE_MODEL=llama3.1
EOF

cat >"$cfg_dir/secrets.env" <<'EOF'
GOOSE_PROVIDER=ollama
GOOSE_MODEL=llama3.1
EOF

set +e
missing_auth="$(run_launcher 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'gh auth login --web --hostname github.com --scopes repo,read:org' "$missing_auth"
assert_not_contains 'goose configure' "$missing_auth"
assert_not_contains 'Legacy compatibility backends stay manual-only' "$missing_auth"
assert_not_contains 'contribute-setup goose' "$missing_auth"

set +e
explicit_goose_missing_auth="$(run_launcher TOOL=goose 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'ERROR: GitHub authentication is required for supported onboarding.' "$explicit_goose_missing_auth"
assert_contains 'gh auth login --web --hostname github.com --scopes repo,read:org' "$explicit_goose_missing_auth"
assert_not_contains 'ERROR: TOOL=goose is installed but not ready.' "$explicit_goose_missing_auth"
assert_not_contains 'goose configure' "$explicit_goose_missing_auth"
assert_not_contains 'contribute-setup goose' "$explicit_goose_missing_auth"

cat >"$home/.config/goose/config.yaml" <<'EOF'
provider: ollama
base_url: http://127.0.0.1:11434/v1
api_key: local-test-key
model: llama3.1
EOF

set +e
invalid_goose="$(run_launcher GH_READY=1 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'goose configure' "$invalid_goose"
assert_not_contains 'gh auth login --web --hostname github.com --scopes repo,read:org' "$invalid_goose"
assert_not_contains 'Legacy compatibility backends stay manual-only' "$invalid_goose"
assert_not_contains 'contribute-setup goose' "$invalid_goose"

write_goose_config
: >"$git_log"
: >"$inner_just_log"
rm -f "$home/.config/hive/contributor.env"

set +e
missing_hive_no_tty="$(run_launcher GH_READY=1 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'missing Hive setup at' "$missing_hive_no_tty"
assert_contains 'stdin/stdout/stderr are not attached to a terminal' "$missing_hive_no_tty"
assert_contains 'pre-seed it yourself from kubestellar/hive @' "$missing_hive_no_tty"
assert_contains 'just contribute-setup goose' "$missing_hive_no_tty"
test ! -s "$git_log"
test ! -s "$inner_just_log"

cat >"$home/.config/hive/contributor.env" <<'EOF'
HIVE_REGISTRATION_TOKEN=super-secret-registration-token
HIVE_HUB=wss://example.invalid/contribute
CONTRIBUTOR_ID=test-contributor
CONTRIBUTOR_USERNAME=test-user
AGENT_BACKEND=ollama
EOF

set +e
invalid_hive="$(run_launcher GH_READY=1 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'Re-run donate-clanker from an interactive terminal to complete pinned Hive setup.' "$invalid_hive"
assert_not_contains 'gh auth login --web --hostname github.com --scopes repo,read:org' "$invalid_hive"
assert_not_contains 'goose configure' "$invalid_hive"
assert_not_contains 'AGENT_BACKEND' "$invalid_hive"
assert_not_contains 'super-secret-registration-token' "$invalid_hive"

cat >"$home/.config/hive/contributor.env" <<'EOF'
HIVE_REGISTRATION_TOKEN=super-secret-registration-token
HIVE_HUB=wss://example.invalid/contribute
CONTRIBUTOR_ID=test-contributor
CONTRIBUTOR_USERNAME=test-user
AGENT_BACKEND=goose
EOF

set +e
valid_hive="$(run_launcher GH_READY=1 CLANKER_SRC="$repo_root" 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_file_not_contains 'LAST_GOOSE_PROVIDER=' "$cfg_dir/last-selections.env"
assert_file_not_contains 'LAST_GOOSE_MODEL=' "$cfg_dir/last-selections.env"
assert_not_contains 'GOOSE_PROVIDER=' "$(cat "$cfg_dir/secrets.env")"
assert_not_contains 'GOOSE_MODEL=' "$(cat "$cfg_dir/secrets.env")"

mkdir -p "$home/.claude"
: >"$gum_log"
set +e
legacy_prompt="$(run_launcher TOOL=claude DONATE_CLANKER_TEST_GUM_TTY=1 GUM_INPUT_RESPONSE=claude-sonnet-test CLANKER_SRC="$repo_root" 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains 'WARNING: no GH_TOKEN available' "$legacy_prompt"
assert_file_contains 'input' "$gum_log"
assert_file_contains 'LAST_MODEL_CLAUDE=claude-sonnet-test' "$cfg_dir/last-selections.env"
assert_file_contains 'AGENT_MODEL=claude-sonnet-test' "$cfg_dir/secrets.env"

: >"$gum_log"
set +e
goose_prompt="$(run_launcher TOOL=goose GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 AGENT_MODEL=forced-goose-model CLANKER_SRC="$repo_root" 2>&1)"
status=$?
set -e
test "$status" -ne 0
test ! -s "$gum_log"
assert_file_contains 'LAST_MODEL_CLAUDE=claude-sonnet-test' "$cfg_dir/last-selections.env"
assert_not_contains 'AGENT_MODEL=' "$(cat "$cfg_dir/secrets.env")"
