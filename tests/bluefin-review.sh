#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
mkdir -p "$scratch/bin"

cat >"$scratch/bin/goose" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >"${GOOSE_ARGS:?}"
exit 23
EOF
chmod +x "$scratch/bin/goose"

set +e
banner="$(PATH="$scratch/bin:$PATH" GOOSE_ARGS="$scratch/goose-args" \
  "$repo_root/image/bin/bluefin-review" --task 42)"
status=$?
set -e

expected_banner=$'+------------------------+\n| BLUEFIN REVIEW         |\n| HUMAN DECISION REQUIRED|\n+------------------------+'
[[ "$banner" == "$expected_banner" ]]
[[ "$status" -eq 23 ]]
[[ "$(cat "$scratch/goose-args")" == 'review --task 42' ]]
