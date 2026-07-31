# donate-clanker Skill Router

Read [`AGENTS.md`](../AGENTS.md) first. Load only the skill relevant to the
task, then validate with the repository's existing checks.

## Skill index

| Task | Skill |
| --- | --- |
| Change Hive credentials, Goose execution, mounts, or assignment prompts | [`worker-credential-boundary.md`](skills/worker-credential-boundary.md) |
| Change native contributor assignment isolation, local observations, or helper profile launch behavior | [`worker-credential-boundary.md`](skills/worker-credential-boundary.md) |
| Onboard or audit this repository against the factory | [`factory-onboarding.md`](skills/factory-onboarding.md) |
| Change GitHub Actions workflows or CI-only validation | [`ci-tooling.md`](skills/ci-tooling.md) |
| Check release health or VM artifact publication ownership | [`release.md`](skills/release.md) |
| Record a durable workaround or repository convention | [`skill-improvement.md`](skills/skill-improvement.md) |

Factory-wide policy lives in
[`projectbluefin/common`](https://github.com/projectbluefin/common). This
repository links to common policy rather than copying it; local repository
rules remain authoritative for launcher, worker, and release behavior.

## Approved runtime boundary

The supported product is a disposable, self-contained QEMU microVM launched
from a pinned containerized runner on Bluefin base or Bluefin DX hosts. The
guest clones Hive-assigned repositories internally. It receives no host
workspace, home directory, tool configuration, or container socket mount.
Lima, guest Podman, `CLANKER_SRC`, and the compatibility image are historical
migration context only, not current product requirements.

The launcher requires an immutable runner image reference and usable KVM.
Local inference is an explicit external dependency on a matching signed FSDK
artifact published by `projectbluefin/fsdk-containers`; this repository does
not own that artifact. VM runner and guest OCI artifacts are likewise external
inputs: `.github/workflows/publish-vm.yml` validates their immutable
references but does not publish them. Keep VM, guest-clone, host-boundary, and
release-ownership documentation consistent with this model without changing
skill-boundary documents.
