#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scratch="$(mktemp -d "${TMPDIR:-/tmp}/donate-clanker-just-test.XXXXXX")"
trap 'rm -rf "$scratch"' EXIT

fake_bin="$scratch/bin"
podman_log="$scratch/podman.log"
mkdir -p "$fake_bin"

cat >"$fake_bin/podman" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@" >"${PODMAN_LOG:?}"
exit 17
EOF
chmod +x "$fake_bin/podman"

home="$scratch/home"
workspace="$scratch/workspace"
mkdir -p "$home/.config/hive" "$home/.config/goose" "$workspace"

set +e
output="$(
  cd "$workspace"
  HOME="$home" \
    PATH="$fake_bin:$PATH" \
    PODMAN_LOG="$podman_log" \
    just --justfile "$repo_root/just/61-donate-clanker.just" donate-clanker 2>&1
)"
status=$?
set -e

test "$status" -eq 17
grep -Fqx -- "run" "$podman_log"
grep -Fqx -- "--rm" "$podman_log"
grep -Fqx -- "-it" "$podman_log"
grep -Fqx -- "--userns=keep-id" "$podman_log"
grep -Fqx -- "-e" "$podman_log"
grep -Fqx -- "AGENT_BACKEND=goose" "$podman_log"
grep -Fqx -- "GOOSE_PROVIDER=github_copilot" "$podman_log"
grep -Fqx -- "GOOSE_MODEL=gpt-5.6-luna" "$podman_log"
grep -Fqx -- "AGENT_MODEL=gpt-5.6-luna" "$podman_log"
grep -Fqx -- "GOOSE_DISABLE_KEYRING=1" "$podman_log"
grep -Fqx -- "GOOSE_TELEMETRY_ENABLED=false" "$podman_log"
grep -Fqx -- "-v" "$podman_log"
grep -Fqx -- "$home/.config/hive:/config:ro,Z" "$podman_log"
grep -Fqx -- "$home/.config/goose:/home/dev/.config/goose:Z" "$podman_log"
grep -Fqx -- "$workspace:/workspace:Z" "$podman_log"
grep -Fqx -- "localhost/donate-clanker:latest" "$podman_log"
! grep -Fq -- "DONATE_CLANKER_BOOTSTRAP_SOCKET" "$podman_log"
! grep -Fq -- "/dev/kvm" "$podman_log"
! grep -Fq -- "donate-clanker-vm-runner" "$podman_log"

printf '%s\n' "direct container launch passed"
printf '%s\n' "$output"
