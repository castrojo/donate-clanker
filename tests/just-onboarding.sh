#!/usr/bin/env bash
# Hermetic regression harness for just/61-donate-clanker.just.
#
# Everything the launcher can shell out to (gh, goose, gum, podman, qemu,
# qemu-img, curl, find, brew, pgrep, uname) is faked on PATH, so this test
# never touches the network, never starts a real VM or container, and never
# depends on what happens to be installed on the developer's machine.
#
# The launcher under test is Goose-only: there is no claude/copilot/codex
# detection and no multi-CLI picker. Assertions here reflect that.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

# DONATE_CLANKER_TEST_JUSTFILE exists so the harness itself can be negative-
# tested against a deliberately broken copy of the launcher.
justfile="${DONATE_CLANKER_TEST_JUSTFILE:-$repo_root/just/61-donate-clanker.just}"
consumer="$repo_root/tests/guest-bootstrap-consumer.py"
real_just="$(command -v just)"

# Absolute scratch root: a relative TMPDIR used to leave stray
# .just-onboarding-tmp-* directories behind in the repository.
scratch="${repo_root}/.just-onboarding-scratch-$$-$(date +%s%N)"

# The launcher opens a UNIX socket under TMPDIR, and AF_UNIX paths are capped
# at ~107 bytes, so TMPDIR has to be both ABSOLUTE (a relative value used to
# leave stray directories in the repository) and short. Pick the first short
# writable base; everything picked here is removed by the EXIT trap.
tmp_root=""
for base in "${XDG_RUNTIME_DIR:-}" "/run/user/$(id -u)" "${HOME:-}/.cache"; do
  [[ -n "$base" && "$base" == /* && -d "$base" && -w "$base" ]] || continue
  candidate="${base}/.just-onboarding-$$"
  ((${#candidate} <= 45)) || continue
  tmp_root="$candidate"
  break
done
[[ -n "$tmp_root" ]] || tmp_root="${scratch}/tmp"
fake_bin="$scratch/bin"
home="$scratch/home"
cfg_dir="$home/.config/donate-clanker"
state_dir="$home/.local/state/donate-clanker"
firmware_dir="$scratch/firmware"
gum_log="$scratch/gum.log"
runner_log="$scratch/runner.log"
image_log="$scratch/image.log"
qemu_log="$scratch/qemu.log"
qemu_img_log="$scratch/qemu-img.log"
curl_log="$scratch/curl.log"
find_log="$scratch/find.log"
consumed_marker="$scratch/runner-consumed"

trap 'rm -rf "$scratch" "$tmp_root"' EXIT

mkdir -p "$fake_bin" "$tmp_root" "$firmware_dir" \
  "$home/.config/goose" "$home/.config/hive" "$cfg_dir" "$state_dir"

# ── failure reporting ─────────────────────────────────────────────────────
scenario="<startup>"
failures=0

begin() {
  scenario="$1"
  printf '• %s\n' "$scenario"
}
fail() {
  printf 'FAIL [%s]: %s\n' "$scenario" "$1" >&2
  failures=$((failures + 1))
  return 0
}
assert_contains() {
  grep -Fq -- "$1" <<<"$2" || fail "expected output to contain: $1
--- output ---
$2
--------------"
}
assert_not_contains() {
  grep -Fq -- "$1" <<<"$2" && fail "expected output NOT to contain: $1
--- output ---
$2
--------------"
  return 0
}
assert_file_contains() {
  grep -Fq -- "$1" "$2" || fail "expected $2 to contain: $1
--- $2 ---
$(cat "$2" 2>/dev/null)
--------------"
}
assert_file_not_contains() {
  grep -Fq -- "$1" "$2" && fail "expected $2 NOT to contain: $1
--- $2 ---
$(cat "$2" 2>/dev/null)
--------------"
  return 0
}
assert_file_exists() { [[ -e "$1" ]] || fail "expected file to exist: $1"; }
assert_file_not_exists() { [[ ! -e "$1" ]] || fail "expected file to be absent: $1"; }
assert_eq() { [[ "$1" == "$2" ]] || fail "${3:-value mismatch}: expected '$2', got '$1'"; }
assert_nonzero_status() { [[ "$1" -ne 0 ]] || fail "${2:-expected a non-zero exit status}"; }
assert_zero_status() { [[ "$1" -eq 0 ]] || fail "${2:-expected exit status 0, got $1}"; }

# ── fake PATH ─────────────────────────────────────────────────────────────
cat >"$fake_bin/gh" <<'EOF'
#!/usr/bin/env bash
[[ "${GH_READY:-}" == "1" ]] || exit 1
EOF
cat >"$fake_bin/goose" <<'EOF'
#!/usr/bin/env bash
[[ "${GOOSE_INSTALLED:-1}" == "1" ]] || exit 127
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
# brew is faked so vm_firmware's search roots can never resolve to a real
# Homebrew prefix on the developer's machine.
cat >"$fake_bin/brew" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "${FAKE_BREW_PREFIX:?}"
EOF
# pgrep is faked so the launcher can never signal a real process, and so a
# scenario can say whether a running container still has an owning terminal.
cat >"$fake_bin/pgrep" <<'EOF'
#!/usr/bin/env bash
[[ "${FAKE_PODMAN_OWNED:-0}" == 1 ]] && exit 0
exit 1
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
# The launcher's vm_firmware() searches several roots for a fixed list of
# firmware names. This fake answers for every supported name itself and never
# falls through to the real find for them, so a developer who genuinely has
# /home/linuxbrew/.linuxbrew/share/qemu/edk2-x86_64-code.fd cannot change the
# result. FAKE_FIRMWARE_MODE selects a split pflash pair or a single blob.
cat >"$fake_bin/find" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${FIND_LOG:?}"
answer() {
  [[ -f "${FAKE_FIRMWARE_DIR:?}/$1" ]] && printf '%s\n' "${FAKE_FIRMWARE_DIR}/$1"
  exit 0
}
case " $* " in
  *" edk2-x86_64-code.fd "*|*" edk2-aarch64-code.fd "*)
    [[ "${FAKE_FIRMWARE_MODE:-pflash}" == "pflash" ]] || exit 0
    case " $* " in
      *" edk2-x86_64-code.fd "*) answer edk2-x86_64-code.fd ;;
      *) answer edk2-aarch64-code.fd ;;
    esac
    ;;
  *" OVMF_CODE.fd "*|*" OVMF_CODE_4M.fd "*|*" AAVMF_CODE.fd "*)
    # Only the edk2 names are provided in pflash mode; these distro names
    # deliberately resolve to nothing so the name order stays observable.
    exit 0
    ;;
  *" OVMF.fd "*|*" QEMU_EFI.fd "*)
    [[ "${FAKE_FIRMWARE_MODE:-pflash}" == "bios" ]] || exit 0
    case " $* " in
      *" OVMF.fd "*) answer OVMF.fd ;;
      *) answer QEMU_EFI.fd ;;
    esac
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
cat >"$fake_bin/qemu-img" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${QEMU_IMG_LOG:?}"
target="${!#}"
: >"$target"
exit 0
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
# Image resolution is a separate concern from launching: it gets its own log
# so the 'exactly one foreground run' assertions stay meaningful, and it
# fails on demand so the missing-tag path can be exercised.
case "${1:-}" in
  image | manifest | pull)
    printf '%s\n' "$*" >>"${IMAGE_LOG:?}"
    [[ "${FAKE_PODMAN_IMAGE_MISSING:-0}" == 1 ]] && exit 1
    exit 0
    ;;
  inspect)
    # Only the liveness probe uses 'podman inspect'; nothing is running
    # unless a scenario asks for it.
    printf '%s\n' "$*" >>"${IMAGE_LOG:?}"
    [[ "${FAKE_PODMAN_RUNNING:-0}" == 1 ]] || { echo false; exit 1; }
    echo true
    exit 0
    ;;
esac
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
  socket_path=""
  for _ in {1..200}; do
    socket_path="$(/usr/bin/find "$state_dir" -maxdepth 1 -type s -print -quit)"
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

# Split pflash pair (CODE + VARS) plus a single-blob fallback firmware.
: >"$firmware_dir/edk2-x86_64-code.fd"
: >"$firmware_dir/edk2-i386-vars.fd"
: >"$firmware_dir/edk2-aarch64-code.fd"
: >"$firmware_dir/edk2-arm-vars.fd"
: >"$firmware_dir/OVMF.fd"
: >"$firmware_dir/QEMU_EFI.fd"

# ── fixtures ──────────────────────────────────────────────────────────────
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

reset_logs() {
  : >"$gum_log"
  : >"$runner_log"
  : >"$image_log"
  : >"$qemu_log"
  : >"$qemu_img_log"
  : >"$curl_log"
  : >"$find_log"
  rm -f "$consumed_marker"
}
reset_logs

# ── runner ────────────────────────────────────────────────────────────────
# run_recipe <recipe> [KEY=VALUE ...] — runs the launcher with a hermetic
# environment; sets OUT and STATUS.
run_recipe() {
  local recipe="$1"
  shift
  set +e
  OUT="$(
    env \
      -u TOOL -u DONATE_CLANKER_VM_RUNNER_IMAGE -u DONATE_CLANKER_HIVE_COMMIT \
      -u AGENT_MODEL -u GOOSE_PROVIDER -u GOOSE_MODEL -u GH_READY \
      -u GITHUB_COPILOT_TOKEN \
      -u TEST_UNAME_M -u TEST_CURL_MODE -u DONATE_CLANKER_TEST_GUM_TTY \
      -u DONATE_CLANKER_NON_INTERACTIVE -u GOOSE_INSTALLED \
      HOME="$home" PATH="$fake_bin:/usr/bin:/bin" TMPDIR="$tmp_root" \
      GUM_LOG="$gum_log" RUNNER_LOG="$runner_log" QEMU_LOG="$qemu_log" \
      IMAGE_LOG="$image_log" \
      QEMU_IMG_LOG="$qemu_img_log" CURL_LOG="$curl_log" FIND_LOG="$find_log" \
      RUNNER_CONSUMED="$consumed_marker" \
      DONATE_CLANKER_TEST_CONSUMER="$consumer" \
      DONATE_CLANKER_BOOTSTRAP_TIMEOUT=20 \
      DONATE_CLANKER_TEST_SKIP_VM_FETCH=1 \
      FAKE_BREW_PREFIX="$scratch/no-such-brew-prefix" \
      FAKE_FIRMWARE_DIR="$firmware_dir" \
      FAKE_FIRMWARE_MODE=pflash \
      DONATE_CLANKER_VM_RUNNER_IMAGE="ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" \
      "$@" \
      "$real_just" --justfile "$justfile" "$recipe" 2>&1
  )"
  STATUS=$?
  set -e
}

error_line_count() { grep -c '^ERROR:' <<<"$1" || true; }

kvm_usable() { [[ -e /dev/kvm && -r /dev/kvm && -w /dev/kvm ]]; }

# ══ 1. Preflight: exactly one actionable ERROR per failure ════════════════
begin "preflight: missing GitHub auth yields one actionable error"
run_recipe donate-clanker
assert_nonzero_status "$STATUS" "unauthenticated gh must fail the launch"
assert_eq "$(error_line_count "$OUT")" 1 "expected exactly one ERROR: line"
assert_contains "gh auth login" "$OUT"
assert_not_contains "claude" "$OUT"
assert_not_contains "copilot" "$OUT"
assert_not_contains "codex" "$OUT"

begin "preflight: missing Goose provider configuration yields one actionable error"
rm -f "$home/.config/goose/config.yaml"
run_recipe donate-clanker GH_READY=1
assert_nonzero_status "$STATUS" "an unconfigured goose must fail the launch"
assert_eq "$(error_line_count "$OUT")" 1 "expected exactly one ERROR: line"
assert_contains "goose configure" "$OUT"
assert_not_contains "claude" "$OUT"
assert_not_contains "codex" "$OUT"
write_goose_config

begin "preflight: an invalid Goose config (no provider) is treated as unconfigured"
printf 'model: llama3.1\n' >"$home/.config/goose/config.yaml"
run_recipe donate-clanker GH_READY=1
assert_nonzero_status "$STATUS" "a provider-less goose config must fail the launch"
assert_eq "$(error_line_count "$OUT")" 1 "expected exactly one ERROR: line"
assert_contains "goose configure" "$OUT"
write_goose_config

# ══ 2. TOOL handling: Goose only ══════════════════════════════════════════
begin "TOOL=claude is rejected with a Goose-only error"
run_recipe donate-clanker GH_READY=1 TOOL=claude
assert_nonzero_status "$STATUS" "a non-Goose TOOL must be a hard error"
assert_contains "TOOL=claude is not supported" "$OUT"
assert_contains "donate-clanker runs Goose only" "$OUT"
assert_not_contains "auto-detected" "$OUT"
assert_not_contains "Multiple AI CLIs" "$OUT"

begin "TOOL=goose is accepted"
run_recipe donate-clanker GH_READY=1 TOOL=goose
assert_nonzero_status "$STATUS" "the fake runner always exits non-zero"
assert_not_contains "is not supported" "$OUT"
assert_not_contains "Unset TOOL" "$OUT"

# ══ 3. VM runner path (container-hosted QEMU runner) ══════════════════════
if kvm_usable; then
  begin "VM runner: foreground podman run, v2 bootstrap handshake, no secrets"
  cat >"$cfg_dir/last-selections.env" <<'EOF'
LAST_GOOSE_PROVIDER=ollama
LAST_GOOSE_MODEL=llama3.1
EOF
  cat >"$cfg_dir/secrets.env" <<'EOF'
GOOSE_PROVIDER=stale-provider
GOOSE_MODEL=stale-model
AGENT_MODEL=stale-model
EOF
  reset_logs
  run_recipe donate-clanker GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 \
    GUM_PROVIDER_RESPONSE=OpenAI GUM_INPUT_RESPONSE=gpt-test
  assert_nonzero_status "$STATUS" "the fake runner always exits non-zero"

  # Picker memory: last run's provider/model preselected, new choice persisted.
  assert_file_contains 'choose --selected=Ollama --header Goose provider' "$gum_log"
  assert_file_contains 'input --value llama3.1' "$gum_log"
  assert_file_not_contains 'Multiple AI CLIs' "$gum_log"
  assert_file_contains 'LAST_GOOSE_PROVIDER=openai' "$cfg_dir/last-selections.env"
  assert_file_contains 'LAST_GOOSE_MODEL=gpt-test' "$cfg_dir/last-selections.env"
  assert_file_not_contains 'LAST_TOOL=' "$cfg_dir/last-selections.env"
  assert_file_contains 'GOOSE_PROVIDER=openai' "$cfg_dir/secrets.env"
  assert_file_contains 'GOOSE_MODEL=gpt-test' "$cfg_dir/secrets.env"
  assert_file_not_contains 'AGENT_MODEL=' "$cfg_dir/secrets.env"
  assert_file_not_contains 'stale-' "$cfg_dir/secrets.env"
  assert_eq "$(stat -c '%a' "$cfg_dir/secrets.env")" 600 "secrets.env must be 0600"

  # The runner is foreground and gets no host secret material.
  assert_file_contains "run --rm --interactive --tty --replace --name donate-clanker-vm" "$runner_log"
  assert_file_contains "--mount type=bind,src=${tmp_root}/donate-clanker-" "$runner_log"
  assert_file_contains "--env DONATE_CLANKER_BOOTSTRAP_SOCKET=/run/donate-clanker/bootstrap-" "$runner_log"
  assert_file_contains "--env DONATE_CLANKER_VM=1" "$runner_log"
  assert_file_contains "--env AGENT_BACKEND=goose" "$runner_log"
  assert_file_contains "--env GOOSE_PROVIDER=openai" "$runner_log"
  assert_file_contains "--env GOOSE_MODEL=gpt-test" "$runner_log"
  assert_file_contains "ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:" "$runner_log"
  assert_file_not_contains "--env-file" "$runner_log"
  assert_file_not_contains "--detach" "$runner_log"
  assert_file_not_contains "${home}/.config/hive/contributor.env" "$runner_log"
  assert_file_not_contains "super-secret-registration-token" "$runner_log"

  # The version-2 envelope reached the guest consumer and was acknowledged.
  assert_file_exists "$consumed_marker"
  assert_file_contains "acknowledged" "$consumed_marker"

  # No secret in any log this run, and the per-run directory is cleaned up.
  for log in "$runner_log" "$qemu_log" "$qemu_img_log" "$curl_log" "$find_log" "$gum_log"; do
    assert_file_not_contains "super-secret-registration-token" "$log"
  done
  assert_not_contains "super-secret-registration-token" "$OUT"
  assert_eq "$(/usr/bin/find "$tmp_root" -maxdepth 1 -name 'donate-clanker-*' -print -quit)" "" \
    "the per-run directory must be removed on exit"
else
  begin "VM runner: SKIPPED (/dev/kvm is not usable by this user)"
fi

# ══ Fetch/verify behaviour of the raw disk ════════════════════════════════
begin "fetch: a raw disk is removed when its checksum sidecar download fails"
rm -rf "$home/.local/state"
reset_logs
run_recipe donate-clanker GH_READY=1 DONATE_CLANKER_VM_RUNNER_IMAGE= \
  DONATE_CLANKER_TEST_SKIP_VM_FETCH=0 TEST_UNAME_M=x86_64 TEST_CURL_MODE=checksum-fail
assert_nonzero_status "$STATUS" "a failed sidecar download must fail the launch"
assert_contains "VM release checksum sidecar is not published yet" "$OUT"
assert_file_contains "donate-clanker-vm-25.08.14-x86_64.raw" "$curl_log"
assert_file_not_exists "$state_dir/donate-clanker-vm-25.08.14-x86_64.raw"
assert_file_not_exists "$state_dir/donate-clanker-vm-25.08.14-x86_64.raw.partial"
assert_file_not_exists "$state_dir/donate-clanker-vm-25.08.14-x86_64.raw.sha256"
assert_file_not_exists "$state_dir/donate-clanker-vm-25.08.14-x86_64.raw.sha256.partial"

begin "verify: a cached raw disk needs a checksum sidecar before it can boot"
mkdir -p "$state_dir"
raw_x86="$state_dir/donate-clanker-vm-25.08.14-x86_64.raw"
raw_arm="$state_dir/donate-clanker-vm-25.08.14-aarch64.raw"
printf 'x86 guest\n' >"$raw_x86"
run_recipe donate-clanker GH_READY=1 DONATE_CLANKER_VM_RUNNER_IMAGE= TEST_UNAME_M=x86_64
assert_nonzero_status "$STATUS" "a missing sidecar must fail the launch"
assert_contains "VM raw disk checksum sidecar not found" "$OUT"

(cd "$state_dir" && sha256sum "$(basename "$raw_x86")" >"$(basename "$raw_x86").sha256")
printf 'arm guest\n' >"$raw_arm"
(cd "$state_dir" && sha256sum "$(basename "$raw_arm")" >"$(basename "$raw_arm").sha256")

# ══ 6/7. Local QEMU: overlay boot, arch selection, firmware detection ═════
if kvm_usable; then
  begin "local QEMU (x86_64): boots a per-run overlay, never the master image"
  reset_logs
  run_recipe donate-clanker GH_READY=1 DONATE_CLANKER_VM_RUNNER_IMAGE= TEST_UNAME_M=x86_64
  assert_nonzero_status "$STATUS" "the fake qemu always exits non-zero"
  assert_contains "booting local donate-clanker VM" "$OUT"
  assert_file_contains "qemu-system-x86_64" "$qemu_log"
  assert_file_contains "-machine q35" "$qemu_log"
  assert_file_contains "-device virtio-serial-pci" "$qemu_log"
  assert_file_contains "-nographic" "$qemu_log"
  assert_file_contains "org.projectbluefin.donate-clanker.bootstrap" "$qemu_log"

  # The verified master image is copy-on-write backing only. Booting it
  # directly mutates it and breaks its checksum.
  assert_file_contains "create -q -f qcow2 -F raw -b $raw_x86" "$qemu_img_log"
  assert_file_contains "overlay.qcow2,format=qcow2,if=virtio" "$qemu_log"
  assert_file_not_contains "file=${raw_x86}" "$qemu_log"

  # Split pflash firmware: read-only CODE at unit 0, writable VARS at unit 1.
  assert_file_contains "-name edk2-x86_64-code.fd" "$find_log"
  assert_file_contains "if=pflash,format=raw,unit=0,readonly=on,file=${firmware_dir}/edk2-x86_64-code.fd" "$qemu_log"
  assert_file_contains "if=pflash,format=raw,unit=1,file=" "$qemu_log"
  assert_file_contains "efivars.fd" "$qemu_log"
  assert_file_not_contains "-bios" "$qemu_log"

  begin "local QEMU (aarch64): arch-specific machine, device and firmware"
  reset_logs
  run_recipe donate-clanker GH_READY=1 DONATE_CLANKER_VM_RUNNER_IMAGE= TEST_UNAME_M=aarch64
  assert_nonzero_status "$STATUS" "the fake qemu always exits non-zero"
  assert_file_contains "qemu-system-aarch64" "$qemu_log"
  assert_file_contains "-machine virt" "$qemu_log"
  assert_file_contains "-device virtio-serial-device" "$qemu_log"
  assert_file_contains "-name edk2-aarch64-code.fd" "$find_log"
  assert_file_contains "if=pflash,format=raw,unit=0,readonly=on,file=${firmware_dir}/edk2-aarch64-code.fd" "$qemu_log"
  assert_file_contains "create -q -f qcow2 -F raw -b $raw_arm" "$qemu_img_log"
  assert_file_not_contains "file=${raw_arm}" "$qemu_log"

  begin "local QEMU: single-blob firmware falls back to -bios"
  reset_logs
  run_recipe donate-clanker GH_READY=1 DONATE_CLANKER_VM_RUNNER_IMAGE= \
    TEST_UNAME_M=x86_64 FAKE_FIRMWARE_MODE=bios
  assert_nonzero_status "$STATUS" "the fake qemu always exits non-zero"
  assert_file_contains "-name edk2-x86_64-code.fd" "$find_log"
  assert_file_contains "-name OVMF.fd" "$find_log"
  assert_file_contains "-bios ${firmware_dir}/OVMF.fd" "$qemu_log"
  assert_file_not_contains "if=pflash" "$qemu_log"

  begin "local QEMU: no firmware anywhere is an actionable error"
  reset_logs
  run_recipe donate-clanker GH_READY=1 DONATE_CLANKER_VM_RUNNER_IMAGE= \
    TEST_UNAME_M=x86_64 FAKE_FIRMWARE_MODE=none
  assert_nonzero_status "$STATUS" "missing firmware must fail the launch"
  assert_contains "matching UEFI firmware for x86_64 was not found" "$OUT"
  assert_contains "brew install qemu" "$OUT"
  assert_eq "$(wc -c <"$qemu_log")" 0 "qemu must not be invoked without firmware"
else
  begin "local QEMU: SKIPPED (/dev/kvm is not usable by this user)"
fi

# ══ 4. Container recipe ═══════════════════════════════════════════════════
begin "donate-clanker-container: exactly one foreground podman run, hive mount only"
reset_logs
run_recipe donate-clanker-container GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 \
  GUM_PROVIDER_RESPONSE=OpenAI GUM_INPUT_RESPONSE=gpt-test
assert_nonzero_status "$STATUS" "the fake podman always exits non-zero"
assert_eq "$(wc -l <"$runner_log")" 1 "expected exactly one podman invocation"
assert_file_contains "run --rm --interactive --tty --replace --name donate-clanker-container" "$runner_log"
assert_file_contains "--volume ${home}/.config/hive:/home/dev/.config/hive:ro" "$runner_log"
assert_file_contains "--env AGENT_BACKEND=goose" "$runner_log"
assert_file_contains "--env GOOSE_PROVIDER=openai" "$runner_log"
assert_file_contains "--env GOOSE_MODEL=gpt-test" "$runner_log"
assert_file_contains "ghcr.io/projectbluefin/donate-clanker" "$runner_log"
assert_file_not_contains " -d " "$runner_log"
assert_file_not_contains "--detach" "$runner_log"
assert_file_not_contains "--env-file" "$runner_log"
assert_file_not_contains ":/config" "$runner_log"
assert_file_not_contains "/workspace" "$runner_log"
assert_file_not_contains "qemu" "$runner_log"
assert_file_not_contains "super-secret-registration-token" "$runner_log"
assert_eq "$(wc -c <"$qemu_log")" 0 "the container recipe must not start a VM"
assert_file_contains "image exists" "$image_log"

begin "donate-clanker-container: the Copilot credential is passed, never a gh token"
# Without this the agent starts a fresh device flow on every launch and the
# pane sits on "enter code XXXX-XXXX" until a human types one in.
reset_logs
run_recipe donate-clanker-container GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 \
  GUM_PROVIDER_RESPONSE="GitHub Copilot" GUM_INPUT_RESPONSE=gpt-4o \
  GITHUB_COPILOT_TOKEN=ghu-test-token
assert_contains "Copilot credential passed" "$OUT"
assert_file_contains "--env GITHUB_COPILOT_TOKEN=ghu-test-token" "$runner_log"

begin "donate-clanker-container: an unobtainable image is one actionable error"
reset_logs
run_recipe donate-clanker-container GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 \
  GUM_PROVIDER_RESPONSE=OpenAI GUM_INPUT_RESPONSE=gpt-test \
  FAKE_PODMAN_IMAGE_MISSING=1
assert_nonzero_status "$STATUS" "an unobtainable contributor image must fail the run"
assert_eq "$(error_line_count "$OUT")" 1 "expected exactly one ERROR line"
assert_contains "cannot obtain the contributor image" "$OUT"
assert_contains "there is no ':latest'" "$OUT"
assert_file_contains "pull" "$image_log"
assert_eq "$(wc -c <"$runner_log")" 0 "no container may start when the image is unobtainable"

# ══ 8. Stop recipe ════════════════════════════════════════════════════════
begin "donate-clanker-container: a live session is never replaced silently"
reset_logs
run_recipe donate-clanker-container GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 \
  GUM_PROVIDER_RESPONSE=OpenAI GUM_INPUT_RESPONSE=gpt-test \
  FAKE_PODMAN_RUNNING=1 FAKE_PODMAN_OWNED=1
assert_nonzero_status "$STATUS" "a container owned by another terminal must stop the relaunch"
assert_eq "$(error_line_count "$OUT")" 1 "expected exactly one ERROR line"
assert_contains "is already running in another terminal" "$OUT"
assert_contains "tmux attach -t contributor" "$OUT"
assert_not_contains "podman rm -f" "$OUT"
assert_eq "$(wc -c <"$runner_log")" 0 "a live session must never be replaced"

begin "donate-clanker-container: an orphaned run is reclaimed without a second command"
# Still running, but its terminal is gone: nobody can reach or Ctrl-C it, so
# the launch takes the name back instead of demanding manual cleanup.
reset_logs
run_recipe donate-clanker-container GH_READY=1 DONATE_CLANKER_TEST_GUM_TTY=1 \
  GUM_PROVIDER_RESPONSE=OpenAI GUM_INPUT_RESPONSE=gpt-test \
  FAKE_PODMAN_RUNNING=1 FAKE_PODMAN_OWNED=0
assert_contains "reclaiming" "$OUT"
assert_not_contains "ERROR:" "$OUT"
assert_not_contains "podman rm -f" "$OUT"
assert_file_contains "--replace --name donate-clanker-container" "$runner_log"

# ══ Doctor is read-only ═══════════════════════════════════════════════════
begin "donate-clanker-doctor: read-only, Goose-only diagnostics"
reset_logs
run_recipe donate-clanker-doctor GH_READY=1 TEST_UNAME_M=x86_64
assert_contains "Agent backend (Goose only)" "$OUT"
assert_not_contains "claude" "$OUT"
assert_not_contains "codex" "$OUT"
assert_eq "$(wc -c <"$qemu_log")" 0 "doctor must not start a VM"
assert_file_not_contains "run --rm" "$runner_log"

# ══ 5/6. Static guarantees read straight off the justfile ════════════════
begin "static: the launcher can never background a VM or a container"
# Comments in this file legitimately discuss --detach/nohup/setsid, so they
# are stripped before any of these greps run.
code="$scratch/justfile-code"
sed -E 's/^[[:space:]]*#.*$//' "$justfile" >"$code"

if grep -nE 'podman run.*(^| )(-d|--detach)( |$)' "$code"; then
  fail "podman run must never detach"
fi
# A lone trailing '&' backgrounds the launch; '&&' and '2>&1' must not match.
if grep -nE '(podman run|qemu-system-).*[^&>]&[[:space:]]*$' "$code"; then
  fail "a launch line must never end in a background '&'"
fi
if grep -nE '(^|[^[:alnum:]_])(nohup|setsid)([^[:alnum:]_]|$)' "$code"; then
  fail "nohup/setsid must never appear on a launch path"
fi
assert_eq "$(grep -c 'podman run --rm --interactive --tty' "$code")" 2 \
  "expected exactly two foreground podman run sites (VM runner + container)"
# A stale container from a hard-killed terminal must never block a relaunch.
assert_eq "$(grep -c 'podman run --rm --interactive --tty --replace --name' "$code")" 2 \
  "every named foreground run must reclaim its name with --replace"

begin "static: the verified master image is never booted directly"
# shellcheck disable=SC2016 # the launcher source is matched literally, not expanded
grep -q 'qemu-img create -q -f qcow2 -F raw -b "\$VM_RAW"' "$code" ||
  fail "the launcher must create a qcow2 overlay backed by \$VM_RAW"
# shellcheck disable=SC2016 # the launcher source is matched literally, not expanded
grep -q 'VM_DISK_ARGS=(-drive "file=\${VM_OVERLAY},format=qcow2,if=virtio")' "$code" ||
  fail "the launcher must boot the per-run overlay"
# shellcheck disable=SC2016 # the launcher source is matched literally, not expanded
if grep -n 'file=\${VM_RAW},format=raw' "$code"; then
  fail "the qemu invocation must never attach the master raw image"
fi

begin "static: the container never defaults to an unpublished ':latest' tag"
# publish-compat-image.yml only pushes sha-<commit>, the version tags and
# 'stable', so a ':latest' default is guaranteed 'manifest unknown'.
if grep -n 'donate-clanker:latest' "$code"; then
  fail "the default contributor image must be a tag the publish workflow actually pushes"
fi
grep -q 'ghcr.io/projectbluefin/donate-clanker:stable' "$code" ||
  fail "the default contributor image must be the published ':stable' tag"

begin "static: the launcher ships no lifecycle command"
# A stop/start/restart verb would mean a run can outlive its terminal. It
# cannot: Ctrl-C is the only way a donate-clanker run ends, and a stale name
# is reclaimed at launch by --replace, not by a second command.
if grep -nE '^donate-clanker-(stop|start|restart|kill|clean|down|up):' "$code"; then
  fail "the launcher must never ship a lifecycle recipe — Ctrl-C is the stop button"
fi
if grep -n 'ujust donate-clanker-stop' "$code"; then
  fail "nothing may point a user at a stop command that must not exist"
fi
# The recipe list is exactly: launch the VM, launch the container, diagnose.
assert_eq "$(grep -cE '^donate-clanker[a-z-]*:' "$code")" 3 \
  "expected exactly three recipes (donate-clanker, -container, -doctor)"

begin "static: no legacy backends survive in the launcher"
for legacy in copilot_live_models 'Multiple AI CLIs' LAST_TOOL AGENT_MODEL=; do
  if grep -Fn -- "$legacy" "$code"; then
    fail "legacy backend leftover found: $legacy"
  fi
done
assert_eq "$(grep -c '^tool_order := "goose"$' "$justfile")" 1 \
  "goose must be the only backend in tool_order"

# ══ result ════════════════════════════════════════════════════════════════
if [[ "$failures" -gt 0 ]]; then
  printf '\n%d assertion(s) FAILED.\n' "$failures" >&2
  exit 1
fi
printf '\nAll donate-clanker onboarding assertions passed.\n'
