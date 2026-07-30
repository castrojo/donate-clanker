# Production Readiness, Onboarding, and Publication

## Goal

Make the GitHub + Goose/local-inference path safe for a new user to
onboard, review the full workload for production readiness, centralize the
RamaLama image in `projectbluefin/fsdk-containers`, and transfer this project
to `projectbluefin/donate-clanker`.

Production readiness means a clean-machine user can authenticate GitHub,
complete Hive registration, start local inference, run a contributor task,
and recover safely from interruption without exposing credentials or leaving
unowned workloads behind.

## Scope

The first supported production backend is GitHub + Goose/local inference.
Claude, Copilot, and Codex remain outside the supported release path until
their authentication and runtime contracts have dedicated validation.

The current upstream-image Quadlet/Justfile path remains a migration and
compatibility path. The new contributor runner, protocol handling, endpoint
readiness, and cleanup behavior are hard prerequisites for calling the
container production-ready.

## Onboarding and authentication

The launcher performs explicit, fail-fast checks in this order:

1. A supported container engine, required host utilities, and a writable
   workspace are available.
2. A valid GitHub CLI session exists for `github.com`. A missing or invalid
   session directs the user to:

   ```text
   gh auth login --web --hostname github.com --scopes repo,read:org
   ```

   Authentication is verified with `gh auth status`; the launcher never
   treats the presence of `~/.config/gh` as proof of authentication.
3. Hive's upstream `contribute-setup goose` flow has completed. The launcher
   invokes or verifies that flow rather than reimplementing registration,
   device-code authentication, or token exchange.
4. The generated Hive configuration is present and semantically valid.
   Validation must not print tokens, raw environment files, or WebSocket
   payloads.
5. Goose and the selected RamaLama endpoint are reachable, and the selected
   model profile is valid before the worker starts.

The read-only doctor command reports each prerequisite independently and
prints only actionable remediation commands. Context7 is optional: its
absence or failure cannot block onboarding or task execution.

The worker mounts only the required Hive/GitHub configuration read-only, the
selected workspace read/write, and the user-owned model cache read/write.
The Goose container never receives the host container socket or a whole-home
mount. Secrets are passed only through the existing upstream configuration
contract and are never logged.

Filesystem-presence checks for Claude, Copilot, and Codex are not valid
authentication checks and are not part of the supported production path.

## Runtime architecture

`projectbluefin/donate-clanker` owns:

- the native launcher and foreground lifecycle;
- GitHub/Hive onboarding and doctor diagnostics;
- Goose policy and optional Context7 configuration;
- model profile selection and validation;
- task protocol handling, liveness, completion, revocation, and cleanup;
- user-facing documentation and tests.

`projectbluefin/fsdk-containers` owns the reusable FSDK BuildStream
definition and CI for the RamaLama helper image. The published image is
multi-architecture and provides the private OpenAI-compatible endpoint,
hardware-specific runtime handoff, persistent model cache contract, SBOM,
provenance, and keyless signature.

The launcher creates one ephemeral pod, starts RamaLama, waits for endpoint
readiness, starts Goose/Hive, forwards logs, and removes all resources it
owns on normal exit, interruption, or failure. Goose never performs nested
container orchestration.

The launcher consumes a signed immutable image digest or versioned release
tag. `latest` may be published for discovery but is not the production
default.

## Review team and release gates

The final review uses four focused passes:

- **Security:** authentication and token handling, secret mounts, host-socket
  isolation, network boundaries, image permissions, and log redaction.
- **Code:** onboarding state transitions, validation, failure handling,
  signal cleanup, and Justfile/Quadlet/documentation drift.
- **FSDK/container:** BuildStream graph, distroless contents, amd64/arm64
  builds, SBOM/signing/provenance, immutable references, and endpoint
  compatibility.
- **User onboarding:** clean-machine GitHub login, Hive registration, model
  setup, doctor output, first task, Ctrl-C, and recovery.

All high-confidence findings are resolved before publication. Remaining
findings must be explicit release blockers rather than silent compromises.

## Validation

The release must pass:

- clean-machine GitHub + Goose onboarding;
- auth/setup validation with redacted diagnostics;
- protocol tests for authentication, ping/pong, assignment, completion, and
  revocation;
- launcher tests for engine selection, profile resolution, mount construction,
  endpoint readiness, signal cleanup, and failure paths;
- container tests proving least-privilege mounts and no Goose host-socket
  access;
- FSDK `validate`, build, verify, SBOM, signature, and provenance checks for
  linux/amd64 and linux/arm64;
- documentation walkthrough from installation through first task and
  interruption recovery.

## Publication sequence

1. Implement and validate the runner and onboarding contract in the current
   repository.
2. Create `projectbluefin/donate-clanker` and validate repository ownership,
   branch protection, Actions permissions, and package publishing permissions.
3. Move the RamaLama BuildStream definition and image CI into
   `projectbluefin/fsdk-containers`.
4. Publish a test digest, run the clean-machine and expert review gates, and
   verify the signed image and SBOM.
5. Transfer `castrojo/donate-clanker` to `projectbluefin/donate-clanker`,
   preserving the old location as GitHub's redirect.
6. Update all installation, image, and source references to the new canonical
   locations.

No repository transfer or production publication occurs before the release
gates pass.

## Deferred work

- Supporting Claude, Copilot, or Codex as production backends.
- Automatic Context7 preflight, retrieval brokers, vector databases, or
  document caches.
- Background services, login linger, queues, lease databases, or a second
  Hive trust/merge system.
- Mutable model tags as production defaults.
