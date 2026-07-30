# donate-clanker

donate-clanker runs the contributor worker in a disposable, self-contained
QEMU microVM. The supported product path is the pinned VM runner; the guest
owns the worker runtime, Git client, task workspace, and repository clones.

## Host prerequisites

The launcher is supported on Bluefin base and Bluefin DX hosts. The host must
provide:

- Podman, to run the pinned QEMU runner image.
- Usable KVM (`/dev/kvm` readable and writable by the invoking user).
- Network access to GitHub, the Hive endpoint, and the configured OCI registry.
- A ready, authenticated supported agent CLI (`claude`, `copilot`, `goose`, or
  `codex`); `gum` is used when an attended choice is needed.

The host does not provide a repository workspace to the guest. Do not install
or configure Lima for the product path.

## Run it

From any supported Bluefin host, run:

```bash
just --justfile just/61-donate-clanker.just donate-clanker
```

The launcher verifies GitHub, agent, and Hive setup, then requires
`DONATE_CLANKER_VM_RUNNER_IMAGE` to name the signed, immutable QEMU runner
image (a `sha256:` digest). It runs that image with KVM and only a per-run
control/overlay directory; no host workspace, home-directory,
tool-configuration, or container-socket mounts are used:

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

Use the read-only preflight before launching:

```bash
just --justfile just/61-donate-clanker.just donate-clanker-doctor
```

## VM artifacts and external FSDK dependency

The VM release is composed of matching, signed immutable artifacts:

- `ghcr.io/projectbluefin/donate-clanker-vm-runner@sha256:<digest>` — the
  containerized QEMU/KVM runner.
- `ghcr.io/projectbluefin/donate-clanker-vm-guest@sha256:<digest>` — the
  immutable guest kernel, initramfs/root filesystem, and worker.

The launcher consumes digests, not floating `latest` images. Runner and guest
artifacts are published and maintained by this repository's release process.

Local inference is an explicit external dependency. The FSDK/RamaLama helper
artifact is built and published by
[`projectbluefin/fsdk-containers`](https://github.com/projectbluefin/fsdk-containers);
donate-clanker does not build or vendor that artifact. A deployment selecting
local inference must provide the matching signed immutable FSDK artifact and
its published launch contract. Changes to that image belong in
`projectbluefin/fsdk-containers`, not here.

## Hive setup and onboarding

If `~/.config/hive/contributor.env` is absent, the launcher fetches the pinned
Hive revision and runs the upstream interactive setup:

```bash
just contribute-setup <tool>
```

The setup is upstream-owned and requires an attended terminal. For
non-interactive use, pre-seed the Hive configuration before invoking the
launcher. The selected agent and optional model settings are passed through
the VM bootstrap channel; no host tool configuration directory is mounted.

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

## Legacy compatibility image (historical migration context)

Older releases used `ghcr.io/projectbluefin/donate-clanker` as a wrapper
in **compatibility mode** around the upstream Hive contributor image. That
legacy path required a Lima
VM, guest Podman, explicit `/config` and `/workspace` mounts, and a host
workspace source sometimes selected with `CLANKER_SRC`. It is retained here
only to explain older deployments and is not the product path. It does **not**
include the native Goose/RamaLama launcher and uses no host container socket.
Do not add new Lima, host-workspace, home-directory, socket, or
compatibility-image requirements.

## Source of truth

The upstream contributor workflow and setup remain documented at:

https://hosted-projectbluefin-knuckle-gjvq.hive.kubestellar.io/contribute

The pinned upstream Hive revision is recorded in
`just/61-donate-clanker.just`.
