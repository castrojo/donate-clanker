# Donate Clanker VM artifact contract

The VM launcher consumes externally built artifacts. This repository does not
build the BuildStream/FSDK guest or QEMU runner; release automation only
verifies that those artifacts satisfy this contract.

`manifest.json` defines the required OCI references:

- runner image: `VM_RUNNER_REF`
- guest image: `VM_GUEST_REF`
- guest kernel, initramfs, and root filesystem: the three
  `VM_GUEST_*_REF` values

Every value must be an OCI digest reference (`image@sha256:<64 hex digits>`).
Tags, branch names, `latest`, and digest aliases are rejected. Runner and
guest images must publish both `amd64` and `arm64` manifests. Guest artifacts
must be present for both supported architectures.

Each image/artifact must have all of the following OCI referrer metadata:

1. a keyless Cosign signature;
2. an SPDX SBOM attestation;
3. a provenance attestation.

The release gate requires `COSIGN_CERT_IDENTITY_REGEXP` and
`COSIGN_CERT_ISSUER`; missing values fail closed. The artifact producer owns
the BuildStream/FSDK implementation, image contents, signing identity, and
registry publication. The consumer must not substitute a mutable reference or
silently continue when an artifact, architecture, signature, SBOM, or
provenance record is absent.

Validate locally with:

```sh
VM_VERIFY_REMOTE=false \
VM_RUNNER_REF=ghcr.io/example/runner@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa \
VM_GUEST_REF=ghcr.io/example/guest@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb \
VM_GUEST_KERNEL_REF=ghcr.io/example/kernel@sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc \
VM_GUEST_INITRAMFS_REF=ghcr.io/example/initramfs@sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd \
VM_GUEST_ROOTFS_REF=ghcr.io/example/rootfs@sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee \
scripts/verify-vm-artifacts.sh
```

The example values only exercise static validation; they are not release
artifacts. A release workflow must provide real immutable references and run
remote verification.
