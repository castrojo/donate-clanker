# Production Readiness Onboarding and Publication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a production-gated GitHub + Goose/local-inference contributor path, centralize the RamaLama image in `fsdk-containers`, and prepare the repository for transfer to `projectbluefin/donate-clanker`.

**Architecture:** The launcher owns onboarding, Hive setup verification, engine/pod lifecycle, and foreground cleanup. The Goose worker owns one-task execution and Hive protocol handling. `fsdk-containers` owns the signed multi-architecture RamaLama helper image; the launcher consumes an immutable image reference.

**Tech Stack:** Go 1.22, Goose, RamaLama, Podman/Docker-compatible engines, Hive WebSocket protocol, BuildStream 2, GitHub Actions, Cosign, shell-based CI.

## Global Constraints

- The first supported backend is GitHub + Goose/local inference.
- GitHub authentication is verified with `gh auth status`; config-directory presence is never authentication.
- Hive registration reuses upstream `contribute-setup`; this project does not reimplement device-code or registration logic.
- Mount only the selected workspace read/write, specific Hive/GitHub paths read-only, and a persistent model cache.
- Never mount the whole home directory or the host container socket into Goose.
- Context7 is optional and cannot block onboarding or task execution.
- Execution is foreground-only and ephemeral; no login linger, auto-start, queue, lease database, or second trust system.
- Production consumes a signed immutable image digest or versioned release tag, not mutable `latest`.
- Do not claim production readiness until the runner, onboarding, cleanup, image, and clean-machine gates pass.

---

### Task 1: Harden GitHub + Goose onboarding checks

**Files:**
- Modify: `just/61-donate-clanker.just`
- Modify: `README.md`
- Modify: `quadlet/donate-clanker.container`
- Create: `internal/setup/auth.go`
- Test: `internal/setup/auth_test.go`
- Modify: `.github/workflows/validate.yml`

**Interfaces:**
- `CheckGitHubAuth(ctx context.Context, runner CommandRunner) error` verifies a valid `github.com` session using `gh auth status` without exposing tokens.
- `ValidateHiveConfig(path string) error` checks required keys and rejects empty values without returning secret contents.
- `CheckGooseLocalConfig(path string) error` validates local-provider configuration semantically, not by directory existence.
- The legacy Justfile doctor calls the same observable checks or matches their exact failure semantics.

- [ ] **Step 1: Write failing tests** for missing `gh`, invalid `gh auth status`, missing Hive config, empty required Hive values, missing Goose local endpoint, and secret-redacted errors.
- [ ] **Step 2: Run `go test ./internal/setup`** and verify the tests fail before implementation.
- [ ] **Step 3: Implement command-backed GitHub auth validation** using argument arrays and bounded stderr capture; never call `gh auth token` in logs.
- [ ] **Step 4: Implement Hive and Goose semantic config checks** with explicit paths and safe error messages.
- [ ] **Step 5: Replace filesystem-presence auth checks in the Justfile's supported GitHub + Goose path** and keep unsupported backends out of the production auto-detect list.
- [ ] **Step 6: Update README onboarding** with the exact `gh auth login --web --hostname github.com --scopes repo,read:org` command and upstream Hive setup step.
- [ ] **Step 7: Run `gofmt -w`, `go test ./internal/setup`, and the Justfile shell validation**.
- [ ] **Step 8: Commit** with `fix: harden github and goose onboarding`.

### Task 2: Validate Context7 and non-thinking image contracts

**Files:**
- Modify: `.github/workflows/validate.yml`
- Modify: `README.md`
- Test: `internal/config/context7_test.go`
- Test: `internal/profile/profile_test.go`

**Interfaces:**
- CI verifies the Context7 URI, enabled streamable HTTP extension, opportunistic policy, local fallback, and explicit non-thinking settings without network access.
- README states that Context7 requires no credential, failure is non-blocking, and all bundled profiles disable extended thinking.

- [ ] **Step 1: Add static CI checks** for the Goose config, policy, and every model profile.
- [ ] **Step 2: Add tests** for missing/empty policy and contradictory thinking flags if coverage is absent.
- [ ] **Step 3: Update README** with the optional Context7 and non-thinking behavior.
- [ ] **Step 4: Run `go test ./...`, `just --justfile just/61-donate-clanker.just --list`, and `git diff --check`**.
- [ ] **Step 5: Commit** with `docs: validate local inference image contract`.

### Task 3: Implement the minimal foreground runner foundation

**Files:**
- Create: `cmd/donate-clanker/main.go`
- Create: `cmd/contributor/main.go`
- Create: `internal/engine/`
- Create: `internal/setup/`
- Create: `internal/pod/`
- Create: `internal/app/`
- Test: colocated Go tests
- Create: `image/Containerfile`

**Interfaces:**
- `app.Run(context.Context, config.Options) error` performs ordered validation, setup, profile resolution, pod startup, endpoint readiness, worker startup, log forwarding, and cleanup.
- `Handle.Close(context.Context) error` removes only resources owned by this invocation and is idempotent.
- `EnsureHiveSetup(context.Context, SetupOptions) error` invokes/verifies upstream setup without reimplementing registration.
- `ResolveMounts(config.Options) ([]Mount, error)` rejects whole-home and host-socket mounts.

- [ ] **Step 1: Add fake-engine and fake-command tests** for startup ordering, setup failure, endpoint timeout, worker failure, Ctrl-C cleanup, and idempotent cleanup.
- [ ] **Step 2: Implement typed options and command-backed engine discovery** for Podman and Docker without shell interpolation.
- [ ] **Step 3: Implement safe setup/config checks and mount construction** using the hardened authentication contract from Task 1.
- [ ] **Step 4: Implement ephemeral pod lifecycle** with private model endpoint, explicit image references, readiness timeout, and owned-resource labels.
- [ ] **Step 5: Implement signal-aware orchestration** and cleanup on every exit path.
- [ ] **Step 6: Add the worker image entrypoint** that starts only the contributor process and receives no host engine socket.
- [ ] **Step 7: Run `go test ./...`, `gofmt -l .`, and image-spec smoke checks**.
- [ ] **Step 8: Commit** with `feat: add foreground contributor runner`.

### Task 4: Implement Hive protocol and Goose task execution

**Files:**
- Create: `internal/hive/`
- Create/modify: `internal/runner/`
- Test: protocol fixtures and Goose command tests

**Interfaces:**
- `Client.Run(context.Context, Credentials, AssignmentHandler) error` handles challenge/auth, ready, assignment, ping/pong, completion, failure, refresh, and revoke.
- `Goose.Run(context.Context, Task) (Result, error)` runs one headless task with bounded output and explicit non-zero failure propagation.

- [ ] **Step 1: Write protocol fixture tests** for auth, liveness, identity, refresh, revoke, and closed-socket failure.
- [ ] **Step 2: Write Goose runner tests** for `--no-session`, model/provider endpoint values, workspace, policy ordering, and non-zero exit propagation.
- [ ] **Step 3: Implement the persistent WebSocket reader/state machine** so authentication and task frames cannot be lost.
- [ ] **Step 4: Implement headless Goose execution** with non-thinking environment defaults, optional Context7 policy, and redacted output.
- [ ] **Step 5: Run targeted protocol/runner tests and then `go test ./...`**.
- [ ] **Step 6: Commit** with `feat: run goose hive contributor tasks`.

### Task 5: Centralize and publish the RamaLama FSDK image

**Files:**
- Modify or create in `projectbluefin/fsdk-containers`: `elements/ramalama/*`, `elements/oci/ramalama.bst`, `Justfile`, workflow, README, verification docs.
- Modify in this repository: image reference/config documentation and release workflow.

**Interfaces:**
- The helper image exposes the private OpenAI-compatible endpoint and persistent model-cache contract consumed by Task 3.
- Published manifests target `linux/amd64` and `linux/arm64`, carry SBOM/provenance, and are keyless-signed.

- [ ] **Step 1: Inspect FSDK component availability and existing image patterns** before editing.
- [ ] **Step 2: Add the RamaLama stack/runtime/OCI elements** using `components/*`, the shared SLIM recipe, and no shell/package manager.
- [ ] **Step 3: Register the image in validation, build, verify, SBOM, manifest, and signing paths**.
- [ ] **Step 4: Add the immutable endpoint/cache contract to both repositories' documentation**.
- [ ] **Step 5: Run FSDK `just validate`, local build/verify where available, workflow checks, and signature/SBOM checks for a test digest**.
- [ ] **Step 6: Commit the FSDK changes** with `feat: publish ramalama fsdk image`.

### Task 6: Final expert review and publication preparation

**Files:**
- Modify: `README.md`
- Modify: `.github/workflows/validate.yml`
- Create: release/transfer checklist under `docs/superpowers/`

**Interfaces:**
- Clean-machine onboarding is reproducible from documented commands.
- Release artifacts reference `projectbluefin/donate-clanker` and the signed immutable RamaLama image.

- [ ] **Step 1: Run security review** over auth, mounts, sockets, network, image permissions, and logs.
- [ ] **Step 2: Run code review** over the complete diff and all failure/cleanup paths.
- [ ] **Step 3: Run FSDK/container review** over BuildStream, architecture, SBOM, provenance, and signature gates.
- [ ] **Step 4: Run user-onboarding review** from GitHub login through first task and Ctrl-C recovery.
- [ ] **Step 5: Resolve all high-confidence findings** and record any remaining release blockers.
- [ ] **Step 6: Validate target repository creation and Actions/package permissions** with `gh`; do not transfer until all gates pass.
- [ ] **Step 7: Transfer `castrojo/donate-clanker` to `projectbluefin/donate-clanker`** only after approval and successful publication checks.
- [ ] **Step 8: Update canonical URLs and verify the old repository redirects**.

---

## Execution Order

Tasks 1 and 2 are independent and can run in parallel. Task 3 depends on
Task 1. Task 4 depends on Task 3's worker contracts. Task 5 can proceed in
parallel with Tasks 3–4 once the endpoint/cache contract is fixed. Task 6
depends on all implementation and image work.

Repository transfer and external publication are final release actions and
must not happen while tests or expert review gates are failing.
