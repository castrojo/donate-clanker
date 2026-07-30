# Donate Clanker Runner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a reusable container workload and thin Go launcher that run one foreground contributor workload with local RamaLama inference across Linux, macOS, and Windows/WSL2; Bluefin exposes it through a convenience `just` command.

**Architecture:** The container owns the commodity Goose CLI, Hive worker, RamaLama integration, profile catalog, and task execution. The Go launcher is a thin host adapter for runtime detection, upstream onboarding, mounts, pod lifecycle, logs, and cleanup; Bluefin only wires it into `just`/`ujust`.

**Tech Stack:** Go, Goose CLI, Hive contributor WebSocket protocol, Podman/Docker-compatible engines, OCI images, GitHub Actions.

## Global Constraints

- Local inference is the only v1 provider mode.
- The launcher is a native Go binary for Linux, macOS, and Windows.
- Images target `linux/amd64` and `linux/arm64`.
- Execution is foreground-only and ephemeral; no systemd, Quadlet, login linger, or background service.
- Authentication reuses upstream Hive `contribute-setup`; this project does not reimplement device-code or registration logic.
- Mount only the selected workspace read/write and a persistent RamaLama cache; pass the minimal Hive client auth via worker env and inject only task-scoped GitHub tokens into Goose.
- The Goose container never receives a host container socket.
- GPU support is best-effort by platform, but startup fails when no supported GPU is available.
- The client must preserve Hive auth, ping/pong, task identity, revocation, and credential-redaction rules.
- Do not add a queue, lease database, trust system, merge path, or remote provider mode.

---

## File Map

### Donate-clanker repository

- Create `go.mod` and `go.sum`: Go module metadata.
- Create `cmd/donate-clanker/main.go`: user-facing CLI entrypoint.
- Create `cmd/contributor/main.go`: container entrypoint for the Hive/Goose worker.
- Create `internal/engine/`: Podman, Docker, and Colima discovery plus command execution.
- Create `internal/profile/`: container-side curated model catalog, hardware detection, and profile selection.
- Create `internal/setup/`: upstream setup checks and safe configuration-path resolution.
- Create `internal/pod/`: pod creation, helper startup, readiness, log forwarding, and cleanup.
- Create `internal/hive/`: WebSocket protocol state machine and task lifecycle.
- Create `internal/runner/`: one-task Goose headless invocation and result reporting.
- Create `internal/config/`: CLI flags, environment overrides, mount policy, and cache paths.
- Create `image/Containerfile`: Goose/Hive application image build definition.
- Create `image/config/`: bundled Goose provider configuration and profile catalog schema.
- Create `internal/*/*_test.go`: unit and protocol tests colocated with implementation.
- Modify `README.md`: replace Quadlet onboarding with launcher installation, container usage, and Bluefin `just` integration.
- Modify `.github/workflows/validate.yml`: Go formatting/tests and image/config validation.

### FSDK containers repository

- Create `elements/ramalama/ramalama-stack.bst`: FSDK/runtime dependencies for the helper.
- Create `elements/ramalama/ramalama-runtime.bst`: composed runtime-only filesystem.
- Create `elements/oci/ramalama.bst`: slim OCI output and FSDK labels.
- Modify `Justfile`: register `ramalama` in build, verify, SBOM, and manifest lists.
- Modify `README.md`: document the image, runtime contract, and model-cache behavior.
- Modify `docs/skills/verify-distroless.md`: add RamaLama binary smoke and size gates.
- Add/update the focused FSDK skill under `docs/skills/`: record the component/source and pruning decisions.

---

### Task 1: Scaffold the Go module and CLI contracts

**Files:**
- Create: `go.mod`
- Create: `cmd/donate-clanker/main.go`
- Create: `cmd/contributor/main.go`
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- `config.Options` contains `Engine`, `Workspace`, `Profile`, `Model`, `CacheDir`, `HiveConfigDir`, `GitHubConfigDir`, and `NonInteractive`.
- `config.Parse(args []string, env map[string]string) (Options, error)` returns normalized paths and rejects conflicting flags.
- `cmd/donate-clanker` calls `config.Parse`, then delegates to a runner interface.
- `cmd/contributor` accepts only container-side worker options and never performs host engine discovery.

- [ ] **Step 1: Write parsing tests** for default local mode, explicit profile/model overrides, workspace resolution, missing workspace, and non-interactive mode.
- [ ] **Step 2: Run `go test ./internal/config`** and verify the new tests fail because the module and parser do not exist.
- [ ] **Step 3: Implement the typed options and parser** with explicit errors for missing workspace, invalid profile, and mutually exclusive model flags.
- [ ] **Step 4: Run `gofmt -w` and `go test ./internal/config`**; verify all parser tests pass.
- [ ] **Step 5: Commit** with `feat: scaffold donate-clanker go commands`.

### Task 2: Implement container-engine discovery and command execution

**Files:**
- Create: `internal/engine/engine.go`
- Create: `internal/engine/exec.go`
- Create: `internal/engine/podman.go`
- Create: `internal/engine/docker.go`
- Create: `internal/engine/engine_test.go`

**Interfaces:**
- `type Engine interface { Name() string; Version(context.Context) error; PodCreate(context.Context, PodSpec) error; Run(context.Context, RunSpec) (Process, error); Logs(context.Context, string) (io.ReadCloser, error); Stop(context.Context, string) error; Remove(context.Context, string) error }`.
- `Detect(context.Context, Preference) (Engine, error)` tries explicit selection first, then Podman, Docker, and Colima-backed Docker in deterministic order.
- `Process` exposes `Wait() error`, `Signal(os.Signal) error`, and `StdoutStderr() io.ReadCloser`.

- [ ] **Step 1: Write fake-engine tests** for explicit selection, unavailable-engine errors, Colima detection, and deterministic fallback order.
- [ ] **Step 2: Run `go test ./internal/engine`** and verify failure against missing implementations.
- [ ] **Step 3: Implement the command-backed engine adapter** without shell interpolation; pass arguments as separate `exec.Cmd` arguments.
- [ ] **Step 4: Implement Podman and Docker capability checks** including the Linux image architecture and host GPU prerequisite probes.
- [ ] **Step 5: Run `go test ./internal/engine`** and verify all fake-engine tests pass.
- [ ] **Step 6: Commit** with `feat: add container engine abstraction`.

### Task 3: Add the container-side RamaLama model profiles

**Files:**
- Create: `internal/profile/catalog.go`
- Create: `internal/profile/hardware.go`
- Create: `internal/profile/select.go`
- Create: `image/config/models.json`
- Test: `internal/profile/profile_test.go`

**Interfaces:**
- `type Profile struct { ID, ModelRef, Runtime string; MinVRAMMiB, MinRAMMiB int; ContextSize int }`.
- `Load(path string) (Catalog, error)` verifies schema, immutable model references, and duplicate IDs.
- `Select(catalog Catalog, hardware Hardware, requested string) (Profile, error)` supports `auto`, explicit IDs, and explicit model URI overrides.

- [ ] **Step 1: Validate candidate tool-calling models with RamaLama** using `ramalama info` and a minimal local tool-call smoke test; record two release-approved immutable model references in the container's `image/config/models.json`.
- [ ] **Step 2: Write tests** for catalog validation, auto-selecting the largest compatible profile, explicit profile selection, no-GPU failure, and insufficient VRAM.
- [ ] **Step 3: Run `go test ./internal/profile`** and verify the tests fail before implementation.
- [ ] **Step 4: Implement catalog loading and hardware/profile selection** with no network access and no mutable `latest` references.
- [ ] **Step 5: Run `go test ./internal/profile`** and verify all tests pass.
- [ ] **Step 6: Commit** with `feat: add local model profile selection`.

### Task 4: Implement safe setup checks and mount construction

**Files:**
- Create: `internal/setup/hive.go`
- Create: `internal/setup/paths.go`
- Create: `internal/config/mounts.go`
- Test: `internal/setup/setup_test.go`
- Test: `internal/config/mounts_test.go`

**Interfaces:**
- `EnsureHiveSetup(context.Context, SetupOptions) error` verifies `contributor.env` and invokes the upstream `contribute-setup` command when absent.
- `ResolveMounts(Options) ([]Mount, error)` returns only canonical workspace and RamaLama cache mounts.
- `Mount` contains host path, container path, read-only flag, and SELinux relabel policy.

- [ ] **Step 1: Write tests** proving whole-home mounts are rejected, symlinked workspace paths are canonicalized, missing auth paths fail before pod creation, and cache directories are created with user-only permissions.
- [ ] **Step 2: Run targeted tests** and verify failure before implementation.
- [ ] **Step 3: Implement setup reuse** by invoking the upstream recipe as a subprocess and surfacing its exit status/output without logging secrets.
- [ ] **Step 4: Implement mount construction** with read-only auth mounts and read/write workspace/cache mounts.
- [ ] **Step 5: Run targeted tests** and verify all mount and setup tests pass.
- [ ] **Step 6: Commit** with `feat: validate hive setup and safe mounts`.

### Task 5: Implement pod lifecycle and RamaLama readiness

**Files:**
- Create: `internal/pod/spec.go`
- Create: `internal/pod/lifecycle.go`
- Create: `internal/pod/readiness.go`
- Test: `internal/pod/lifecycle_test.go`

**Interfaces:**
- `Create(ctx, Engine, Spec) (Handle, error)` creates one ephemeral pod with private networking.
- `StartModel(ctx, Handle, Profile) error` starts the FSDK RamaLama helper and waits for its private OpenAI-compatible endpoint.
- `StartWorker(ctx, Handle, WorkerSpec) (Process, error)` starts the Goose/Hive image.
- `Handle.Close(ctx) error` stops and removes only resources created by this invocation.

- [ ] **Step 1: Write fake-engine tests** for startup ordering, model readiness timeout, worker-not-started-on-model-failure, signal cleanup, and idempotent cleanup.
- [ ] **Step 2: Run `go test ./internal/pod`** and verify failure before implementation.
- [ ] **Step 3: Implement pod specs** with private endpoint binding, explicit image digests, mounts, environment values, and no Goose socket mount.
- [ ] **Step 4: Implement readiness polling** against the local model endpoint with bounded timeout and explicit error output.
- [ ] **Step 5: Implement cleanup** using context cancellation and owned-resource names/labels.
- [ ] **Step 6: Run `go test ./internal/pod`** and verify all lifecycle tests pass.
- [ ] **Step 7: Commit** with `feat: manage ephemeral inference pod`.

### Task 6: Implement Hive protocol and headless Goose execution

**Files:**
- Create: `internal/hive/client.go`
- Create: `internal/hive/messages.go`
- Create: `internal/hive/client_test.go`
- Create: `internal/runner/goose.go`
- Create: `internal/runner/goose_test.go`

**Interfaces:**
- `Client.Run(context.Context, Credentials, AssignmentHandler) error` performs challenge/auth, ready, assignment, ping/pong, completion, failure, refresh, and revoke handling.
- `AssignmentHandler.Handle(context.Context, Assignment) (Result, error)` runs one task and returns redacted result metadata.
- `Goose.Run(context.Context, Task) (Result, error)` invokes `goose run --no-session` with the task prompt, workspace, provider endpoint, model, and bounded output capture.

- [ ] **Step 1: Write protocol fixture tests** for auth challenge, auth failure, ping/pong during work, assignment identity, token refresh, revoke, and closed-socket failure.
- [ ] **Step 2: Write Goose command tests** proving `--no-session`, explicit model/provider values, workspace selection, and non-zero exit propagation.
- [ ] **Step 3: Run `go test ./internal/hive ./internal/runner`** and verify failure before implementation.
- [ ] **Step 4: Implement the permanent WebSocket reader/state machine** so no frames are lost during authentication or task execution.
- [ ] **Step 5: Implement the Goose subprocess runner** with redacted logs, bounded context, and explicit result/error mapping.
- [ ] **Step 6: Run targeted tests** and verify all protocol and runner tests pass.
- [ ] **Step 7: Commit** with `feat: run headless goose hive tasks`.

### Task 7: Build the Goose/Hive application image

**Files:**
- Create: `image/Containerfile`
- Create: `image/entrypoint`
- Modify: `.github/workflows/validate.yml`
- Test: `image/image_test.go`

**Interfaces:**
- The image entrypoint accepts `HIVE_WS_URL`, `GOOSE_PROVIDER`, `GOOSE_MODEL`, and the mounted workspace/config paths.
- The image contains no host engine socket and starts only `cmd/contributor`.

- [ ] **Step 1: Add an image smoke test** that checks the configured entrypoint, required binaries, absence of shell/package-manager artifacts, and absence of socket mounts in generated specs.
- [ ] **Step 2: Run the image test** and verify failure before the image exists.
- [ ] **Step 3: Implement the multi-stage build** using a pinned FSDK-compatible runtime base and a statically linked contributor binary; keep the runtime image free of a shell and package manager.
- [ ] **Step 4: Add CI steps** for `go test ./...`, `gofmt -l`, image build, and architecture-specific smoke checks.
- [ ] **Step 5: Run Go tests and the image smoke test** locally.
- [ ] **Step 6: Commit** with `feat: add goose hive worker image`.

### Task 8: Wire the user-facing launcher and Bluefin convenience integration

**Files:**
- Modify: `cmd/donate-clanker/main.go`
- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`
- Modify: `README.md`
- Downstream: Bluefin's `just`/`ujust` recipe imports or invokes the released launcher

**Interfaces:**
- `app.Run(context.Context, config.Options) error` composes engine detection, setup, profile selection, mount construction, pod lifecycle, and worker execution.
- The default command is attached foreground execution; `--non-interactive`, `--profile`, `--model`, and `--workspace` provide automation overrides.

- [ ] **Step 1: Write orchestration tests** for successful startup, setup failure, profile failure, model readiness failure, worker exit, Ctrl-C cleanup, and direct endpoint mode.
- [ ] **Step 2: Implement `app.Run`** with signal-aware context cancellation and ordered cleanup.
- [ ] **Step 3: Pass profile/model selection into the container** and keep interactive selection/profile resolution out of Bluefin.
- [ ] **Step 4: Replace README Quadlet instructions** with binary installation, runtime prerequisites, local-model selection, credential boundaries, direct-image usage, and the thin Bluefin `just` integration contract.
- [ ] **Step 5: Add the downstream Bluefin recipe** that invokes the launcher with Bluefin defaults without copying its implementation.
- [ ] **Step 6: Run `go test ./...` and `gofmt -l .`**; verify no formatting or test failures.
- [ ] **Step 7: Commit** with `feat: add foreground donate-clanker launcher`.

### Task 9: Cross-platform integration validation

**Files:**
- Modify: `.github/workflows/validate.yml`
- Create: `.github/workflows/release.yml`
- Modify: `README.md`

- [ ] **Step 1: Add Linux Podman smoke coverage** for image pull, model helper readiness, Goose startup, signal cleanup, and no lingering owned containers.
- [ ] **Step 2: Add macOS CI/manual instructions** covering Podman machine, Docker Desktop, and Colima engine selection.
- [ ] **Step 3: Add Windows/WSL2 manual smoke instructions** and do not advertise Windows support until the full launcher path passes.
- [ ] **Step 4: Add release builds** for Go `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and `windows/amd64`.
- [ ] **Step 5: Run the smallest local validation** (`go test ./...`, image smoke test, and existing workflow syntax checks).
- [ ] **Step 6: Commit** with `ci: validate cross-platform launcher`.

---

## Execution Order

Tasks 1–4 can be developed independently with fakes. Task 5 depends on Tasks
2–4. Task 6 depends on Task 1 and the protocol fixtures. Task 7 depends on
Tasks 1 and 6. Task 8 depends on Tasks 2–7. Task 9 follows the first
end-to-end Linux run and gates any platform support claims.

The RamaLama image itself is planned separately in
`2026-07-29-fsdk-ramalama-image-plan.md`; the launcher plan consumes its
published immutable image reference.
