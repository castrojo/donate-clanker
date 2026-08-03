# Reviewer Client Banner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display an exact ASCII human-decision banner when the managed reviewer client starts without changing Goose invocation behavior.

**Architecture:** Keep `image/bin/bluefin-review` as the sole reviewer-client wrapper. Replace its one identity line with the approved static banner and preserve its `exec goose review "$@"` handoff. Add an isolated shell test that substitutes `goose` on `PATH` to assert output, forwarded arguments, and exit status.

**Tech Stack:** Bash, existing repository shell-test conventions.

## Global Constraints

- Print the exact ASCII-only banner from `docs/superpowers/specs/2026-08-03-review-banner-design.md`.
- The banner is an orientation marker, not lifecycle evidence; do not add completion language.
- Do not change launcher, Hive, queue, persistence, credential, argument, or exit-status behavior.
- Preserve the wrapper invocation as `goose review "$@"`.

---

### Task 1: Add the reviewer banner and executable wrapper contract

**Files:**
- Modify: `image/bin/bluefin-review:2`
- Create: `tests/bluefin-review.sh`

**Interfaces:**
- Consumes: reviewer-client arguments supplied as `"$@"`.
- Produces: the exact three-line banner on standard output, then replaces the wrapper process with `goose review "$@"`.

- [ ] **Step 1: Write the failing wrapper test**

Create `tests/bluefin-review.sh` with a temporary fake `goose` executable that records its arguments and exits with status 23:

```bash
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bash tests/bluefin-review.sh`

Expected: FAIL because the wrapper prints `Tactical Ledger | Bluefin Review` instead of the exact banner.

- [ ] **Step 3: Replace the wrapper identity line with the banner**

In `image/bin/bluefin-review`, retain the shebang and `exec` handoff. Replace the existing `printf` with:

```bash
printf '%s\n' \
  '+------------------------+' \
  '| BLUEFIN REVIEW         |' \
  '| HUMAN DECISION REQUIRED|' \
  '+------------------------+'
exec goose review "$@"
```

- [ ] **Step 4: Run targeted and image-contract validation**

Run: `bash tests/bluefin-review.sh && bash tests/image-contract.sh && git diff --check`

Expected: the wrapper test passes, the image contract holds, and whitespace validation produces no output.

- [ ] **Step 5: Commit the implementation**

```bash
git add image/bin/bluefin-review tests/bluefin-review.sh
git commit -m "feat: add reviewer decision banner" \
  -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```
