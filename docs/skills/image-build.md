---
name: image-build
version: "1.0"
last_updated: 2026-07-31
id: image-build
one_line_purpose: Derive and pin the donate-clanker contributor image safely.
entry_point: docs/skills/image-build.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [containerfile, image, digest, pinning, build]
description: "Covers deriving from the pinned KubeStellar Hive contributor image, what donate-clanker layers on top, and digest pinning rules. Use when editing image/Containerfile, adding a layer, or updating a pinned reference."
metadata:
  type: procedure
---

# Image Build

## When to Use

Load this before editing `image/Containerfile`, adding anything to
`image/config/`, or changing a pinned image digest or Hive commit.

## Core Process

### Derive, do not rebuild

The image derives:

```dockerfile
FROM ghcr.io/kubestellar/hive-contributor@sha256:<digest>
```

The base already contains the Hive entrypoint, the contributor protocol
client, tmux, and the agent CLI launch path. Do not reinstall, patch, or
shadow any of it. If you find yourself reimplementing base behavior, stop:
the change probably belongs upstream in `kubestellar/hive`.

### What gets layered in

Only the review context payload:

1. **Goose configuration** under `/opt/bluefin/goose`, referenced by
   `GOOSE_PATH_ROOT`, declaring the Context7 MCP extension. It lives here
   because Hive overwrites `~/.config/goose/config.yaml` at every startup.
2. **Org skills**, generated at build time from `projectbluefin/common`'s
   `docs/skills/index.json` into `/home/dev/.agents/skills/<id>/SKILL.md`.
   Generated during the build, never committed to this repository.
3. **Git hooks** at `/opt/bluefin/git-hooks`, wired through a global
   `core.hooksPath`. Ergonomics only — `git commit --no-verify` bypasses
   them. Enforcement is GitHub rulesets and required status checks.
4. **Agent policy and model configuration** from `image/config/`.

Nothing else. No credentials, no workspace, no host state. Credentials arrive
at runtime as environment variables from the launcher.

### Digest pinning

Every external reference is pinned:

- The base image is pinned by `@sha256:` digest, never by a floating tag such
  as `latest` or a branch name.
- The Hive commit is pinned in `just/61-donate-clanker.just` as
  `hive_commit`, a full 40-character SHA, and is overridable at runtime with
  `DONATE_CLANKER_HIVE_COMMIT`.
- `DONATE_CLANKER_VM_RUNNER_IMAGE` must reference a signed, immutable runner
  image.

Resolve a new digest before changing one:

```bash
skopeo inspect docker://ghcr.io/kubestellar/hive-contributor:<tag> \
  | jq -r '.Digest'
git ls-remote --heads https://github.com/kubestellar/hive v2
```

Bumping a pin is a human decision gate. Record what changed upstream in the
pull request body, and bump base image and Hive commit in separate pull
requests so a regression bisects cleanly.

### Build order

Put slow, rarely-changing layers first and the generated skills tree last;
`index.json` changes far more often than the base packages do.

## Red Flags

- A floating tag, a branch name, or `:latest` anywhere in a `FROM` or an
  image reference.
- Secrets, tokens, or `secrets.env` baked into a layer.
- A committed `.agents/skills/` directory. It is build output.
- Reinstalling tmux, the Hive entrypoint, or an agent CLI the base already
  ships.
- Adding a second agent backend. Goose is the only one.
- Writing Goose config to `~/.config/goose`. Hive erases it at startup.

## Verification

```bash
go test ./...                 # includes image config assertions
gofmt -l .
git diff --check
pre-commit run --all-files
```

Then build and inspect:

```bash
podman build -f image/Containerfile -t donate-clanker:dev .
podman run --rm donate-clanker:dev sh -c \
  'echo "$GOOSE_PATH_ROOT"; ls /opt/bluefin/goose /opt/bluefin/git-hooks; \
   ls /home/dev/.agents/skills | head'
```

Confirm the digest actually changed by comparing `skopeo inspect` output
before and after, and confirm no credential material appears in
`podman history` layer commands.
