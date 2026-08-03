---
name: image-build
version: "2.0"
last_updated: 2026-08-02
id: image-build
one_line_purpose: Derive and pin the donate-clanker contributor image safely.
entry_point: docs/skills/image-build.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [containerfile, image, digest, pinning, build]
description: "Maintains the pinned FSDK-derived contributor image, Hive runtime, Goose configuration, and generated skills. Use when editing image layers, pins, or image publication behavior."
metadata:
  type: procedure
---

# Image Build

## When to Use

Load this before changing `image/Containerfile`, `image/config/`, image pins,
or published contributor-image behavior.

## Core Process

1. Derive from the digest-pinned FSDK lab-runner base. Keep the Hive commit in
   the image equal to the launcher setup commit so both use the same protocol
   revision.
2. Add only the contributor delta: Goose, tmux, GitHub CLI, Node with `ws`,
   the pinned Hive runtime, the minimal `cmp` compatibility wrapper, and the
   required terminal definition. Do not turn the image into a general-purpose
   distribution.
3. Pin downloaded tool versions and verify their checksums. Keep archive
   extraction constrained to safe archive members. Install `ws` with
   `npm --ignore-scripts`; advisory churn is not an image-build gate.
4. Place controlled Goose configuration under `/opt/bluefin/goose`, because
   Hive overwrites the default config path. The entrypoint sets the Copilot
   provider, default model, telemetry setting, context file names, and
   image-owned agent policy before it starts Hive.
5. Generate org skills at build time from the pinned common catalog into
   `/home/dev/.agents/skills`. Review the generator and catalog inputs, never
   generated output.
6. Keep credentials, workspaces, and host configuration out of image layers.

The publish workflow moves `:stable` on main. It also publishes immutable
`sha-<commit>` tags; use an immutable tag or digest when reproducibility is
required. Do not use `:latest`.

## Red Flags

- A floating base image, mutable external source, or unverified download.
- A Hive pin that differs from the launcher setup pin.
- A secret, host workspace, or host configuration baked into a layer.
- Writing Goose configuration to `~/.config/goose`.
- Committing generated `.agents/skills/` output.
- Adding a second agent backend or unrelated runtime package.

## Verification

```bash
bash tests/image-contract.sh
bash tests/generate-skills.sh
podman build -f image/Containerfile -t donate-clanker:dev .
git diff --check
```

Inspect the built image only for the controlled Goose root and generated skill
directories; never use image history or command output to expose credentials.
