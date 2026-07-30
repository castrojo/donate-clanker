# donate-clanker Self-Contained VM Design

## Decision

The supported product becomes one self-contained, bootable donate-clanker VM.
The Bluefin host (base and DX) supplies only the launcher, KVM access, and
network connectivity. The guest owns the worker runtime, Git client, task
workspace, and repository clone. No host repository, home directory, container
socket, or workspace is mounted into the guest.

Use a small QEMU microVM launched inside a pinned OCI runner image. QEMU is
preferred over Lima because it makes the guest boundary explicit and is
available on the supported Bluefin host classes. Firecracker-style
principles apply—minimal devices, immutable guest base, one workload, and
ephemeral writable state—but Firecracker itself is not required for the first
implementation. Do not add microagent.

## Artifacts

The release consists of:

- `ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:<digest>`: pinned
  QEMU/KVM launcher image containing only the runner and its dependencies.
- `ghcr.io/projectbluefin/donate-clanker-vm-guest@sha256:<digest>`: immutable
  guest kernel, initramfs/root filesystem, and guest worker.
- A signed OCI manifest and SBOM for each image, plus provenance attestation.
- A host launcher in `just/61-donate-clanker.just`, with lifecycle code moved
  into `internal/vm` as part of the implementation plan, that verifies and
  runs the matching digests.

The guest base is built from the supported Bluefin userspace contract, but is
not the host filesystem. The guest image contains QEMU-visible boot metadata,
the worker, `git`, CA certificates, DNS support, and no host credentials.

## Host and guest flow

1. The launcher checks that it is running on supported Bluefin base or DX,
   that `/dev/kvm` is usable, and that the selected runner and guest digests
   are signature-verified.
2. It creates a per-run directory under the user's state directory containing
   only an ephemeral overlay disk, a QEMU log, and a bootstrap channel socket.
3. Podman runs the pinned QEMU runner image with KVM, no host filesystem mounts,
   and a private per-run network namespace. QEMU boots the immutable guest
   image and an ephemeral writable overlay.
4. The host sends a one-shot bootstrap envelope over a virtio-serial channel.
   The envelope contains the Hive endpoint, registration credential, backend
   selection, and run identifier. The guest acknowledges boot and readiness
   over the same channel.
5. The guest starts the contributor worker. For each assignment it creates a
   fresh clone directory, executes one task, reports the terminal result, and
   deletes the clone and task credentials before requesting another task.
6. The host forwards redacted guest status and exits with the worker result.
   It never receives repository files or task output as a mounted filesystem.

The VM is foreground-only. Ctrl-C cancels the guest, waits briefly for QEMU to
exit, then removes the overlay, channel, and logs according to the local
retention policy.

## Credential and trust boundary

The host may read existing Hive setup, but must not copy host GitHub config,
SSH keys, cloud credentials, or a general-purpose environment into the guest.
Bootstrap secrets are sent once over the channel, held in guest memory, and
never written to the immutable base or overlay. The guest clears the channel
after parsing.

The Hive registration credential is used only to establish the guest's
connection. The assignment-scoped GitHub token received from Hive is used only
for that assignment's clone and push operations, preferably through an
in-memory askpass helper. It is not persisted in Git config, clone URLs, logs,
observations, or task artifacts. The worker must scrub it before cleanup.

The guest is trusted to execute the assigned workload, not to access the host.
QEMU exposes only KVM, the minimal serial/control devices, and outbound
networking. No host socket, shared directory, clipboard, GUI, or inbound port
is exposed.

## Clone lifecycle

Each assignment maps to a sanitized task ID under `/run/donate-clanker/tasks`.
The guest:

1. validates the repository URL and expected revision;
2. creates a new empty directory and a temporary askpass/token scope;
3. clones the assigned repository using HTTPS credentials;
4. verifies the checkout is inside the task directory and records no secret in
   remotes or configuration;
5. runs the worker with the repository as its current directory;
6. reports success or failure to Hive;
7. removes the task directory, askpass helper, and token from the environment.

A clone or checkout failure is a terminal assignment failure, not a retry loop
that could reuse credentials. A VM restart starts with no prior clone.

## Networking and readiness

The guest has outbound-only networking using QEMU user networking (or an
equivalent rootless-compatible minimal backend). DNS, HTTPS GitHub access, and
the Hive WSS endpoint are required. No guest service is reachable from the
host network.

Readiness is explicit and layered: QEMU process started; guest boot marker
received; control channel acknowledged; network/DNS check passed; Hive
authenticated; worker sent `ready`. A timeout at any layer identifies the
failed layer without printing credentials or task content.

## Failure and cleanup

All startup and runtime failures use one cleanup path. The host stops the
runner container and QEMU process, closes the control channel, removes the
ephemeral overlay, and preserves only bounded redacted logs. Guest shutdown
first revokes/ends the worker session, scrubs task credentials, unmounts task
state, and powers off.

Cleanup is idempotent and runs on normal completion, Ctrl-C, failed readiness,
QEMU crash, lost control channel, or host launcher error. No persistent VM,
queue, lease database, or background service is created.

## Release, SBOM, and signing

CI builds amd64 and arm64 runner/guest artifacts from pinned inputs, emits
SPDX SBOMs, provenance attestations, and signs the OCI manifest digests with
keyless Cosign. The launcher consumes immutable digests; version tags are
release aliases only and `latest` is never the production default.

Publication gates include guest boot smoke tests, signature verification,
SBOM presence and referrer linkage, QEMU/KVM smoke coverage where available,
and static checks proving there are no host workspace or socket mounts.

## Explicitly punted

- Firecracker-specific packaging or a second VM backend.
- Lima, microagent, Docker-in-Docker, and host container-engine sockets.
- Host workspace sharing, bidirectional file sync, or editor integration.
- Persistent guest disks, reusable clones, task replay, and offline queues.
- In-guest model hosting; inference remains an external/backend concern.
- Supporting non-Bluefin hosts, nested virtualization, GUI applications, or
  inbound guest services.
- Automatic secret rotation, a new Hive protocol, and a second authorization
  system.
- Full guest transcript export, telemetry retention, and remote debugging.
