# Copilot instructions for `projectbluefin/review`

`AGENTS.md` is the authoritative operating contract; read it first. This file
records the mistakes agents actually make in this repository so they are not
repeated.

## Read the docs before acting

`docs/skills/index.md` is the router and `docs/skills/index.json` the catalog.
Before changing the image, launcher, Hive integration, or PR workflow, read the
one matching skill document. `docs/factory/agentic-model.md` is the canonical
model — code, tests, and docs must agree with it.

These documents already answer most questions asked here. Sessions repeatedly
lost hours to work that a two-minute documentation read would have prevented.
Search `docs/` before designing anything.

## Image content belongs upstream at the BuildStream seam

`image/Containerfile` derives from `ghcr.io/projectbluefin/lab-runner`, which
BuildStream assembles in `projectbluefin/fsdk-containers` from
`elements/lab-runner/lab-runner-stack.bst`.

When a tool is missing from the container, add the FSDK component upstream and
bump `FSDK_RUNNER_IMAGE` here. That is the whole fix.

Never, as a substitute:

- `curl` a prebuilt binary into the image
- multi-stage `COPY` from a third-party image (busybox, alpine, etc.)
- add an `apt`/`dnf`/`apk` package overlay
- write a shell shim to paper over the absence
- invent a new intermediate base image (`review-base` was rejected outright)

Every one of these has been proposed and rejected. `tests/image-audit.sh`
enforces the policy. The `find` and `cmp` shims are grandfathered, not
precedent — see `docs/skills/image-build.md`.

## Do the smallest thing that satisfies the request

This is a toil-reduction factory, not a feature factory. Prefer one line to
fifty, a config change to a new subsystem, and an existing seam to a new one.

Do not re-litigate an explicit instruction. If the user has decided, implement
it; raise a concern once, briefly, then proceed. Judging that a change "will
land badly" is not your call to make unilaterally.

## "Merge" means merge

When asked to merge, clear the blocker rather than reporting it. A stale
digest in `tests/image-contract.sh`, a missing exec bit, or a formatting
failure is yours to fix. Only genuine policy gates (the Hive protocol gate,
required human review) are legitimate stopping points, and say so explicitly.

`gh pr merge` refusing while `mergeable=MERGEABLE` and all checks pass usually
means the PR is a draft — check `isDraft` and run `gh pr ready`.

## Validate the way CI does

```bash
bash scripts/check-skill-frontmatter.sh
bash tests/skill-conformance.sh
bash tests/generate-skills.sh
bash tests/image-contract.sh
bash tests/hive-compatibility.sh
bash tests/find-semantics.sh
bash tests/mcp-app-contract.sh
bash tests/bluefin-review.sh
bash tests/just-onboarding.sh
git diff --check
pre-commit run --all-files
pre-commit run shellcheck --hook-stage manual --all-files
```

Run a single suite directly with `bash tests/<name>.sh`.

**ShellCheck is a CI-only manual hook.** `pre-commit run --all-files` does not
include it, so it passes locally and then fails CI. Always run both lines.

New shell scripts need the executable bit (`git update-index --chmod=+x`) and
must be `shfmt`-clean — 2 spaces under `scripts/` and `tests/`, tabs under
`image/`. Missing either costs a CI round-trip.

`tests/image-audit.sh` needs a container engine and network; on a podman host
pass `CONTAINER_ENGINE=podman`.

Local `bst` builds fail at OCI assembly (`Staged artifacts do not provide
command 'sh'`). That is a sandbox limitation, not your change — wait for CI.

## Never disturb uncommitted work

This checkout usually has the user's work in progress. `git add -A`,
`git checkout --`, and `git stash` have all destroyed it here.

Commit from a clean worktree off `origin/main`:

```bash
git worktree add /tmp/<branch> -b <branch> origin/main
```

Stage explicit paths, never `-A`, in the main checkout. If WIP is lost,
recover via `git fsck --unreachable --no-reflogs | grep blob`.

## Launcher facts

The launcher is foreground-only and Ctrl-C is the stop mechanism; no `-d`,
`nohup`, or systemd units. `tests/just-onboarding.sh` enforces this statically.

`just` reads only the current directory's justfile, so review recipes fail from
other repositories. `~/.local/bin/just` and `~/.local/bin/ujust` shim
`review`, `review-container`, and `review-doctor` to `~/src/review/justfile`.

Rootless podman maps host uid 1000 to container **root**, not container uid
1000, so `0600` host files mount unreadable to the `dev` user. The launcher
passes `--userns keep-id:uid=1000,gid=1000`. Never loosen host file modes —
they hold Hive credentials.

Hive owns task selection. Never filter, reorder, or pick work. An idle
contributor reporting `no_matching_work` is usually an upstream queue with no
admitted work, not a broken container — see `docs/skills/hive-triage.md`.

## Record learnings

Durable, source-backed learnings go in the closest `docs/skills/` document,
with `docs/skills/index.json` updated in the same change. Do not add
changelogs, session notes, or plan documents.
