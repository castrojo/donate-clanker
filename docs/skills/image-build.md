---
name: image-build
version: "1.3"
last_updated: 2026-08-01
id: image-build
one_line_purpose: Derive and pin the donate-clanker contributor image safely.
entry_point: docs/skills/image-build.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [containerfile, image, digest, pinning, build]
description: "Covers deriving from the Project Bluefin FSDK base, which Hive contributor runtime pieces donate-clanker layers on top, digest pinning rules, and which registry tags each ref publishes. Use when editing image/Containerfile, a layer, or publish tagging."
metadata:
  type: procedure
---

# Image Build

## When to Use

Load this before editing `image/Containerfile`, adding anything to
`image/config/`, or changing a pinned image digest or Hive commit.

Not for what the launcher does with the built image -- that is
[`launcher.md`](launcher.md).

## Core Process

### Derive, then add only the missing runtime pieces

The image derives:

```dockerfile
FROM ghcr.io/projectbluefin/lab-runner@sha256:<digest>
```

The base provides the FSDK shell-enabled runtime: bash, curl, git, jq,
python3, openssh, and the libc/userland we inherit from Project Bluefin.
donate-clanker then adds only what contributor mode still needs:

- Goose
- tmux
- Node + `ws` for Hive's relay
- GitHub CLI
- the pinned Hive contributor scripts (`contributor-agent.sh`,
  `contributor-relay.sh`, `backends.conf`)

Do not grow this into a second general-purpose distro. If a new package is not
needed for Goose contributor mode, it probably does not belong here.

### What gets layered in

Only the contributor runtime delta plus the review context payload:

1. **Pinned contributor runtime files** fetched from the pinned Hive commit:
   `contributor-agent.sh`, `contributor-relay.sh`, and `backends.conf`.
2. **Goose configuration** under `/opt/bluefin/goose`, referenced by
   `GOOSE_PATH_ROOT`, declaring the Context7 MCP extension. It lives here
   because Hive overwrites `~/.config/goose/config.yaml` at every startup.
3. **Org skills**, generated at build time from `projectbluefin/common`'s
   `docs/skills/index.json` into `/home/dev/.agents/skills/<id>/SKILL.md`.
   Generated during the build, never committed to this repository.
4. **Git hooks** at `/opt/bluefin/git-hooks`, wired through a global
   `core.hooksPath`. Ergonomics only — `git commit --no-verify` bypasses
   them. Enforcement is GitHub rulesets and required status checks.
5. **Agent policy and model configuration** from `image/config/`.

Nothing else. No credentials, no workspace, no host state. Credentials arrive
at runtime as environment variables from the launcher.

Each of those four is pinned down by `tests/image-contract.sh`, which asserts
the required lines are present and the forbidden ones absent across the
`Containerfile`, `goose.yaml`, `local-agent-policy.md`, the entrypoint and the
git hooks. They are substring assertions over files, so they are grep and need
no toolchain -- add to that script when you add a layer, and check that it
still fails when you break the thing it claims to protect.

### Digest pinning

Every external reference is pinned:

- The FSDK base image is pinned by `@sha256:` digest, never by a floating tag
  such as `latest` or a branch name.
- The Hive commit is pinned in `just/61-donate-clanker.just` as
  `hive_commit`, a full 40-character SHA, and is overridable at runtime with
  `DONATE_CLANKER_HIVE_COMMIT`.
- `DONATE_CLANKER_VM_RUNNER_IMAGE` must reference a signed, immutable runner
  image.

Resolve a new digest before changing one:

```bash
skopeo inspect docker://ghcr.io/projectbluefin/lab-runner:<tag> \
  | jq -r '.Digest'
git ls-remote --heads https://github.com/kubestellar/hive v2
```

Bumping a pin is a human decision gate. Record what changed upstream in the
pull request body, and bump base image and Hive commit in separate pull
requests so a regression bisects cleanly.

### Build order

Put slow, rarely-changing layers first and the generated skills tree last;
`index.json` changes far more often than the base packages do.

### Publishing and tags

`.github/workflows/publish-compat-image.yml` publishes
`ghcr.io/projectbluefin/donate-clanker`. The ref alone decides the shape:

| Ref | Tags pushed | Platforms | Cancellable |
|---|---|---|---|
| `refs/tags/v*.*.*` | `sha-<commit>`, `X.Y.Z`, `vX.Y.Z`, `stable` | amd64 + arm64 | never |
| `refs/heads/main` | `sha-<commit>`, `main` | amd64 | yes |
| dispatch elsewhere | `sha-<commit>` | amd64 | yes |

`stable` is the released line and the launcher's default; only a version tag
moves it. `main` is the newest merge, so a merged fix is consumable without
cutting a release:
`DONATE_CLANKER_CONTRIBUTOR_IMAGE=ghcr.io/projectbluefin/donate-clanker:sha-<commit>`.
`:latest` is never published — a default pointing at it dies on
`manifest unknown`, and `tests/just-onboarding.sh` fails the build if one
appears.

Main builds skip emulated arm64 because it costs ten-plus minutes per merge.
Never trade a release's architectures, smoke-test assertions, or digest
verification for speed.

The repository variable `DONATE_CLANKER_PUBLISH_STABLE` once gated the
`stable` alias on tag builds. The workflow stopped reading it in `bc8f157`;
the variable still exists but is inert. Do not repurpose it to gate branch
tagging.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "A tag is fine, the digest is a hassle to resolve." | A floating base image makes the build irreproducible and unbisectable. |
| "Goose config belongs in `~/.config/goose`." | Hive overwrites that file at every startup. `GOOSE_PATH_ROOT` exists for exactly this reason. |
| "I'll add the contract assertion in a follow-up." | An unasserted layer is a layer that silently disappears on the next refactor. Same PR. |

## Red Flags

- A floating tag, a branch name, or `:latest` anywhere in a `FROM` or an
  image reference.
- Secrets, tokens, or `secrets.env` baked into a layer.
- A committed `.agents/skills/` directory. It is build output.
- Pulling in a full distro package manager or a second general-purpose shell
  environment just to add one more tool.
- Adding a second agent backend. Goose is the only one.
- Writing Goose config to `~/.config/goose`. Hive erases it at startup.
- A new layer with no matching assertion in `tests/image-contract.sh`.
- Reaching for a language runtime to assert that a string appears in a file.

## Verification

```bash
bash tests/image-contract.sh  # Containerfile, goose.yaml, entrypoint, hooks
git diff --check
pre-commit run --all-files
```

Then build and inspect:

```bash
podman build -f image/Containerfile -t donate-clanker:dev .
podman run --rm donate-clanker:dev /usr/bin/bash -lc \
  'echo "$GOOSE_PATH_ROOT"; ls /opt/bluefin/goose /opt/bluefin/git-hooks; \
   ls /home/dev/.agents/skills | head'
```

Confirm the digest actually changed by comparing `skopeo inspect` output
before and after, and confirm no credential material appears in
`podman history` layer commands.
