# donate-clanker VM Implementation Plan

> Implementation follows the approved self-contained guest design. Do not
> implement Lima, microagent, host workspace mounts, or a second trust system.

## Global constraints

- The guest clones each assigned repository internally; no host workspace is
  mounted or synchronized.
- Host support is Bluefin base and DX with usable `/dev/kvm`.
- Prefer the pinned containerized QEMU runner and a minimal guest image.
- Bootstrap and assignment credentials are one-shot, memory-only, and redacted.
- Production references immutable signed digests, with SBOM and provenance.
- Preserve foreground execution and idempotent cleanup.

## Task 1: Define VM artifact and digest contracts

**Files:**
- Create `vm/Containerfile`
- Create `vm/guest/Containerfile`
- Create `vm/manifest.json`
- Create `vm/README.md`
- Modify `README.md`

Define the runner image, guest kernel/rootfs layout, supported architectures,
required QEMU devices, control-channel name, status messages, and immutable
image-reference configuration. Document Bluefin base/DX prerequisites and
explicitly reject unsupported hosts before startup.

**Validation:** static manifest tests; inspect image metadata; verify no host
mount or socket appears in the documented command.

## Task 2: Build the minimal guest runtime

**Files:**
- Create `vm/guest/init`
- Create `vm/guest/worker`
- Create `vm/guest/network.conf`
- Create `vm/guest/README.md`

Build an immutable guest containing the worker, `git`, CA certificates, DNS,
and the virtio-serial bootstrap reader. Implement boot acknowledgement,
network readiness, Hive authentication, and orderly poweroff. Keep writable
state under `/run` or the ephemeral overlay only.

**Tests:** boot the guest without credentials; assert it waits for bootstrap,
rejects malformed envelopes, and never writes bootstrap values to disk.

## Task 3: Implement host VM lifecycle

**Files:**
- Create `internal/vm/spec.go`
- Create `internal/vm/qemu.go`
- Create `internal/vm/channel.go`
- Create `internal/vm/cleanup.go`
- Modify `cmd/donate-clanker/main.go`
- Modify `just/61-donate-clanker.just`

Add typed construction for the runner command, QEMU arguments, per-run state
directory, ephemeral overlay, and virtio-serial channel. Use argument arrays,
fixed device lists, outbound-only networking, bounded readiness timeouts, and
owned-resource labels. Reject missing KVM, unsupported architecture, mutable
image references, and unsafe state paths before starting QEMU.

**Tests:** fake command runner tests for argument exactness, startup ordering,
readiness timeouts, signal propagation, idempotent cleanup, and no workspace,
home, or socket mounts.

## Task 4: Implement bootstrap and credential handling

**Files:**
- Create `internal/vm/bootstrap.go`
- Create `internal/vm/credentials.go`
- Modify `docs/skills/worker-credential-boundary.md`

Load only the existing Hive endpoint/registration values required to connect.
Send a versioned bootstrap envelope over the control channel, close it after
acknowledgement, and ensure secret values never enter logs, QEMU arguments,
disk images, or child process command lines. Define assignment-token
redaction and test malformed, missing, and expired credentials.

**Tests:** allow-list envelope fields; assert token absence from serialized
state, errors, and logs; verify cleanup clears references.

## Task 5: Implement guest clone and assignment lifecycle

**Files:**
- Modify `vm/guest/worker`
- Create `vm/guest/clone`
- Modify `internal/vm/status.go`
- Add guest/host protocol fixtures under `internal/vm/testdata`

For every assignment, sanitize the task ID, create a fresh task directory,
clone over HTTPS with an in-memory askpass helper, verify the checkout, run
one task, report exactly one terminal result, and delete all task state before
the next assignment. Treat clone failure as terminal assignment failure and
never retry with a prior token or directory.

**Tests:** two assignments use distinct directories; clone failure cleans
credentials; cancellation deletes the active clone; report ordering is
terminal result before the next `ready`.

## Task 6: Add release, SBOM, and signing workflow

**Files:**
- Create `.github/workflows/publish-vm.yml`
- Modify `.github/workflows/validate.yml`
- Create `scripts/verify-vm-artifacts.sh`
- Modify `docs/skills/worker-credential-boundary.md`

Build amd64/arm64 runner and guest artifacts from pinned inputs. Generate
SPDX SBOMs and provenance, attach them as OCI referrers, and keylessly sign
manifest digests and SBOM references. Publish immutable `sha-<commit>` and
version tags; keep mutable aliases out of launcher defaults.

**Validation:** workflow/static checks for both architectures, SBOM and
signature verification, digest pinning, and guest boot smoke tests.

## Task 7: Documentation and final verification

**Files:**
- Update the VM design/spec references in `README.md`
- Update `docs/skills/worker-credential-boundary.md` with the VM boundary
- Add no changelog or session-log files

Document install/run, Bluefin prerequisites, readiness failures, Ctrl-C
cleanup, clone behavior, credential limits, artifact verification, and all
explicit punts. Keep compatibility-image and old Lima references clearly
marked as superseded rather than supported VM behavior.

Run:

```bash
gofmt -w internal/vm cmd/donate-clanker
go test ./...
git diff --check
test -z "$(gofmt -l .)"
just --justfile just/61-donate-clanker.just --list
```

Also run the guest boot smoke test and artifact verification script on a
Bluefin DX host with KVM. Record remaining hardware-dependent gaps as release
blockers, not silent fallbacks.
