---
name: donate-clanker-worker-credential-boundary
description: Use when changing donate-clanker worker auth, Goose task launching, Hive credential flow, workspace mounts, or prompt construction around policy and assignment text.
metadata:
  context7-sources:
    - /containers/ramalama
    - /websites/podman_io_en
    - /websites/pkg_go_dev_go1_25_3
    - /lima-vm/lima
    - /charmbracelet/gum
---

# Donate Clanker Worker Credential Boundary

## When to Use

Use this when you touch:

- `internal/app`, `cmd/contributor`, or `internal/runner` auth flow
- worker/container mount wiring
- Hive credential loading or scrubbing
- Goose prompt construction or token injection
- redaction logic for worker output

## When NOT to Use

Do not use this for:

- helper-only RamaLama runtime changes that do not affect worker auth or mounts
- pure UI/docs edits unrelated to worker credentials
- Hive protocol changes that do not alter what the Goose process can see

## Core Process

1. Keep the worker mount set minimal: mount only `/workspace` into Goose; reserve `/cache/ramalama` for the helper container.
2. Validate and load Hive credentials on the host side before worker start.
3. Pass only the minimum worker env needed for the Hive client to reconnect: `HIVE_REGISTRATION_TOKEN`, `HIVE_WS_URL`, `AGENT_BACKEND`, model/provider settings, and `WORKSPACE`.
4. Immediately scrub host auth env from the contributor process after loading credentials: unset Hive config/token paths plus `GH_TOKEN`, `GITHUB_TOKEN`, `GH_CONFIG_DIR`, and `GITHUB_CONFIG_DIR`.
5. Give Goose only the active assignment's GitHub token via `GH_TOKEN`/`GITHUB_TOKEN`; never pass host GitHub config directories into Goose.
6. Keep local policy and Hive assignment as separate prompt sections. Policy must precede assignment with explicit headings, and the assignment body must remain verbatim under `Hive assignment (verbatim):`.
7. Native task execution must fail closed before Goose starts whenever the bundled contract cannot load, the required repository documents cannot be read, or the manifest-backed contract section cannot be injected ahead of the verbatim Hive assignment.
8. Validate contract manifests at use sites too, not just when decoding bundled JSON. Reject absolute/traversal paths, any symlink-resolved component outside the cleaned workspace, and non-regular document targets; surface failures using only manifest-relative paths.
9. Preserve redaction on both command output and surfaced errors. Redact `*_TOKEN=...`, YAML-style secret keys, and the full `Authorization:` line value.
10. For the published compatibility image, mount only `/config` and `/workspace`, source `/config/hive/gh-auth.env` into the upstream process environment, and attach the contributor tmux session to the invoking terminal.
11. Launcher auto-detection considers every ready CLI. Preserve the attended
    `gum choose` chooser when several are ready and `gum input --value` for
    remembered Goose provider/model prompts; persist only current nonblank
    launcher-managed model settings in the mode-`0600` selections state.
12. Run focused tests for `cmd/contributor`, `internal/app`, `internal/config`, `internal/runner`, and `internal/hive`, then run `go test ./...`.

## Self-Contained VM Boundary

The approved product path is a disposable, bootable QEMU microVM launched from
a pinned containerized runner on supported Bluefin base/DX hosts. The guest
clones each Hive-assigned repository internally; it receives no host workspace,
home directory, container socket, or tool configuration mount. Lima is not part
of this path, and microagent is not used.

The host sends only a versioned bootstrap envelope over the VM control channel.
Hive registration values are memory-only; the guest receives each
assignment-scoped GitHub token only for that clone/task and scrubs it before
cleanup. The guest uses outbound-only networking, a minimal device set, an
immutable base, and an ephemeral overlay. QEMU, the runner container, the
control channel, and the overlay are removed on every exit path.

The former Lima compatibility-image behavior is historical/migration context
only. Do not add new host workspace-sharing or Lima mounts when changing the
VM launcher.

## Local Observation Boundary

After each terminal native Goose result, the contributor writes one best-effort
newline-delimited JSON observation to local stderr before returning its Hive
report. The schema is exactly `schema_version`, `event`, `task_id`, `kind`,
`repo`, `number`, `started_at`, `finished_at`, `duration_ms`, and `outcome`.
`event` is `task_finished`; `outcome` is `success`, `failure`, or
`cancelled`.

Observations are local metadata, not telemetry or task transcripts. They must
never contain title, URL, prompt, credentials or tokens, model summary/output,
commands, or raw error text. Observation write failures are non-disruptive:
they must not alter the terminal Hive report.

The native runtime does not persist observations or send them to an external
service. Their retention ceiling is the lifetime and retention policy of the
stderr consumer outside this runtime.

Use `json.NewEncoder(writer).Encode(observation)` for the record: its
`func (enc *Encoder) Encode(v any) error` API appends the required newline.
Ignore that error at this best-effort boundary.

## Native Assignment Runtime

The native contributor owns one Hive assignment at a time. Each assignment
receives one fresh, isolated `goose run --no-session` and a task-specific
runtime directory; a token refresh must not restart an active Goose process.
Native execution must not create, select, inject keys into, or supervise tmux
sessions, panes, or windows.

Context7 is optional, credential-free public documentation assistance. Use it
only when current external documentation helps; an unavailable or empty lookup
must not block work that can proceed from local repository evidence.

Local pre-commit hooks provide fast feedback. CI remains the authoritative
quality gate for formatting, static policy checks, and repository validation.

## Helper Profile Launch Boundary

The FSDK RamaLama helper image currently establishes only its `ramalama` CLI
entrypoint, cache-store requirement, and consumer-owned configuration. It does
not publish a versioned invocation contract for translating a profile's
`context_size` or `runtime_args` into helper command arguments or environment
variables.

Treat catalog profiles as validated policy, not helper launch syntax. The
native launcher must reject `--profile` / `DONATE_CLANKER_PROFILE` during
option parsing, before auth/setup or pod creation, because a selected profile
would require unpublished helper-launch semantics. Do not guess
`ramalama serve` flags or introduce profile-derived environment variables.
`--profile-catalog` stays reserved until FSDK publishes an explicit versioned
helper launch contract. When that contract exists, forward only the validated
context size and explicit non-thinking runtime arguments through it, with
focused launcher and pod tests.

## Sources

RamaLama documents `--runtime-args` as arguments for its selected runtime and
documents `ctx_size` configuration for serving (source: Context7
`/containers/ramalama`). Those general CLI options do not establish an FSDK
helper-image contract for translating this catalog's `context_size` or
`runtime_args` into a helper invocation.

Go `encoding/json` `Encoder.Encode` appends a newline after each record
(source: Context7 `/websites/pkg_go_dev_go1_25_3`).

Podman `info` exposes the selected OCI runtime in host information and
supports Go-template formatting (source: Context7
`/websites/podman_io_en`).

The VM design in
`docs/superpowers/specs/2026-07-30-donate-clanker-vm-design.md` defines the
QEMU runner, one-shot control channel, guest clone lifecycle, and cleanup
boundary. It supersedes the earlier Lima compatibility path for new work.

Gum documents `gum choose` for option selection and `gum input --value`,
`--placeholder`, and `--header` for prefilled text prompts (source: Context7
`/charmbracelet/gum`).

## Common Rationalizations

- “Mounting the GitHub config dir is easier.”
  It also hands untrusted assignment text long-lived host auth state. Use the task token instead.
- “The worker can keep Hive env around after startup.”
  It only needs the parsed credentials in memory; scrubbing the process env shrinks the blast radius.
- “Policy and assignment can just be concatenated.”
  Without explicit boundaries, untrusted assignment text can blur local policy and task instructions.
- “Goose can discover the repository docs after it starts.”
  By then the untrusted assignment is already running; the contract documents must load and inject before any Goose command is invoked.
- “Redacting `Authorization:` up to the first space is enough.”
  Bearer-style values often include prefixes; redact the full line value.

## Red Flags

- Worker mounts include `/config/hive` or `/config/github`
- Goose env contains `HIVE_CONFIG_DIR`, `GITHUB_CONFIG_DIR`, or `GH_CONFIG_DIR`
- Host `GH_TOKEN` can flow into Goose without coming from the assignment
- Prompt text lacks separate policy/assignment headings
- Goose command launch can proceed after contract/document load failures, or the prompt omits the injected agent-contract section
- Hand-built contract manifests can skip validation, follow symlinks outside the workspace, accept non-regular targets, or leak absolute workspace paths in read errors
- Local observations include task content, credentials, model output, commands, or raw error text
- Native task execution reuses a Goose process or runtime directory across assignments, restarts an active task after token refresh, or manipulates tmux
- Multiple ready CLIs silently select one, Goose skips its provider/model
  prompts, or launcher state is not refreshed from the current selection
- Context7 availability can block assignment execution
- Local hooks are treated as a substitute for CI
- Tests only cover success paths and not env/mount exposure or redaction
- A foreground compatibility container starts a detached tmux session but never attaches it to the user's terminal
- Host Podman, Docker, systemd, or a container socket launches the product
  contributor outside the pinned QEMU runner
- The VM exposes a host workspace, home directory, socket, clipboard, GUI, or
  inbound service

## Verification

- [ ] The VM runner exposes no host workspace, home, or container socket
- [ ] Bootstrap uses only the versioned control-channel allowlist
- [ ] Guest state contains no persisted registration or assignment token
- [ ] Goose receives only assignment-scoped GitHub token values
- [ ] Missing required repository documents stop native task execution before any Goose command starts
- [ ] Prompt output preserves verbatim assignment text under a dedicated heading
- [ ] Valid workspaces inject the manifest-backed contract section before `Hive assignment (verbatim):`
- [ ] Contract manifests fail closed when unvalidated, every resolved path component remains in the workspace, final targets are regular files, and failures expose only manifest-relative paths
- [ ] Redaction covers `GH_TOKEN`, `GITHUB_TOKEN`, and full `Authorization:` lines
- [ ] Local observations use only the documented metadata allowlist and do not affect Hive reporting when stderr fails
- [ ] Native assignments use one isolated Goose session and task runtime directory, without tmux manipulation
- [ ] All ready CLI backends participate in auto-detect; multiple candidates
  invoke the attended `gum` chooser, Goose prompts for provider/model, and
  remembered selections plus mode-`0600` current-run state have deterministic
  coverage without requiring a real TTY
- [ ] Context7 is optional and local hooks are not treated as CI authority
- [ ] QEMU readiness reports boot, control channel, network, Hive, and worker
  stages without secrets
- [ ] Each assignment gets a fresh guest clone directory and cleanup
- [ ] QEMU, runner, channel, and overlay cleanup is idempotent on every exit
- [ ] `go test ./cmd/contributor ./internal/app ./internal/config ./internal/runner ./internal/hive` passes
- [ ] `go test ./...` passes
