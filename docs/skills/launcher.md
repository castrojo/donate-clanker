---
name: launcher
version: "1.0"
last_updated: 2026-07-31
id: launcher
one_line_purpose: Change donate-clanker just recipes without breaking foreground.
entry_point: docs/skills/launcher.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [just, launcher, qemu, podman, foreground]
description: "Covers the three donate-clanker just recipes, the foreground guarantee (and why there is no stop command), and the VM versus container-only modes. Use when editing just/61-donate-clanker.just, adding a recipe, or debugging a launch that exits early."
metadata:
  type: runbook
---

# Launcher

## When to Use

Load this before editing `just/61-donate-clanker.just`, adding or renaming a
recipe, changing how the VM runner container is invoked, or diagnosing a
launch that exits before the tmux session appears.

## Core Process

### The three public recipes

| Recipe | Behavior |
|---|---|
| `donate-clanker` | Boots the pinned QEMU VM in the foreground, attaches to tmux session `contributor`. Ctrl-C stops it. |
| `donate-clanker-container` | Runs only the container, no VM, attaches to the same session. Local development. |
| `donate-clanker-doctor` | Read-only preflight. Starts nothing. |

Everything else in the file is private (`_`-prefixed) recipes and variables,
deliberately. A user browsing the repository should find one file and three
commands, not a `bin/` of scripts they might run out of context.

There is deliberately no fourth recipe. A `donate-clanker-stop` used to exist
and was removed: a stop verb only has meaning when something can outlive the
terminal that started it, so shipping one contradicts the foreground
guarantee below and invites exactly the daemon this repository refuses to
become. Ctrl-C ends a run. A container name left behind by a hard-killed
terminal is reclaimed by the next launch via `--replace`, so cleanup happens
at startup rather than as a user-facing command. `tests/just-onboarding.sh`
fails the build if any `donate-clanker-{stop,start,restart,kill,clean,down,up}`
recipe reappears, or if the recipe count drifts from three.

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

## Red Flags

- A recipe that returns while the guest is still running.
- A new top-level public recipe. Four commands is the contract; add private
  helpers instead.
- A host path bind-mounted into the runner beyond the per-run control
  directory.
- Silent fallback when `DONATE_CLANKER_VM_RUNNER_IMAGE` is unset. Fail loudly.
- Duplicating anything Hive already does: session creation, prompt injection,
  output capture.
- Secrets echoed to stdout or written outside `secrets.env`.

## Verification

```bash
just --justfile just/61-donate-clanker.just --list
just --justfile just/61-donate-clanker.just donate-clanker-doctor
go test ./...
gofmt -l .
git diff --check
pre-commit run --all-files
```

`--list` must show exactly the four public recipes. `doctor` must exit
non-zero when a prerequisite is missing and must never start a container.
Confirm the foreground guarantee by launching, backgrounding your terminal's
job, and verifying nothing survives an interrupt.
