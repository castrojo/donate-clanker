---
name: launcher
version: "1.6"
last_updated: 2026-08-02
id: launcher
one_line_purpose: Change donate-clanker just recipes without breaking foreground.
entry_point: docs/skills/launcher.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [just, launcher, qemu, podman, foreground]
description: "Maintains the three foreground donate-clanker recipes, VM and container-only launch paths, and credential boundaries. Use when editing just/61-donate-clanker.just."
metadata:
  type: runbook
---

# Launcher

## When to Use

Load this before editing `just/61-donate-clanker.just` or changing VM,
container-only, foreground, or credential-passthrough behavior.

## Core Process

1. Keep exactly three public recipes:

   | Recipe | Purpose |
   |---|---|
   | `donate-clanker` | Run the disposable QEMU VM in the foreground. |
   | `donate-clanker-container` | Run the contributor container directly for fast local iteration. |
   | `donate-clanker-doctor` | Perform read-only preflight checks. |

2. Keep both launch paths foreground. No detached Podman, background service,
   lifecycle command, or persistent launcher state beyond launcher
   configuration and verified caches is allowed. Ctrl-C is the stop mechanism;
   `--replace` only reclaims a container name when a new launch starts.
3. Keep VM isolation narrow. The guest receives per-run control data and
   clones assigned work itself; it does not receive a host workspace or home.
   VM disks are verified and use a disposable overlay.
4. Keep the container path narrow too. It mounts only the read-only Hive
   contributor configuration and runs the image entrypoint, which attaches to
   Hive's `contributor` session.
5. Keep Goose Copilot-only. The launcher resolves a Copilot credential for
   the provider secret and remembers only the selected model, never a secret
   or provider choice.
6. For container-only mode, pass Copilot and GitHub credentials by inherited
   environment (`--env NAME`), not command-line values or host configuration
   mounts. Resolve the GitHub token from the dedicated token, existing
   `GH_TOKEN`, then `gh auth token`.

The VM can carry the Copilot provider secret, but the current guest has no
compatible bootstrap mapping for a host `GH_TOKEN`. Use
`donate-clanker-container` when the task needs GitHub fork, push, or
pull-request access. Do not attempt to fix this by mounting GitHub
configuration or adding an unconsumed bootstrap field.

Hive selects every task. The launcher must not filter, skip, rank, or decline
assignments by repository, label, title, author, or issue.

## Red Flags

- A fourth public recipe or any start, stop, restart, kill, clean, or daemon
  command.
- `&`, `nohup`, `setsid`, `podman run -d`, `--detach`, or a service unit.
- A host directory mount beyond the per-run VM control path or read-only Hive
  configuration for container-only mode.
- A token in output, files, Podman arguments, or persistent launcher state.
- A second implementation of launcher behavior in another language.
- Task-selection policy outside Hive.

## Verification

```bash
just --justfile just/61-donate-clanker.just --list
just --justfile just/61-donate-clanker.just donate-clanker-doctor
bash tests/just-onboarding.sh
git diff --check
```

The recipe list must contain only the three public commands. Doctor must not
start a VM or container.

## Sources

- Podman environment inheritance: Context7 `/websites/podman_io_en`
