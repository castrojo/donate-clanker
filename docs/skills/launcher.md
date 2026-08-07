---
name: launcher
version: "2.0"
last_updated: 2026-08-07
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
   lifecycle command, or persistent launcher state is allowed beyond the
   verified caches and pinned Hive checkout under `~/.local/state/review/`.
   Ctrl-C is the stop mechanism; `--replace` only reclaims a container name
   when a new launch starts.
3. Keep VM isolation narrow. The guest receives per-run control data and
   clones assigned work itself; it does not receive a host workspace or home.
   VM disks are verified and use a disposable overlay.
4. Keep the container path narrow too. It mounts only the read-only Hive
   contributor configuration and runs the image entrypoint, which attaches to
   Hive's `contributor` session.
5. Keep Goose Copilot-only. The launcher resolves a Copilot credential for the
   provider secret and recomputes the provider and model from the environment
   at every launch. Nothing is persisted: not a secret, not a provider, not a
   model. There is no last-selection file, and `tests/just-onboarding.sh`
   asserts one is never written.
   `review-container` must set its own thinking-effort default before forming
   the Podman environment, while still honoring `GOOSE_THINKING_EFFORT` from
   the caller. Do not apply that container default to the VM or replace the
   image's direct-invocation fallback.
   That default comes from the model profile: `review-container [profile]
   [effort]` resolves `luna` to `gpt-5.6-luna` at `max` with the provider's
   own context window, and `opus5` to `claude-opus-5` at `high` with
   `GOOSE_CONTEXT_LIMIT=264000`. An empty profile asks with `gum` when a
   terminal is attached and falls back to `luna` when one is not, so the
   headless path stays noninteractive. Profiles are defaults, never
   overrides: `GOOSE_MODEL`, `GOOSE_THINKING_EFFORT`, and
   `GOOSE_CONTEXT_LIMIT` from the environment always win.
6. For container-only mode, pass Copilot and GitHub credentials by inherited
   environment (`--env NAME`), not command-line values or host configuration
   mounts. Resolve the GitHub token from `REVIEW_GH_TOKEN`, existing
   `GH_TOKEN`, then `gh auth token`.
7. When renaming launcher-facing product identifiers, do a tracked-file sweep
   for both active names and legacy spellings in code, comments, workflow
   assertions, fixture image names, and environment variables. Keep only the
   live `review` / `REVIEW_*` surface; do not leave compatibility aliases
   behind.

## Mode Capabilities Are Not Symmetric

The two run modes are separate products, and the difference is user-visible:

| | `review` (VM) | `review-container` |
|---|---|---|
| Credential channel | one-shot `0600` AF_UNIX JSON bootstrap | inherited `--env NAME` |
| Copilot `provider_secret` | yes | yes |
| `GH_TOKEN` identity | no | yes |
| Fork, push, open a pull request | no | yes |
| Host mounts | none | `~/.config/hive`, read-only |
| Host requirements | qemu, qemu-img, UEFI firmware, python3, curl, zstd, `/dev/kvm` | podman |

The VM can carry the Copilot provider secret, but the current guest has no
compatible bootstrap mapping for a host `GH_TOKEN`; the launcher reports that
block unconditionally on both VM branches. Hive's task prompt is unconditional
and tells the agent to fork, push, and open a pull request with `GH_TOKEN`, so
VM mode can be assigned work it cannot complete. Document that honestly; do not
filter assignments to route around it. Use `review-container` when the task
needs GitHub fork, push, or pull-request access. Do not attempt to fix this by
mounting GitHub configuration or adding an unconsumed bootstrap field.

## Container Ownership

`podman run --rm -it` does **not** bind a container's lifetime to its client.
`conmon` supervises the container, survives the client, and reparents to
`systemd --user`, so a hard-killed terminal leaves a fully running, ownerless,
unreachable container — not merely an exited name. Inferring ownership from a
`pgrep` for the `podman run` command line cannot tell that apart from a live
session.

Ownership must therefore be proven, not guessed: stamp
`--label review.owner=<boot-id>:<client-pid>` at launch, and treat a container
as owned only when all three hold — the PID is alive, the boot id matches, and
that process still names the container in `/proc/<pid>/cmdline`. Anything else
is an orphan and is reclaimed silently at the next launch. Never answer an
ownerless container by telling a user to press Ctrl-C in a terminal that no
longer exists, and never reintroduce a user-facing stop or clean verb.

## Concurrent Instances

Every ownership check is keyed on the container name, so the name is what
scopes an instance. `REVIEW_CONTAINER_NAME` overrides the default
`review-container` and is the only supported way to run a second contributor
agent at the same time:

```bash
REVIEW_CONTAINER_NAME=review-container-2 just review-container opus5 high
```

Keep it to that one variable. Do not add a `--name` recipe parameter, instance
numbering, a multi-instance manager, or any registry of running instances;
that would be launcher state and task-selection surface this repository does
not have.

A name supplied by a user reaches `podman run --name` and both ownership
probes, so validate it against podman's own rule
(`[a-zA-Z0-9][a-zA-Z0-9_.-]*`) before launch rather than letting podman fail
late. Validate before the model picker and the Hive setup so a typo costs
nothing. The pre-marker `pgrep` fallback in `container_has_owner` builds a
regex from the name, and podman's rule allows `.`; escape it, or one
instance's heuristic can answer for a differently-named sibling. Every
user-facing message — the refusal, the attach hint, the reclaim line — must
name the container that was actually requested, or a second agent is told to
attach to the first one's session.

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
- A launch path whose final process is neither `exec`'d nor the last foreground
  command whose status propagates: `nohup`, `setsid`, `podman run -d`,
  `--detach`, a service unit, or a background job that outlives the run.
  A background job the shell `wait`s on and reaps by trap is not this, and
  removing one can break signal handling.
- A host directory mount beyond the per-run VM control path or read-only Hive
  configuration for container-only mode.
- A token in output, files, Podman arguments, or any persisted launcher file.
- Ownership inferred from `pgrep` rather than a label plus a live, same-boot,
  still-naming PID.
- A user-supplied container name reaching `podman run` or an ownership probe
  unvalidated, or a hint that names the default container instead of the one
  the caller asked for.
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
