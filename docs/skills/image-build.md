---
name: image-build
version: "2.12"
last_updated: 2026-08-04
id: image-build
one_line_purpose: Derive and pin the review contributor image safely.
entry_point: docs/skills/image-build.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [containerfile, image, digest, pinning, build, audit]
description: "Use when maintaining the pinned FSDK-derived contributor image, audit, Goose canary assets, Hive runtime, or publishing path."
metadata:
  type: procedure
---
# Image Build
## When to Use
Load this before changing `image/Containerfile`, `image/config/`, image pins,
or published contributor-image behavior.
## Core Process
1. Derive from the FSDK lab-runner base pinned by a tagged digest
   (`name:tag@sha256:`). The digest is what the build resolves to and is the
   security property; the tag is what makes the pin *trackable*, because a
   reference carrying no tag gives an update manager no version series to
   compare against. A bare digest is not a stricter pin, it is an untracked
   one. Keep the Hive commit equal to the launcher setup commit so both use
   the same protocol revision.
2. Audit the exact base digest at runtime before adding anything. Moving FSDK
   source, image labels, and SBOM package records can disagree with the
   filesystem; command execution and file inspection against the pinned digest
   define the base interface.
3. Add only the contributor delta: Goose, tmux, GitHub CLI, Node with `ws`,
   the pinned Hive runtime, controlled policy/configuration, and approved agent
   tools. Do not duplicate a capability already present in the verified base.
   Do not turn the image into a general-purpose distribution.
4. Preserve canonical command semantics. Never shadow `grep`, `find`, `cat`, or
   `ls` with modern alternatives. If a modern tool is added, install it under
   its own native name (e.g. `rg`) beside the canonical command, never as a
   replacement; none is installed today.
5. Missing standard runtime utilities belong at the FSDK seam. Prefer real
   FSDK-owned findutils, diffutils, and terminfo over local Python
   replacements; if the pinned artifact cannot provide them, record the base
   gap rather than inventing a second implementation. An interim shim must
   satisfy every caller, not just the agent: Hive's relay prunes stale `/tmp`
   every ten minutes with `-maxdepth`, `-type`, `-user`, `-not`, `-name`,
   `-mmin` and `-exec ... +` and discards its stderr, so a shim rejecting
   those fails invisibly. Match GNU precedence and fail loudly on an
   unimplemented predicate; `tests/find-semantics.sh` pins the expressions.
6. Pin Node, GitHub CLI, and tmux versions and verify their checksums. For
   mutable Goose `canary`, CI resolves official `unknown-linux-musl` asset
   digests before each build, passes them as build inputs, and records them in
   image configuration and provenance. The build verifies the selected archive
   checksum and `gh attestation verify` provenance against the official
   repository and `canary.yml`; a moved asset fails rather than silently
   changing an image. Extract safely; never compile, strip, repack, or fork
   Goose; preserve glibc loader links for dynamic Node and GitHub CLI. Lock `ws` in root `package-lock.json` with `npm ci --omit=dev --ignore-scripts`; keep fixed Node/gh/tmux/ws ahead of mutable Goose. Remove only Node headers, `/opt/node/share/doc`, and verified-unused npm cache; retain `node`, `npm`, and `corepack`.
7. Place controlled Goose configuration under `/opt/bluefin/goose` as the
   image-owned policy, data, and state seam. Revalidate compatibility settings
   against the pinned Hive runtime before retaining them; do not preserve stale
   workarounds solely because an older Hive revision needed them. The current
   pin preserves its runtime config when present and creates Goose-native
   `AGENTS.md` and `.goosehints` links for refreshed knowledge, so do not add a
   `CONTEXT_FILE_NAMES` compatibility override for legacy `CLAUDE.md`.
8. Generate org skills at build time from the pinned common catalog into
   `/home/dev/.agents/skills`. Review the generator and catalog inputs, never
   generated output. Remove build-only generation tooling from the final
   filesystem when the build shape permits it.
9. Keep credentials, workspaces, and host configuration out of image layers.
   Supply the GitHub token used for canary provenance verification as the
   required `github_token` build secret; it is available only to that `RUN`
   step and must not be an argument or environment layer.
10. Treat the image as a task runtime, not a general validation distribution.
   At startup, probe the baseline validation commands (`bats`, `shellcheck`,
   `systemd-analyze`, `pre-commit`, `just`, and `podman`) and report only the
   missing ones — `just` comes from the FSDK base — without blocking Hive or
   installing them solely to hide the absence.
11. Before an FSDK pin is built, audit its immutable input with
    `bash tests/image-audit.sh --verify-base-evidence`; it verifies the
    `projectbluefin/fsdk-containers` GitHub attestation and exactly native
    linux/amd64 and linux/arm64 manifests. Audit each derived build with
    `bash tests/image-audit.sh --derived <image>`.
    It records exact-base manifest, composition, OCI, command, terminal, user,
    loader, and package-manager facts in a CI summary, never git. Publishing
    requires explicit BuildKit `provenance: mode=max` and `sbom: true`, a
    GitHub artifact attestation for the pushed digest, and post-publish
    verification of exactly those two platforms, OCI labels/annotations, both
    BuildKit attestations, and the GitHub attestation. Never call QEMU runtime
    proof native.
12. Measure compressed manifest, unpacked filesystem, layer/directory deltas, cold/warm builds, and native amd64/arm64 runtime behavior before and after each composition change. Deleting inherited files in a later layer does not reclaim the base layer.
The publish workflow moves `:stable` on main. It also publishes immutable
`sha-<commit>` tags; use an immutable tag or digest when reproducibility is
required. Do not use `:latest`.
## Pin Maintenance
**An unmaintainable pin is a stale pin.** A pin's strictness is worthless if no
automation can see past it, and a frozen pin raises no failing check — it looks
maximally strict while being maximally stale. Both pins in this image reached
that state at once: the Hive commit had no manager able to match it, and the
FSDK base carried a digest with no tag, so neither was ever proposed for
update. When adding or reshaping a pin, establish its update path in the same change and prefer a reference shape a manager can resolve. The Hive SHA lives in three places that must move together in one commit:

| Location | Form |
|---|---|
| `justfile` | `hive_commit := "<sha>"` |
| `image/Containerfile` | `ARG HIVE_COMMIT=<sha>` |
| `README.md` | the bare SHA in prose |

CI enforces this: `tests/image-contract.sh` requires the launcher and image
pins to be equal, and `.github/workflows/validate.yml` requires `README.md` to
contain the launcher pin. Updating any two of the three fails the build.
Hive's default branch is `v2`, not `main`. Resolve a candidate SHA from `v2`
and use the full 40-character commit; the launcher rejects a branch name.

Hive is a **protocol** dependency, not a library. The image consumes exactly
three upstream files — `bin/contributor-agent.sh`, `bin/contributor-relay.sh`,
and `config/backends.conf`. A bump is only safe to automerge when those three
are unchanged between the old and new SHA; otherwise read the diff and update
[`hive-runtime.md`](hive-runtime.md) and [`hive-triage.md`](hive-triage.md)
in the same change. That condition is machine-checked by
`.github/workflows/hive-pin-gate.yml`, which derives the consumed-file list
from `image/Containerfile` rather than from a hand-maintained list; keep the
two in step when the image starts or stops consuming an upstream file.

Never add a downstream workaround for an upstream protocol gap. Moving the pin
is the fix; a local retry, poll, timeout, or shim becomes a permanent
compatibility burden for both sides. See
[`upstream-hive.md`](upstream-hive.md).

## When Not to Use

Do not use this runbook to change Hive assignment, checkout, or contributor protocol behavior; those belong in Hive. Do not add task-specific validation dependencies when an unavailable-command report is sufficient. Do not switch the final image to a shell-less base or copy a hand-selected dynamic-library closure from another distribution.

## Common Rationalizations

- "Renovate covers it." Confirm the pin's shape is one a manager can resolve. A
  bare `image@sha256:` reference and an unmanaged shell variable both look
  pinned and never move.
- "A digest with no tag is the safest possible pin." It is the safest *build*
  and the least maintainable pin. Safety that decays unobserved is not safety;
  carry the tag so the digest moves forward deliberately.
- "It's only a SHA bump, automerge it." Hive is a protocol dependency; verify the
  three consumed files are unchanged first.
- "Adding every validator makes contributors more useful." The image must stay a
  narrow contributor runtime; report a missing tool and let the assigned
  repository choose its validation environment.
- "Replacing grep/find/cat/ls makes every agent faster." These commands are a
  script interface, and the scripts are not all yours: Hive's relay calls `find`
  too. Install modern tools beside them, never substituting semantics.
- "A multi-stage build makes copied libraries safe." Multi-stage syntax does not
  make a manually selected ABI closure maintainable. Keep the final image
  directly on the verified FSDK shell base.
- "Passing `--env NAME=value` is harmless." Podman exposes command arguments
  locally; export the value and use `--env NAME` so Podman inherits only that
  host environment entry.

## Red Flags

- A floating base image or unverified download. Goose's canary source is
  mutable by design, but its archive needs verified signed provenance.
- A bare-digest reference with no tag, or any pin with no update path: untrackable,
  silently frozen, and reads as the file's strictest pin.
- Treating current FSDK source or labels as proof of an older digest.
- A Hive pin differing from the launcher setup pin, a bump moving fewer than all
  three locations, or an automerge whose consumed upstream files changed.
- A secret, host workspace, or host configuration baked into a layer.
- Writing Goose configuration to `~/.config/goose`.
- Keeping a legacy `CONTEXT_FILE_NAMES` override once the pinned Hive runtime
  provides Goose-native knowledge links.
- Committing generated `.agents/skills/` output, or adding a second agent
  backend or unrelated runtime package.
- A custom compile, repacked bundle, package manager, command shadow, or copied
  cross-distribution closure.

## Verification

```bash
bash tests/image-contract.sh
bash tests/hive-compatibility.sh
bash tests/generate-skills.sh
bash tests/image-audit.sh --verify-base-evidence
grep -Fq "$(sed -n 's/^hive_commit := "\(.*\)"$/\1/p' justfile)" README.md
GH_TOKEN="$(gh auth token)" podman build \
  --secret id=github_token,env=GH_TOKEN \
  --build-arg GOOSE_REFRESH="$(date +%s)" \
  -f image/Containerfile -t review:dev .
bash tests/image-audit.sh --derived review:dev
git diff --check
```

Inspect the built image only for the controlled Goose root and generated skill
directories; never expose credentials. The audit reads public manifests and local
metadata; use native amd64 and arm64 hosts for runtime evidence. To refresh local
canary, resolve both release-asset digests and pass `GOOSE_X86_64_SHA256` and
`GOOSE_AARCH64_SHA256`; never use the channel name as artifact identity. Build off
the workstation: a warm native amd64 build plus `--derived` audit takes about
seventy seconds on a lab node with a persistent `/var/lib/containers`, but never
replaces the publish workflow, which alone yields both platforms, provenance, SBOM,
and the attestation. Run `tests/find-semantics.sh` against the shim installed in
the image; that copy, not the checkout's, is what Hive's relay calls.
## Sources
- Hive `v2` files: `bin/contributor-agent.sh`, `bin/contributor-relay.sh`, and `config/backends.conf`; Goose `canary` assets; Context7 `/npm/cli`, `/websites/podman_io_en`, `/websites/cli_github_manual`, `/websites/github_en_actions`, `/docker/docs`, and `/docker/build-push-action`.
