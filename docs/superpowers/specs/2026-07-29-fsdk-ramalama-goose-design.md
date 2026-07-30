# FSDK RamaLama Goose Contributor Design

## Goal

Replace the current Linux/systemd-specific `donate-clanker` orchestration with
a cross-platform, foreground-only container workload using local inference.
Bluefin exposes it through a convenience `just`/`ujust` command, but does not
duplicate the runtime logic.

The v1 experience is local-only: users do not need an API key or a separately
installed agent CLI. They do need a Docker-compatible container engine
(Podman, Docker Desktop, or Colima on macOS; Podman/Docker through WSL2 on
Windows) and hardware supported by the selected RamaLama runtime.

## Architecture

The system has four ownership boundaries:

1. **Container workload**
   - Owns the commodity Goose CLI, Hive protocol worker, RamaLama integration,
     model/profile catalog, provider configuration, and task execution logic.
   - Runs as the reusable artifact independent of Bluefin's `just` packaging.
   - Receives only explicit workspace, auth/config, cache, and model settings.

2. **`donate-clanker` Go launcher**
   - Detects and selects Podman, Docker, or Colima.
   - Runs the host-side upstream Hive contributor setup when required.
   - Passes model/profile settings to the container; it does not reimplement
     Goose, Hive, or RamaLama behavior.
   - Creates one ephemeral pod and manages its complete foreground lifecycle.
   - Mounts only the selected workspace, required Hive/GitHub configuration
     paths, and persistent model storage.
   - Forwards logs and termination signals, then removes the pod and helper
     containers on exit.

3. **Goose/Hive application image**
   - Is a minimal, distroless-compatible image containing Goose and the
     headless Hive contributor protocol client.
   - Receives one Hive assignment at a time.
   - Invokes `goose run --no-session` for the assignment.
   - Uses a private OpenAI-compatible endpoint exposed by the RamaLama helper.
   - Reports task completion/failure and requests the next assignment.
   - Does not contain or access a host container socket.

4. **RamaLama distribution image**
   - Is maintained and published by `projectbluefin/fsdk-containers`.
   - Provides the RamaLama CLI and the runtime integration needed to select
     hardware-specific inference images.
   - Downloads the selected model and runtime assets on first use into the
     persistent model store.
   - Runs as the second container in the same pod as Goose, exposing only the
     private model endpoint.

The launcher is the only component that owns the host pod lifecycle. If the
RamaLama distribution requires the host engine API to start a
hardware-specific runtime, only the dedicated RamaLama helper receives that
engine access; the Goose container never receives a host socket and never
performs nested container orchestration.

## Model selection and hardware

The container offers curated model profiles and supports explicit overrides.
`auto` selects the largest compatible curated profile for detected hardware.
Profiles are versioned configuration entries containing the RamaLama model
reference, runtime preference, context size, and minimum resource
requirements.

The initial policy is:

- GPU acceleration is required for startup.
- Runtime selection is best-effort across supported Linux, macOS, and
  Windows/WSL2 paths.
- Unsupported or unavailable GPU hardware fails before the Goose workload
  starts.
- Model weights and runtime images are cached in a user-owned persistent
  directory and are never written into the application image.

The launcher must not silently track mutable model tags. A profile resolves to
an immutable model reference or a versioned catalog entry, and an explicit
update operation refreshes the catalog/cache.

## Authentication and mounts

Authentication remains host-side and reuses Hive's upstream
`contribute-setup` flow. The launcher does not reimplement GitHub device-code
authentication or Hive registration.

The pod receives only:

- the selected workspace, read/write;
- the user-owned RamaLama model cache, read/write, and only for the helper;
- resolved provider/model environment values;
- the minimal Hive client authentication values required to connect before
  the worker scrubs them from its process environment.

Goose itself receives only task-scoped GitHub credentials from the active
Hive assignment. Host Hive/GitHub auth directories are not mounted into the
Goose process.

The launcher never mounts the whole home directory, prints raw credentials, or
persists transient Hive registration tokens outside the existing upstream
configuration flow.

## Task lifecycle

The normal command is attached and foreground-only:

1. Validate the container engine, workspace, authentication prerequisites,
   profile, and GPU support.
2. Run or verify upstream contributor setup.
3. Pull/start the RamaLama helper and wait for its local endpoint.
4. Start the Goose/Hive container in the same pod.
5. Forward pod logs to the terminal.
6. On Ctrl-C, termination, or process failure, stop and remove all owned
   containers and the ephemeral pod.

There is no systemd/Quadlet unit, login linger, auto-start behavior, or
background service in v1. Direct image invocation remains supported for
advanced users, but requires a pre-existing compatible local model endpoint;
the Go launcher is the supported zero-to-running path.

## Cross-platform contract

The launcher is distributed as native Go binaries for Linux, macOS, and
Windows. Images are published for `linux/amd64` and `linux/arm64`; Windows
uses Linux containers through WSL2 or a compatible Docker/Podman setup.

The launcher abstracts runtime-specific commands behind a small engine
interface. Bluefin's `just`/`ujust` recipe only locates or invokes this
launcher and supplies the normal workspace/config defaults:

- discover or validate the selected engine;
- create a pod/network;
- start the RamaLama helper;
- start the Goose/Hive container;
- stream logs;
- stop and remove owned resources.

No runtime-specific systemd behavior is part of the application contract.

Bluefin integration is intentionally thin. It may package the launcher,
provide a convenience recipe, and set Bluefin-specific defaults, but it must
not contain a second copy of model selection, Hive protocol handling, pod
cleanup, or credential mounting logic.

## Security and failure behavior

Failures are explicit and happen before task execution when possible:

- missing engine, workspace, setup, credentials, model, or GPU;
- unsupported architecture or runtime;
- failed model download or endpoint readiness;
- failed Hive authentication;
- revoked or mismatched Hive task identity.

The client follows the checked-in Hive protocol guidance: authenticate before
ready, preserve ping/pong liveness, verify task identity, honor revocation,
and never log raw WebSocket frames or tokens. It does not create a second
queue, lease database, trust system, or merge path.

## Validation

The design requires:

- launcher unit tests for engine selection, profile resolution, mount
  construction, signal cleanup, and failure paths;
- protocol tests for authentication, ping/pong, assignment, completion, and
  revocation;
- image checks for architecture, minimal contents, and absence of host
  socket access in the Goose container;
- an end-to-end smoke test on Linux Podman and macOS Podman/Docker paths;
- a Windows/WSL2 smoke test before claiming Windows support.

The existing Justfile/Quadlet implementation is not extended. It remains a
legacy migration target until the launcher and image path replace it.
