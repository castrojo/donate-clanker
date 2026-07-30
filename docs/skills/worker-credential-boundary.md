---
name: donate-clanker-worker-credential-boundary
description: Use when changing donate-clanker worker auth, Goose task launching, Hive credential flow, workspace mounts, or prompt construction around policy and assignment text.
metadata:
  context7-sources:
    - /containers/ramalama
    - /websites/podman_io_en
    - /websites/pkg_go_dev_go1_25_3
    - /lima-vm/lima
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
11. Supported Goose onboarding must rely on validated `~/.config/goose/config.yaml`; launcher-managed model prompts, remembered `AGENT_MODEL` defaults, and secrets-file persistence remain legacy-only behavior for explicit `claude`, `copilot`, and `codex` paths.
12. Run focused tests for `cmd/contributor`, `internal/app`, `internal/config`, `internal/runner`, and `internal/hive`, then run `go test ./...`.

## Lima Guest Runtime Boundary

The product launcher runs the published compatibility container entirely in
the reusable `donate-clanker` Lima VM. It invokes the guest's Podman through
`limactl shell`, never the host container engine. When `limactl` is absent,
only Bluefin DX may install Lima with `brew install lima`; all other hosts
must provide Lima themselves.

Create the VM from `template:podman` with an explicit writable workspace mount;
leave `~/.config/hive` and selected tool config mounts unsuffixed so Lima keeps
them read-only by default. Bind only `/workspace` and `/config` into
the guest container, with `/config` read-only; selected tool configuration is
likewise read-only. Keep the container attached to the invoking terminal with
`podman run --rm -it`, and reuse the VM rather than creating a host service.

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

Lima documents `limactl start --name=<name> template:podman`, named-instance
reuse, `limactl shell <name> <command>`, and `--mount-only /path` as
read-only unless the path ends in `:w` (source: Context7 `/lima-vm/lima`).

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
- Supported Goose onboarding reaches a launcher-managed model picker, remembers Goose-specific model defaults, or persists `AGENT_MODEL` for `TOOL=goose`
- Context7 availability can block assignment execution
- Local hooks are treated as a substitute for CI
- Tests only cover success paths and not env/mount exposure or redaction
- A foreground compatibility container starts a detached tmux session but never attaches it to the user's terminal
- Host Podman, Docker, systemd, or a container socket launches the product
  contributor instead of `limactl shell`
- Lima mounts the full home directory writable or the contributor receives
  more than its workspace, Hive config, and selected tool config

## Verification

- [ ] `ResolveMounts` returns only workspace and cache mounts
- [ ] Worker env contains Hive connection values but not host auth paths
- [ ] `clearWorkerCredentialEnvironment` removes host Hive/GitHub auth env
- [ ] Goose receives only assignment-scoped GitHub token values
- [ ] Missing required repository documents stop native task execution before any Goose command starts
- [ ] Prompt output preserves verbatim assignment text under a dedicated heading
- [ ] Valid workspaces inject the manifest-backed contract section before `Hive assignment (verbatim):`
- [ ] Contract manifests fail closed when unvalidated, every resolved path component remains in the workspace, final targets are regular files, and failures expose only manifest-relative paths
- [ ] Redaction covers `GH_TOKEN`, `GITHUB_TOKEN`, and full `Authorization:` lines
- [ ] Local observations use only the documented metadata allowlist and do not affect Hive reporting when stderr fails
- [ ] Native assignments use one isolated Goose session and task runtime directory, without tmux manipulation
- [ ] Supported Goose onboarding skips launcher-managed model prompt/persistence, while legacy backends keep deterministic coverage for remembered model overrides without requiring a real TTY
- [ ] Context7 is optional and local hooks are not treated as CI authority
- [ ] Compatibility image sources `gh-auth.env` and attaches tmux without requiring a host container socket
- [ ] Missing `limactl` invokes Homebrew only on Bluefin DX, and a present
  `limactl` never invokes Homebrew
- [ ] The named `donate-clanker` VM is created from `template:podman` once
  and reused thereafter
- [ ] The guest contributor runs foreground with a read-only `/config` mount,
  writable `/workspace` mount, and Lima host mounts that use `:w` only for
  the workspace
- [ ] `go test ./cmd/contributor ./internal/app ./internal/config ./internal/runner ./internal/hive` passes
- [ ] `go test ./...` passes
