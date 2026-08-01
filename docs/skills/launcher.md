---
name: launcher
version: "1.3"
last_updated: 2026-08-01
id: launcher
one_line_purpose: Change donate-clanker just recipes without breaking foreground.
entry_point: docs/skills/launcher.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [just, launcher, qemu, podman, foreground]
description: "Covers the three donate-clanker just recipes, the foreground guarantee (and why there is no stop command), the VM versus container-only modes, and why the launcher never filters Hive's work. Use when editing just/61-donate-clanker.just."
metadata:
  type: runbook
---

# Launcher

## When to Use

Load this before editing `just/61-donate-clanker.just`, adding or renaming a
recipe, changing how the VM runner container is invoked, or diagnosing a
launch that exits before the tmux session appears.

Not for how tasks arrive or why one was assigned -- that is
[`hive-runtime.md`](hive-runtime.md). Not for image layers or pinned digests
-- that is [`image-build.md`](image-build.md).

## Core Process

### The three public recipes

| Recipe | Behavior |
|---|---|
| `donate-clanker` | Boots the pinned QEMU VM in the foreground, attaches to tmux session `contributor`. Ctrl-C stops it. |
| `donate-clanker-container` | Runs only the container, no VM, attaches to the same session. Local development. |
| `donate-clanker-doctor` | Read-only preflight. Starts nothing. |

Both launch paths pass the contributor's Copilot credential to the agent, so
neither stalls on a device code: the container gets it as
`GITHUB_COPILOT_TOKEN`, the VM gets it as `provider_secret` inside the v2
bootstrap envelope on the one-shot socket. `donate-clanker-doctor` reports
whether one can be resolved at all, without printing it. See
[`goose-context.md`](goose-context.md) for why a `gh auth token` is not a
substitute.

Doctor also warns when `~/.config/hive/contributor.env` names a backend other
than `goose`. That file is upstream's and the launcher passes
`AGENT_BACKEND=goose` itself, so the stale line is harmless -- but it is
reported, never rewritten. Editing a user's file to silence a warning is worse
than the warning.

The launcher is Bash, and only Bash. A Go implementation of the same
preflight, Hive checkout, credential resolution and QEMU launch once lived
beside it in `cmd/` and `internal/`; nothing built or installed it, so it
drifted into a parallel universe that only its own tests visited, and it was
deleted. Anything the launcher does belongs in this file. If a helper feels
too big for Bash, that is a signal to make the launcher do less, not to add
a second language.

Everything else in the file is private (`_`-prefixed) recipes and variables,
deliberately. A user browsing the repository should find one file and three
commands, not a `bin/` of scripts they might run out of context.

A running container is not automatically a container in use. The launcher
distinguishes the two: if a foreground `podman run` client still holds the
name, somebody is working in that terminal and the launch refuses with the
attach command. If the container is running but its client is gone, the run
is an orphan nobody can reach or Ctrl-C, and the launch reclaims it silently
via `--replace`. Telling the user to run `podman rm -f` for the orphan case
would reintroduce the stop command through the back door.

There is deliberately no fourth recipe. A `donate-clanker-stop` used to exist
and was removed: a stop verb only has meaning when something can outlive the
terminal that started it, so shipping one contradicts the foreground
guarantee below and invites exactly the daemon this repository refuses to
become. Ctrl-C ends a run. A container name left behind by a hard-killed
terminal is reclaimed by the next launch via `--replace`, so cleanup happens
at startup rather than as a user-facing command. `tests/just-onboarding.sh`
fails the build if any `donate-clanker-{stop,start,restart,kill,clean,down,up}`
recipe reappears, or if the recipe count drifts from three.

### The launcher never chooses the work

The launcher boots a contributor and gets out of the way. It passes no
repository, label, issue or title selector to the guest, and it must never
grow one: Hive's `selectTask` decides what the agent works on, using the
hub's config and cooldown. A "just skip that repo" flag added here would
shadow selection that Hive already owns and would silently hide work the
maintainers admitted hub-side, where the policy is visible to everyone. If
the assigned mix is wrong, the fix is the hub's config, not this file. See
[`hive-runtime.md`](hive-runtime.md); `tests/just-onboarding.sh` fails the
build if selection logic appears in the launcher or the image entrypoint.

### The foreground guarantee

`donate-clanker` runs in the foreground and dies with its terminal. The
launcher `exec`s or waits on `podman run --rm --interactive --tty`, captures
the exit status, and exits with it. There is no daemon, no systemd unit, and
no state to reap afterward.

Anything that breaks this is a defect: `&`, `nohup`, `setsid`,
`podman run -d`, `--detach`, `systemd-run`, or a recipe that returns while
the guest is still alive.

### VM mode

VM mode requires `DONATE_CLANKER_VM_RUNNER_IMAGE` to name the signed
immutable runner image. Without it the recipe fails fast with a message
instead of falling back. The container is run with `--device /dev/kvm` and a
per-run control/overlay bind mount at `/run/donate-clanker`. Inside, QEMU
runs `-nographic` with a virtio serial control port.

The guest receives its per-run control directory and nothing else. No
workspace mount. No host home. No host configuration directory. Assigned
repositories are cloned inside the disposable VM.

### Container-only mode

Skips QEMU and runs the derived contributor image directly under podman.
Same tmux session name, same attach flow, much faster iteration. Use it when
testing image layers or Goose configuration; use VM mode when testing
isolation or anything KVM-dependent.

### Credential passthrough

`TOOL` selects the agent backend and is read from the environment, not as a
recipe parameter, because `just` parameters are positional rather than
`KEY=VALUE`. Goose is the only supported backend. `GOOSE_PROVIDER` and
`GOOSE_MODEL` are forwarded into the guest as `--env` flags when set.
Launcher state lives in the config directory: `last-selections.env` for the
previous run's choices and `secrets.env`, mode `0600`, for provider and model
values.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "Backgrounding it just while I test something." | The foreground guarantee is the product. A run nobody is watching is an unsupervised agent with a live GitHub token. |
| "A stop command would be more convenient." | It advertises a daemon. Ctrl-C ends a run; `--replace` reclaims a name at launch. |
| "This helper is too gnarly for Bash." | Then the launcher is doing too much. A second language becomes a second implementation nobody runs. |
| "Just skipping that one noisy repo." | Hive's `selectTask` owns selection. Filter hub-side, where maintainers can see the policy. |

## Red Flags

- A recipe that returns while the guest is still running.
- A new top-level public recipe. Three commands is the contract; add private
  helpers instead.
- A host path bind-mounted into the runner beyond the per-run control
  directory.
- Silent fallback when `DONATE_CLANKER_VM_RUNNER_IMAGE` is unset. Fail loudly.
- Duplicating anything Hive already does: session creation, prompt injection,
  output capture, or task selection.
- Any recipe, variable or `--env` that names a repository, label, title or
  issue to accept or skip. Hive drives; the launcher does not filter.
- Secrets echoed to stdout or written outside `secrets.env`.
- A reimplementation of any launcher step in another language, or a test
  harness whose only exercised path is its own `--self-test`. Both are
  parallel implementations that no user reaches.

## Verification

```bash
just --justfile just/61-donate-clanker.just --list
just --justfile just/61-donate-clanker.just donate-clanker-doctor
bash tests/just-onboarding.sh
git diff --check
pre-commit run --all-files
```

`--list` must show exactly the three public recipes. `doctor` must exit
non-zero when a prerequisite is missing and must never start a container.
Confirm the foreground guarantee by launching, backgrounding your terminal's
job, and verifying nothing survives an interrupt.
