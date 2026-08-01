# donate-clanker — Agent Operating Contract

`donate-clanker` is a thin, foreground launcher that boots a QEMU VM running a
self-owned FSDK-derived contributor image with Goose. It owns three things:
booting the VM in the foreground, passing contributor credentials to Goose,
and the review context payload. Nothing else.

## Read order

1. This file — repo rules, commands, boundaries.
2. [`docs/SKILL.md`](docs/SKILL.md) — task→skill routing table.
3. The single `docs/skills/*.md` entry that matches your task. Load one.

## Repository purpose and boundaries

Own no lifecycle Hive already owns. Hive owns the WebSocket contributor
protocol, task selection, the `contributor` tmux session, prompt injection,
and output capture. Do not reimplement, wrap, or shadow any of it.

This is duct tape, not a platform. Reject changes that add daemons, services,
background processes, or persistent state beyond launcher configuration.

## Build, test, and lint

```bash
bash tests/image-contract.sh
bash tests/just-onboarding.sh
git diff --check
just --justfile just/61-donate-clanker.just --list
pre-commit run --all-files
```

`git diff --check` must produce no output. Run all five before opening a
pull request.

## Architecture and package boundaries

- `just/61-donate-clanker.just` — the only file that ships or installs. All
  three user-facing commands and their private helpers live here, on purpose.
- `image/` — `Containerfile` deriving `FROM ghcr.io/projectbluefin/lab-runner`
  pinned by digest, then adding the pinned Hive contributor runtime plus the
  layered config (`image/config/`).
- `scripts/` — the build-time skills generator and the skill-doc linter.
- `tests/` — Bash contract checks over the launcher and the image.
- `docs/` — `SKILL.md` router and `skills/` catalog with `index.json`.

Hive unconditionally overwrites `~/.config/goose/config.yaml` at every
startup. `GOOSE_PATH_ROOT` points at `/opt/bluefin/goose` so our config — the
one declaring the Context7 MCP extension — survives. Do not "fix" this by
writing to `~/.config/goose`.

## What agents may touch

- `just/61-donate-clanker.just`
- `image/` including `Containerfile` and `image/config/`
- `tests/`
- `docs/`, `README.md`, `AGENTS.md`
- `.github/workflows/`

## What agents must not touch

- Never write to `ublue-os/*`. Those repositories are not ours.
- Never commit generated `.agents/skills/` content. Org skills are generated
  at image build time from `projectbluefin/common`'s `docs/skills/index.json`.
  The generator is the artifact; the output is not.
- Never weaken the foreground guarantee. `ujust donate-clanker` runs in the
  foreground and dies with its terminal. No `&`, no `nohup`, no `--detach`,
  no systemd unit, no `podman run -d`.
- Never add a lifecycle command. There is no `donate-clanker-stop`, and no
  start, restart, kill, or clean either. A stop verb is only meaningful when
  a run can outlive its terminal, so shipping one contradicts the line above
  and advertises a daemon this repository does not have. Ctrl-C is the stop
  button. Reclaim a name a hard-killed run left behind at launch time with
  `--replace`; cleanup is a startup concern, never a user-facing verb.
- Never filter the work Hive assigns. Hive's `selectTask` is the sole
  authority on what gets worked on: the hub's own config decides which
  repositories are in the pool, which titles, authors and labels are denied,
  and the 168-hour per-issue cooldown. Nothing here may filter, skip,
  re-order, prioritise or decline an assignment by repository, label, title,
  author, issue number, or any other property. Doing so would shadow a
  lifecycle Hive already owns, which the purpose above forbids, and it would
  silently hide work the maintainers deliberately admitted hub-side.
  `tests/just-onboarding.sh` fails the build if selection logic appears in
  the launcher or the image entrypoint.
- Never add a second implementation of what the launcher already does. A Go
  copy of the preflight, Hive checkout, credential resolution and QEMU launch
  once lived in `cmd/` and `internal/`; nothing built or installed it, so it
  drifted unnoticed and was deleted. The same goes for a test harness whose
  only exercised path is its own `--self-test`, and for a contract covering
  artifacts no workflow publishes. If a change is not reachable from
  `ujust donate-clanker`, `image/Containerfile`, or a CI step, it is dead on
  arrival — do not write it.

## PR rules

- `main` is protected by a repository ruleset: direct pushes are rejected and
  every change goes through a pull request passing `validate` and
  `conventional-title`. This includes docs-only changes.
- Pull request titles must follow Conventional Commits. Merges are
  squash-only and the squash commit inherits the PR title, so the title is
  the permanent commit message.
- One logical change per pull request.
- Branch naming for Hive-assigned work: `clanker/<task_id>`.
- Include a `Hive-Task-Id: <task_id>` trailer on commits produced from a Hive
  assignment.
- Link the issue with `Closes #NNN` when the work closes one.
- When behavior changes, update the matching `docs/skills/*.md` in the same
  pull request.

Git hooks at `/opt/bluefin/git-hooks`, wired via a global `core.hooksPath`,
are ergonomics only — `git commit --no-verify` bypasses them. Deterministic
enforcement is GitHub rulesets and required status checks. Do not treat a
green local hook run as a merge gate.

## This is a development tool

The people running donate-clanker are the people changing it. Optimise for
their loop, not for the ceremony a production cluster would want.

- `:stable` moves on every merge to main and is the launcher default. A
  contributor should never be debugging a bug that was fixed yesterday, and
  should never have to name a tag or a digest to get current code.
- A moving tag is re-pulled on every launch. "Already present locally" is not
  the same as current, and treating it as current silently pins people to
  stale images.
- Do not add gates, sign-offs, pins or manual steps to the contributor loop.
  If something must be pinned for reproducibility, that is what `sha-<commit>`
  tags and digests are for, via `DONATE_CLANKER_CONTRIBUTOR_IMAGE` -- an
  opt-in, not the default path.
- Ship it. Merge, let CI publish, test the published image. Do not invent
  manual verification steps that CI already performs.

Still worth a human conversation, because they change what the tool IS rather
than how fast it moves: adding a second agent backend (Goose is the only one),
and adding a dependency, service or background lifecycle.

Hive facts that constrain planning: task completion is self-reported and
places a 168-hour cooldown on that issue, while a reported *failure* records
no cooldown at all, so an issue that keeps failing is handed straight back
out; the scoped GitHub token Hive issues expires 55 minutes after assignment
and is never refreshed. Do not design flows that assume a longer credential
life.

## Canonical sources

- Hive protocol, entrypoint, tmux contract: `kubestellar/hive`.
- Org skills and factory-wide agent rules: `projectbluefin/common`
  (`docs/skills/index.json` is the source of truth).
- External tool documentation: Context7, before asserting flags or APIs.
- This repository's own behavior: read `just/61-donate-clanker.just` and
  `image/Containerfile`. Do not assert launcher behavior from memory.
