---
name: launcher
version: "1.7"
last_updated: 2026-08-03
id: launcher
one_line_purpose: Change review just recipes without breaking foreground.
entry_point: docs/skills/launcher.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [just, launcher, qemu, podman, foreground]
description: "Maintains the three foreground review recipes, VM and container-only launch paths, and credential boundaries. Use when editing justfile."
metadata:
  type: runbook
  context7-sources: [/websites/podman_io_en]
---

# Launcher

## When to Use

Load this before editing `justfile` or changing VM,
container-only, foreground, or credential-passthrough behavior.

## When Not to Use

Do not use this for Hive task selection, contributor-session triage, Goose
configuration internals, or image-layer pinning. Those belong to the Hive,
Goose, or image build skill documents.

## Core Process

1. Keep exactly three public recipes:

   | Recipe | Purpose |
   |---|---|
   | `review` | Run the disposable QEMU VM in the foreground. |
   | `review-container` | Run the contributor container directly for fast local iteration. |
   | `review-doctor` | Perform read-only preflight checks. |

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
   mounts. Resolve the GitHub token from `REVIEW_GH_TOKEN`, existing
   `GH_TOKEN`, then `gh auth token`.
7. When renaming launcher-facing product identifiers, do a tracked-file sweep
   for both active names and legacy spellings in code, comments, workflow
   assertions, fixture image names, and environment variables. Keep only the
   live `review` / `REVIEW_*` surface; do not leave compatibility aliases
   behind.

The VM can carry the Copilot provider secret, but the current guest has no
compatible bootstrap mapping for a host `GH_TOKEN`. Use
`review-container` when the task needs GitHub fork, push, or
pull-request access. Do not attempt to fix this by mounting GitHub
configuration or adding an unconsumed bootstrap field.

Hive selects every task. The launcher must not filter, skip, rank, or decline
assignments by repository, label, title, author, or issue.

## Common Rationalizations

- "It's only a comment or test fixture." Workflow assertions, onboarding
  fixtures, and operator comments are part of the public launcher surface and
  must be rebranded with the code.
- "We can leave an alias for safety." This launcher's contract is a clean
  break; aliases preserve stale instructions and weaken test coverage.
- "Passing `--env NAME=value` is equivalent." For secrets it is not: inherited
  `--env NAME` avoids printing values into the Podman command line.

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
just --list
just review-doctor
bash tests/just-onboarding.sh
git diff --check
```

The recipe list must contain only the three public commands. Doctor must not
start a VM or container.

## Sources

- Podman environment inheritance: Context7 `/websites/podman_io_en`
