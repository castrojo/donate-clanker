# Reviewing Runtime Design

## Goal

Make assigned contributor tasks immediately actionable by using a Hive runtime
that prepares the task repository before the agent's first work turn. Make
common validation capabilities available in the contributor image. Preserve
the VM identity limitation as an explicit compatibility error until a
compatible guest artifact is released.

## Architecture

Hive remains the only owner of task selection, claim handling, repository
routing, and checkout preparation. review pins the same Hive commit in
the host launcher and contributor image, so setup and runtime use one
protocol revision.

The contributor image adds only the small validation toolset used by common
repository workflows. The image entrypoint continues to keep credentials out
of image layers and reports its controlled startup configuration without
printing secrets.

The launcher sends its versioned one-shot VM bootstrap envelope. A guest that
does not advertise a compatible GitHub identity field receives no GitHub
token. The launcher reports that blocked prerequisite and directs users to the
container-only path for fork, push, and pull-request work.

## Flow

1. The launcher starts the pinned Hive contributor runtime.
2. Hive selects and claims a task, resolves its repository, and creates or
   reuses its checkout in the disposable contributor workspace.
3. The repository's `AGENTS.md` and skill routing are available before normal
   agent inspection begins.
4. The agent uses the image-provided validation tools or reports a specific
   unavailable command when a task needs something outside the small baseline.

## Failure Handling

Hive surfaces repository resolution, clone, permission, and branch failures
while retaining the claimed task. The launcher neither retries nor substitutes
another task. The VM path must fail before exposing any token if the guest
bootstrap capability is incompatible.

## Validation

Contract tests assert the host/image Hive pins match, the entrypoint retains
foreground lifecycle behavior, bootstrap data stays on the one-shot channel,
and no secrets enter Podman arguments or persistent files. Build the updated
contributor image with Podman after the contract suite passes.
