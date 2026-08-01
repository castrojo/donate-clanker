# donate-clanker — Agent Operating Contract

`donate-clanker` is a thin, foreground launcher that boots a QEMU VM running a
derived KubeStellar Hive contributor image with Goose. It owns three things:
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
go test ./...
gofmt -l .
git diff --check
just --justfile just/61-donate-clanker.just --list
pre-commit run --all-files
```

`gofmt -l .` and `git diff --check` must produce no output. Run all five
before opening a pull request.

## Architecture and package boundaries

- `just/61-donate-clanker.just` — the only file that ships or installs. All
  four user-facing commands and their private helpers live here, on purpose.
- `image/` — `Containerfile` deriving `FROM ghcr.io/kubestellar/hive-contributor`
  pinned by digest, plus the layered config (`image/config/`).
- `cmd/`, `internal/` — Go helpers used during build and by the VM runner.
- `vm/`, `quadlet/`, `scripts/` — VM artifact manifest and verification.
- `docs/` — `SKILL.md` router and `skills/` catalog with `index.json`.

Hive unconditionally overwrites `~/.config/goose/config.yaml` at every
startup. `GOOSE_PATH_ROOT` points at `/opt/bluefin/goose` so our config — the
one declaring the Context7 MCP extension — survives. Do not "fix" this by
writing to `~/.config/goose`.

## What agents may touch

- `just/61-donate-clanker.just`
- `image/` including `Containerfile` and `image/config/`
- `cmd/`, `internal/`, `tests/`
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
- Never unpin the Hive contributor image digest or the Hive commit without an
  explicit human decision.

## PR rules

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

## Human decision gates

Stop and ask a human before:

- Changing the pinned Hive contributor image digest or `hive_commit`.
- Adding a second agent backend. Goose is the only supported backend.
- Adding any dependency, service, or background lifecycle.
- Changing credential handling or anything that touches `secrets.env`.
- Changing merge, ruleset, or required-status-check configuration.

Hive facts that constrain planning: task completion is self-reported and
places a 168-hour cooldown per issue whether the task passed or failed; the
scoped GitHub token Hive issues expires 55 minutes after assignment and is
never refreshed. Do not design flows that assume a longer credential life.

## Canonical sources

- Hive protocol, entrypoint, tmux contract: `kubestellar/hive`.
- Org skills and factory-wide agent rules: `projectbluefin/common`
  (`docs/skills/index.json` is the source of truth).
- External tool documentation: Context7, before asserting flags or APIs.
- This repository's own behavior: read `just/61-donate-clanker.just` and
  `image/Containerfile`. Do not assert launcher behavior from memory.
