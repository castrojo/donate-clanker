#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"
scratch="${repo_root}/.just-onboarding-scratch-$$-$(date +%s%N)"
tmp_root=".just-onboarding-tmp-$$-$(date +%s%N)"
trap 'rm -rf "$scratch" "$tmp_root"' EXIT
fake_bin="$scratch/bin"
home="$scratch/home"
cfg_dir="$home/.config/donate-clanker"
gum_log="$scratch/gum.log"
runner_log="$scratch/runner.log"
qemu_log="$scratch/qemu.log"
curl_log="$scratch/curl.log"
find_log="$scratch/find.log"
consumer="$repo_root/tests/guest-bootstrap-consumer.py"
real_just="$(command -v just)"
mkdir -p "$fake_bin" "$home/.config/goose" "$home/.config/hive" "$cfg_dir" "$tmp_root"

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
cat >"$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
url=""
while (($#)); do
  case "$1" in
    --output) output="$2"; shift 2 ;;
    *) url="$1"; shift ;;
  esac
done
printf '%s -> %s\n' "$url" "$output" >> "${CURL_LOG:?}"
if [[ "${TEST_CURL_MODE:-}" == "checksum-fail" && "$url" == *.sha256 ]]; then
  printf 'partial checksum\n' >"$output"
  exit 98
fi
if [[ -n "${TEST_CURL_MODE:-}" ]]; then
  printf 'downloaded VM\n' >"$output"
  exit 0
fi
echo "unexpected raw VM fetch" >&2
exit 98
EOF
cat >"$fake_bin/find" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${FIND_LOG:?}"
case " $* " in
  *"OVMF_CODE.fd"*|*"AAVMF_CODE.fd"*|*"QEMU_EFI.fd"*)
    printf '%s\n' /dev/null
    exit 0
    ;;
esac
exec /usr/bin/find "$@"
EOF
cat >"$fake_bin/uname" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "-m" && -n "${TEST_UNAME_M:-}" ]]; then
  printf '%s\n' "$TEST_UNAME_M"
else
  exec /usr/bin/uname "$@"
fi
EOF
cat >"$fake_bin/qemu-system-x86_64" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s %s\n' "$(basename "$0")" "$*" >> "${QEMU_LOG:?}"
exit 97
EOF
cp "$fake_bin/qemu-system-x86_64" "$fake_bin/qemu-system-aarch64"
cat >"$fake_bin/podman" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${RUNNER_LOG:?}"
mount_arg=""
bootstrap_socket=""
while (($#)); do
  case "$1" in
    --mount) mount_arg="${2:-}"; shift 2 ;;
    --env)
      case "${2:-}" in
        DONATE_CLANKER_BOOTSTRAP_SOCKET=*) bootstrap_socket="${2#*=}" ;;
      esac
      shift 2
      ;;
    *) shift ;;
  esac
done
if [[ -n "${RUNNER_CONSUMED:-}" && -n "$mount_arg" && -n "$bootstrap_socket" ]]; then
  state_dir="$(sed -E 's/^type=bind,src=(.*),dst=.*$/\1/' <<<"$mount_arg")"
  for _ in {1..50}; do
    socket_path="$(find "$state_dir" -maxdepth 1 -type s -print -quit)"
    [[ -n "$socket_path" ]] && break
    sleep 0.01
  done
  [[ -n "$socket_path" ]] || { echo "bootstrap socket was not created" >&2; exit 1; }
  DONATE_CLANKER_BOOTSTRAP_SOCKET="$socket_path" \
    DONATE_CLANKER_TEST_CONSUMED="${RUNNER_CONSUMED}" \
    python3 "${DONATE_CLANKER_TEST_CONSUMER:?}"
fi
exit 97
EOF
chmod +x "$fake_bin"/*

run_launcher() {
  env \
    -u TOOL -u DONATE_CLANKER_VM_RUNNER_IMAGE -u DONATE_CLANKER_HIVE_COMMIT \
    -u AGENT_MODEL -u GOOSE_PROVIDER -u GOOSE_MODEL \
    HOME="$home" PATH="$fake_bin:$PATH" TMPDIR="$tmp_root" \
    GUM_LOG="$gum_log" RUNNER_LOG="$runner_log" QEMU_LOG="$qemu_log" CURL_LOG="$curl_log" FIND_LOG="$find_log" RUNNER_CONSUMED="$scratch/runner-consumed" \
    DONATE_CLANKER_TEST_CONSUMER="$consumer" \
    DONATE_CLANKER_VM_RUNNER_IMAGE="ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
    "$@" \
    "$real_just" --justfile "$repo_root/just/61-donate-clanker.just" donate-clanker
}

assert_contains() { grep -Fq -- "$1" <<<"$2"; }
assert_file_contains() { grep -Fq -- "$1" "$2"; }
assert_file_not_contains() { ! grep -Fq -- "$1" "$2"; }
assert_file_not_exists() { [[ ! -e "$1" ]]; }

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
assert_file_contains 'choose --selected=Ollama --header Goose provider' "$gum_log"
assert_file_contains 'input --value llama3.1' "$gum_log"
assert_file_contains 'LAST_TOOL=goose' "$cfg_dir/last-selections.env"
assert_file_contains 'LAST_GOOSE_PROVIDER=openai' "$cfg_dir/last-selections.env"
assert_file_contains 'LAST_GOOSE_MODEL=claude-3-5-sonnet' "$cfg_dir/last-selections.env"
assert_file_contains 'GOOSE_PROVIDER=openai' "$cfg_dir/secrets.env"
assert_file_contains 'GOOSE_MODEL=claude-3-5-sonnet' "$cfg_dir/secrets.env"
assert_file_not_contains 'stale-' "$cfg_dir/secrets.env"
test "$(stat -c '%a' "$cfg_dir/secrets.env")" = 600
assert_file_contains "run --rm --interactive --tty --name donate-clanker-vm --device /dev/kvm --mount type=bind,src=${tmp_root}/donate-clanker-" "$runner_log"
assert_file_contains "--env DONATE_CLANKER_BOOTSTRAP_SOCKET=/run/donate-clanker/bootstrap-" "$runner_log"
assert_file_contains "--env DONATE_CLANKER_VM=1" "$runner_log"
assert_file_contains "ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:" "$runner_log"
assert_file_not_contains "--env-file" "$runner_log"
assert_file_not_contains "${home}/.config/hive/contributor.env" "$runner_log"
test -f "$scratch/runner-consumed"
assert_file_contains "acknowledged" "$scratch/runner-consumed"
test -z "$(find "$tmp_root" -maxdepth 1 -name 'donate-clanker-*' -user "$(id -un)" -print -quit 2>/dev/null)"
assert_file_not_contains "super-secret-registration-token" "$runner_log"

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

# A fetched raw disk is removed when its checksum sidecar download fails.
rm -rf "$home/.local/state"
: >"$curl_log"
set +e
fetch_failure="$(run_launcher TOOL=claude DONATE_CLANKER_VM_RUNNER_IMAGE= TEST_UNAME_M=x86_64 TEST_CURL_MODE=checksum-fail 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains "VM release checksum sidecar is not published yet" "$fetch_failure"
assert_file_contains "donate-clanker-vm-25.08.14-x86_64.raw" "$curl_log"
assert_file_not_exists "$home/.local/state/donate-clanker/donate-clanker-vm-25.08.14-x86_64.raw"
assert_file_not_exists "$home/.local/state/donate-clanker/donate-clanker-vm-25.08.14-x86_64.raw.partial"
assert_file_not_exists "$home/.local/state/donate-clanker/donate-clanker-vm-25.08.14-x86_64.raw.sha256"
assert_file_not_exists "$home/.local/state/donate-clanker/donate-clanker-vm-25.08.14-x86_64.raw.sha256.partial"

# A cached raw disk must have a checksum sidecar before it can boot.
state_dir="$home/.local/state/donate-clanker"
mkdir -p "$state_dir"
raw_x86="$state_dir/donate-clanker-vm-25.08.14-x86_64.raw"
raw_arm="$state_dir/donate-clanker-vm-25.08.14-aarch64.raw"
printf 'x86 guest\n' >"$raw_x86"
set +e
missing_sidecar="$(run_launcher TOOL=claude DONATE_CLANKER_VM_RUNNER_IMAGE= TEST_UNAME_M=x86_64 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains "VM raw disk checksum sidecar not found" "$missing_sidecar"

(
  cd "$state_dir"
  sha256sum "$(basename "$raw_x86")" >"$(basename "$raw_x86").sha256"
)
printf 'arm guest\n' >"$raw_arm"
(
  cd "$state_dir"
  sha256sum "$(basename "$raw_arm")" >"$(basename "$raw_arm").sha256"
)

# Local QEMU follows the host architecture, firmware, machine, and guest
# artifact instead of always selecting the x86_64 path.
: >"$qemu_log"
: >"$find_log"
set +e
local_x86="$(run_launcher TOOL=claude DONATE_CLANKER_VM_RUNNER_IMAGE= TEST_UNAME_M=x86_64 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains "booting local donate-clanker VM" "$local_x86"
assert_file_contains "qemu-system-x86_64" "$qemu_log"
assert_file_contains "-machine q35" "$qemu_log"
assert_file_contains "-bios /dev/null" "$qemu_log"
assert_file_contains "-device virtio-serial-pci" "$qemu_log"
assert_file_contains "$raw_x86" "$qemu_log"
assert_file_contains "-name OVMF_CODE.fd" "$find_log"

: >"$qemu_log"
: >"$find_log"
set +e
local_arm="$(run_launcher TOOL=claude DONATE_CLANKER_VM_RUNNER_IMAGE= TEST_UNAME_M=aarch64 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains "booting local donate-clanker VM" "$local_arm"
assert_file_contains "qemu-system-aarch64" "$qemu_log"
assert_file_contains "-machine virt" "$qemu_log"
assert_file_contains "-bios /dev/null" "$qemu_log"
assert_file_contains "-device virtio-serial-device" "$qemu_log"
assert_file_contains "$raw_arm" "$qemu_log"
assert_file_contains "-name AAVMF_CODE.fd" "$find_log"

# More than one ready tool without an attended gum prompt fails with an
# actionable explicit override instead of silently choosing one.
write_goose_config
set +e
multiple_without_gum="$(run_launcher GH_READY=1 2>&1)"
status=$?
set -e
test "$status" -ne 0
assert_contains "gum unavailable — auto-selected: goose" "$multiple_without_gum"

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
