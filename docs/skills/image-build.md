---
name: image-build
version: "2.3"
last_updated: 2026-08-04
id: image-build
one_line_purpose: Derive and pin the review contributor image safely.
entry_point: docs/skills/image-build.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [containerfile, image, digest, pinning, build, renovate]
description: "Maintains the pinned FSDK-derived contributor image, Hive runtime, Goose configuration, generated skills, and the pin update path. Use when editing image layers, pins, or image publication behavior."
metadata:
  type: procedure
---

# Image Build

## When to Use

Load this before changing `image/Containerfile`, `image/config/`, image pins,
or published contributor-image behavior.

## Core Process

1. Derive from the FSDK lab-runner base pinned by a tagged digest
   (`name:tag@sha256:`). The tag gives Renovate's `dockerfile` manager a
   resolvable reference; the digest is what the build actually uses. Keep the
   Hive commit in the image equal to the launcher setup commit so both use the
   same protocol revision.
2. Audit the exact base digest at runtime before adding anything. Moving FSDK
   source, image labels, and SBOM package records can disagree with the
   filesystem; command execution and file inspection against the pinned digest
   define the base interface.
3. Add only the contributor delta: Goose, tmux, GitHub CLI, Node with `ws`,
   the pinned Hive runtime, controlled policy/configuration, and approved agent
   tools. Do not duplicate a capability already present in the verified base.
   Do not turn the image into a general-purpose distribution.
4. Preserve canonical command semantics. Never shadow `grep`, `find`, `cat`, or
   `ls` with modern alternatives. Add tools such as `rg` under their native
   names and teach agents to prefer them for exploration.
5. Missing standard runtime utilities belong at the FSDK seam. Prefer real
   FSDK-owned findutils, diffutils, and terminfo over local Python command
   replacements or reconstructed databases. If the existing FSDK artifact
   cannot provide them, record the base gap rather than inventing a second
   implementation.
6. Pin Node, GitHub CLI, and tmux versions and verify their checksums. Goose
   follows upstream's mutable `canary` release instead: verify each archive
   with `gh attestation verify`, constrained to the official repository and
   `canary.yml` signer workflow. Keep archive extraction constrained to safe
   archive members. Prefer an official static-musl artifact when it is
   behaviorally equivalent on both architectures; never compile or repack
   Goose locally. Lock `ws` and disable lifecycle scripts.
7. Place controlled Goose configuration under `/opt/bluefin/goose` as the
   image-owned policy, data, and state seam. Revalidate compatibility settings
   against the pinned Hive runtime before retaining them; do not preserve stale
   workarounds solely because an older Hive revision needed them.
8. Generate org skills at build time from the pinned common catalog into
   `/home/dev/.agents/skills`. Review the generator and catalog inputs, never
   generated output. Remove build-only generation tooling from the final
   filesystem when the build shape permits it.
9. Keep credentials, workspaces, and host configuration out of image layers.
   Supply the GitHub token used for canary provenance verification as the
   required `github_token` build secret; it is available only to that `RUN`
   step and must not be an argument or environment layer.
10. Treat the image as a task runtime, not a general validation distribution.
   At startup, report unavailable baseline validation commands (`bats`,
   `shellcheck`, `systemd-analyze`, `pre-commit`, `just`, and `podman`) without
   blocking Hive or installing them solely to hide the absence.
11. Measure before and after every composition change: compressed manifest,
    unpacked filesystem, layer and directory deltas, cold/warm builds, and
    native amd64/arm64 runtime behavior. Deleting inherited files in a later
    layer does not reclaim the base layer.

The publish workflow moves `:stable` on main. It also publishes immutable
`sha-<commit>` tags; use an immutable tag or digest when reproducibility is
required. Do not use `:latest`.

## Pin Maintenance

Every pin in this image needs an update path. A pin with no update path is the
root-cause bug class here: the Hive pin had no Renovate manager at all, and
the FSDK base pinned a bare digest with no tag, so Renovate's `dockerfile`
manager silently never fired on either. Both stayed frozen without any failing
check. When adding a pin, add or confirm its manager in the same change, and
give an image reference a `name:tag@sha256:` form so the digest is trackable.

The Hive SHA lives in three places that must move together in one commit:

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

Do not use this runbook to change Hive assignment, checkout, or contributor
protocol behavior; those belong in Hive. Do not add task-specific validation
dependencies to the image when an unavailable-command report is sufficient.
Do not switch the final image to a shell-less base or copy a hand-selected
dynamic-library closure from another distribution.

## Common Rationalizations

- "Renovate covers it." Confirm the manager actually matches the pin's syntax.
  A bare `image@sha256:` reference and an unmanaged shell variable both look
  pinned and never move.
- "It's only a SHA bump, automerge it." Hive is a protocol dependency; verify
  the three consumed files are unchanged before trusting an automerge.
- "Adding every validator makes contributors more useful." The image must stay
  a narrow contributor runtime; report a missing tool and let the assigned
  repository choose its validation environment.
- "Replacing grep/find/cat/ls makes every agent faster." These commands are a
  script interface. Install modern tools alongside them; never substitute
  incompatible semantics.
- "A multi-stage build makes copied libraries safe." Multi-stage syntax does
  not make a manually selected ABI closure maintainable. Keep the final image
  directly on the verified FSDK shell base.
- "Passing `--env NAME=value` is harmless." Podman exposes command arguments
  locally; export the value and use `--env NAME` so Podman inherits only that
  host environment entry.

## Red Flags

- A floating base image or an unverified download. Goose's canary source is
  intentionally mutable, but its archive must have verified signed provenance.
- A new pin with no Renovate manager, or a digest-only image reference that no
  manager can resolve.
- Treating current FSDK source or labels as proof of an older pinned digest's
  runtime contents.
- A Hive pin that differs from the launcher setup pin, or a Hive bump that
  moves fewer than all three pin locations.
- Automerging a Hive bump whose consumed upstream files changed.
- A secret, host workspace, or host configuration baked into a layer.
- Writing Goose configuration to `~/.config/goose`.
- Committing generated `.agents/skills/` output.
- Adding a second agent backend or unrelated runtime package.
- A custom compile, repacked binary bundle, package manager, command shadow, or
  copied cross-distribution library closure.

## Verification

```bash
bash tests/image-contract.sh
bash tests/generate-skills.sh
grep -Fq "$(sed -n 's/^hive_commit := "\(.*\)"$/\1/p' justfile)" README.md
GH_TOKEN="$(gh auth token)" podman build \
  --secret id=github_token,env=GH_TOKEN \
  --build-arg GOOSE_REFRESH="$(date +%s)" \
  -f image/Containerfile -t review:dev .
git diff --check
```

Inspect the built image only for the controlled Goose root and generated skill
directories; never use image history or command output to expose credentials.
Verify the exact FSDK input digest's attestation and runtime command contract,
then verify the derived image on native amd64 and arm64.

## Sources

- Hive consumed files and default branch `v2`: `kubestellar/hive`
  `bin/contributor-agent.sh`, `bin/contributor-relay.sh`,
  `config/backends.conf`.
- Podman environment inheritance: Context7 `/websites/podman_io_en`
- GitHub CLI attestation verification: Context7 `/websites/cli_github_manual`
- Docker image and multi-stage best practices: Context7 `/docker/docs`
