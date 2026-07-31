# donate-clanker

donate-clanker runs the contributor worker in a disposable, self-contained
QEMU microVM. The default launcher path boots a checksum-verified raw FSDK
artifact; deployments can instead force a pinned containerized QEMU runner.
In both paths, the guest owns the worker runtime, Git client, task workspace,
and repository clones.

## Host prerequisites

The launcher is supported on Bluefin base and Bluefin DX hosts. The host must
provide:

- QEMU and matching firmware (OVMF on x86_64 or AAVMF/QEMU_EFI on aarch64),
  to boot the local raw VM artifact.
- Podman, only when using the pinned containerized QEMU runner.
- Usable KVM (`/dev/kvm` readable and writable by the invoking user).
- `curl` and `sha256sum` when the raw VM artifact must be downloaded.
- Network access to GitHub, the Hive endpoint, and the configured artifact
  registry.
- A ready, authenticated supported agent CLI (`claude`, `copilot`, `goose`, or
  `codex`). `gum` is used for an attended choice when available; if several
  CLIs are ready and `gum` is unavailable, the plain `ujust donate-clanker`
  path selects Goose automatically.

The host does not provide a repository workspace to the guest. Do not install
or configure Lima for the product path.

## Run it

From any supported Bluefin host, run:

```bash
ujust donate-clanker
```

The launcher verifies GitHub, agent, and Hive setup, then chooses the VM source
in this order:

1. An explicit `DONATE_CLANKER_VM_RAW` path.
2. An explicitly configured `DONATE_CLANKER_VM_RUNNER_IMAGE` digest.
3. One cached raw artifact in `~/.local/state/donate-clanker/`.
4. The versioned raw artifact downloaded from `projectbluefin/fsdk-containers`.

The download path is fail-closed: the selected FSDK release must publish the
matching `.raw` file and `.sha256` sidecar. Set `DONATE_CLANKER_VM_VERSION` to
another published version when needed; until a matching raw release exists,
use an explicit `DONATE_CLANKER_VM_RAW` path or the pinned runner image.

When using a checkout directly, substitute
`just --justfile just/61-donate-clanker.just` for `ujust`. To force the pinned
containerized runner, set `DONATE_CLANKER_VM_RUNNER_IMAGE` to an immutable
`sha256:` digest:

```bash
DONATE_CLANKER_VM_RUNNER_IMAGE=\
ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:<digest> \
  just --justfile just/61-donate-clanker.just donate-clanker
```

The guest boots the matching immutable guest artifact and clones each
Hive-assigned repository internally. Repository files, task output, and host
workspace state do not cross the VM boundary. Ctrl-C stops the foreground
worker and removes the runner, guest overlay, and control channel.

The launcher reads existing Hive setup on the host only to bootstrap the
guest. It does not copy the host home directory, SSH keys, general-purpose
environment, or container-engine socket into the VM. Assignment credentials
are scoped to the guest task and are scrubbed during cleanup.

For local VM prototyping, the launcher auto-fetches the versioned raw VM
artifact for the host architecture into
`~/.local/state/donate-clanker/`, then verifies its downloaded SHA-256
checksum. To use an existing disk instead, point `DONATE_CLANKER_VM_RAW` at
it:

```bash
DONATE_CLANKER_VM_RAW=/path/to/donate-clanker-vm-25.08.14-x86_64.raw \
  ujust donate-clanker
```

This boots the raw FSDK disk directly with host QEMU/KVM using 4 vCPUs and
8 GiB RAM. A remote agent needs at least 2 vCPUs/2 GiB RAM; 2 vCPUs/4 GiB is
the recommended remote-agent size. Local inference is separate and must be
sized for the host model runtime; it is not added to the worker VM sizing.

The upstream BuildStream build has taken about 10 minutes on x86_64 and
produced a roughly 2.2G raw disk in local runs. This is a non-guaranteed
benchmark, not a release-time or hardware-performance promise.

For Goose with a Copilot subscription, run `goose configure`, select **GitHub
Copilot**, and complete Goose's device-flow login. The launcher defaults Goose
to provider `github_copilot`; no API key or host Goose config mount is needed.

Use the read-only preflight before launching:

```bash
just --justfile just/61-donate-clanker.just donate-clanker-doctor
```

## Agent selection and onboarding

The launcher detects every ready CLI in deterministic order:
`claude`, `copilot`, `goose`, then `codex`. One ready CLI is selected without a
prompt. When several are ready, `gum` provides the attended chooser; if `gum`
is unavailable and Goose is ready, Goose is selected automatically. Otherwise,
set `TOOL=<name>` explicitly.

Goose prompts for its provider and optional model when an attended terminal and
`gum` are available. `GOOSE_PROVIDER`, `GOOSE_MODEL`, and `AGENT_MODEL` can
pre-seed those choices. Goose defaults to the `github_copilot` provider, and
the provider/model choices are remembered in
`~/.config/donate-clanker/last-selections.env`. The current launcher-managed
values are written fresh to `~/.config/donate-clanker/secrets.env` with mode
`0600`; stale provider, model, and agent-model values are removed between runs.

If `~/.config/hive/contributor.env` is absent, the launcher fetches the pinned
Hive revision and runs the upstream interactive setup:

```bash
just contribute-setup <tool>
```

The setup is upstream-owned and requires an attended terminal. For
non-interactive use, pre-seed the Hive configuration before invoking the
launcher. The default pin is
`e73f9c6cd650ed50fff22f5d5ac232bd8b7f434e`; `DONATE_CLANKER_HIVE_COMMIT` may
override it, but must be a full 40-character commit SHA. The selected agent
and optional model settings are passed through the VM bootstrap channel; no
host tool configuration directory is mounted.

Use `donate-clanker-stop` to remove a stale containerized runner:

```bash
just --justfile just/61-donate-clanker.just donate-clanker-stop
```

## VM artifacts and external FSDK dependency

The VM release is composed of matching, signed immutable artifacts:

- `ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:<digest>` — the
  containerized QEMU/KVM runner.
- `ghcr.io/projectbluefin/donate-clanker-vm-guest@sha256:<digest>` — the
  immutable guest kernel, initramfs/root filesystem, and worker.

The launcher consumes digests, not floating `latest` images. The local raw VM
download is likewise versioned and checksum-verified. The
`.github/workflows/publish-vm.yml` workflow validates externally published
runner and guest references on version tags or manual dispatch; it does not
build or publish those artifacts. The raw FSDK disk is also external.

Local inference is an explicit external dependency. The FSDK/RamaLama helper
artifact is built and published by
[`projectbluefin/fsdk-containers`](https://github.com/projectbluefin/fsdk-containers);
donate-clanker does not build or vendor that artifact. A deployment selecting
local inference must provide the matching signed immutable FSDK artifact and
its published launch contract. Changes to that image belong in
`projectbluefin/fsdk-containers`, not here.

## Scope

This repository owns the donate-clanker launcher, VM lifecycle, Hive/Goose
worker protocol, guest boundary, and user-facing onboarding. The guest
repository clone lifecycle is internal to the VM. Shared FSDK image work
belongs in `projectbluefin/fsdk-containers`.

The only installed launcher file is
`just/61-donate-clanker.just`. For a personal installation:

```bash
git clone https://github.com/projectbluefin/donate-clanker \
  ~/.local/share/donate-clanker
just --justfile ~/.local/share/donate-clanker/just/61-donate-clanker.just \
  donate-clanker-doctor
```

## Compatibility image (separate legacy path)

`ghcr.io/projectbluefin/donate-clanker` is a compatibility mode wrapper around
the upstream Hive contributor image. Version tags still build this image
through `.github/workflows/publish-compat-image.yml`, but it is not the VM
runner. It does **not** include the native Goose/RamaLama launcher or local VM
artifacts and uses no host container socket. The old Lima, guest-Podman,
`/config`, `/workspace`, and `CLANKER_SRC` behavior is historical migration
context only. Do not add new host workspace, home-directory, socket, Lima, or
compatibility-image requirements to the supported VM path.

## Source of truth

The upstream contributor workflow and setup remain documented at:

https://hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io/contribute

The pinned upstream Hive revision is recorded in
`just/61-donate-clanker.just`.
