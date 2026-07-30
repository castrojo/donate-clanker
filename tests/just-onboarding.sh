#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scratch="${repo_root}/.just-onboarding-scratch-$$-$(date +%s%N)"
trap 'rm -rf "$scratch"' EXIT
fake_bin="$scratch/bin"
home="$scratch/home"
cfg_dir="$home/.config/donate-clanker"
mkdir -p "$fake_bin" "$home/.config/goose" "$home/.config/hive" "$cfg_dir"

cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
[[ "${GH_READY:-}" == "1" ]] || exit 1
EOF
cat >"$fake_bin/goose" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$fake_bin/systemctl" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "$fake_bin/gh" "$fake_bin/goose" "$fake_bin/systemctl"

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
    "$@" \
    just --justfile "$repo_root/just/61-donate-clanker.just" donate-clanker
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
