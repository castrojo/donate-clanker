# FSDK RamaLama Image Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish a signed, multi-architecture FSDK-based RamaLama distribution image that the donate-clanker launcher can use as its local-inference helper.

**Architecture:** Add a dedicated FSDK BuildStream image using the repository's stack/compose/OCI pattern. Keep it distroless and runtime-only, consume FSDK components or an official upstream artifact rather than maintaining a package set, and register it in every existing build, verify, SBOM, and manifest matrix.

**Tech Stack:** BuildStream 2, FSDK components, OCI Builder, Podman, Just, GitHub Actions, Cosign, BuildStream-native SPDX SBOMs.

## Global Constraints

- Compose from `components/*`, never `platform.bst`.
- Keep the image distroless: no shell, no package manager, and no unnecessary build/runtime domains.
- Do not add `x86_64_v3`.
- Use the existing shared SLIM recipe and `just verify` gates.
- Build and publish `linux/amd64` and `linux/arm64`.
- Keep all GitHub Actions references SHA-pinned.
- Generate BuildStream-native SBOMs and sign the manifest-list digest and SBOM referrer.
- Follow the existing FSDK release labels and immutable point-release tag policy.

---

### Task 1: Determine the RamaLama source and runtime dependency graph

**Files:**
- Inspect: `elements/freedesktop-sdk.bst`
- Inspect: `elements/base/base-stack.bst`
- Inspect: `elements/base/base-runtime.bst`
- Inspect: `Justfile`
- Modify: `docs/skills/ramalama-image.md`

- [ ] **Step 1: Check for FSDK components** with `just bst show freedesktop-sdk.bst:components/<candidate>` for the RamaLama runtime and its transitive dependencies.
- [ ] **Step 2: Check the upstream RamaLama distribution contract** for an official binary/container artifact and identify whether it can run without a shell/package manager.
- [ ] **Step 3: Choose the smallest supported source path**: FSDK component when present, otherwise an official upstream static/runtime artifact consumed without rebuilding an existing maintained tool.
- [ ] **Step 4: Record the source, version, hashes, runtime dependencies, GPU handoff, and required entrypoint in `docs/skills/ramalama-image.md`.**
- [ ] **Step 5: Commit** with `docs: document ramalama image source`.

### Task 2: Add the BuildStream stack and runtime elements

**Files:**
- Create: `elements/ramalama/ramalama-stack.bst`
- Create: `elements/ramalama/ramalama-runtime.bst`
- Test: `elements/ramalama/ramalama-element-test.sh`

- [ ] **Step 1: Write the element graph check** that resolves `elements/oci/ramalama.bst` and asserts the base stack, CA certificates, timezone data, and selected RamaLama dependencies are present.
- [ ] **Step 2: Run `just validate`** and verify failure because the new elements are not registered.
- [ ] **Step 3: Implement the stack** by inheriting the existing base essentials and adding only the verified RamaLama runtime dependencies.
- [ ] **Step 4: Implement the compose runtime** by copying the base exclude list for debug/devel/doc/locale/static-blocklist/vm-only/tests/shells and adding only verified RamaLama-specific pruning.
- [ ] **Step 5: Run `just validate`** and verify the graph resolves for both target architectures.
- [ ] **Step 6: Commit** with `feat: add ramalama fsdk runtime elements`.

### Task 3: Add the OCI image and verification gates

**Files:**
- Create: `elements/oci/ramalama.bst`
- Modify: `Justfile`
- Modify: `docs/skills/verify-distroless.md`
- Modify: `README.md`

- [ ] **Step 1: Add a failing verify branch** for the RamaLama executable, required CA/tzdata files, shell absence, and an image-size ceiling calibrated with headroom for both architectures.
- [ ] **Step 2: Run `just verify`** and verify the new branch fails before the image is built.
- [ ] **Step 3: Implement the OCI script** with the shared SLIM block, image title, FSDK version/ref labels, and `ghcr.io/projectbluefin/ramalama:latest` index annotation.
- [ ] **Step 4: Register `ramalama` in `Justfile`** image selection, `just validate`, `just verify`, `just sbom`, and `just sboms` case lists.
- [ ] **Step 5: Add the README image row** including the private endpoint contract, model-cache path, GPU/runtime behavior, and no-shell limitation.
- [ ] **Step 6: Run `just validate && just build && just verify`** for the local architecture and record the actual size ceiling and smoke command.
- [ ] **Step 7: Commit** with `feat: publish fsdk ramalama image`.

### Task 4: Add SBOM, signing, and multi-architecture publishing

**Files:**
- Modify: `.github/workflows/build.yml`
- Modify: `Justfile`
- Test: `.github/workflows/ramalama-workflow-check.sh`

- [ ] **Step 1: Add a workflow check** that fails when `ramalama` is absent from either architecture matrix, manifest loop, or SBOM case list.
- [ ] **Step 2: Add the image to the existing per-architecture build matrix** with `fail-fast: false`.
- [ ] **Step 3: Add the image to manifest assembly** and preserve the requirement that both architectures succeed before rolling/minor tags move.
- [ ] **Step 4: Generate the BuildStream-native SBOM** with the existing pinned `buildstream-sbom` cache and attach/sign the SBOM referrer.
- [ ] **Step 5: Sign the resolved manifest-list digest** with keyless Cosign and retain GitHub artifact provenance permissions.
- [ ] **Step 6: Run the workflow check and inspect the rendered workflow** for full SHA pins, required permissions, and no mutable action references.
- [ ] **Step 7: Commit** with `ci: publish ramalama image`.

### Task 5: Update FSDK skill knowledge and consumer contract

**Files:**
- Modify: `docs/skills/README.md`
- Modify: `docs/skills/ramalama-image.md`
- Modify: `README.md`

- [ ] **Step 1: Document the image's source and runtime boundary**: RamaLama helper only, private model endpoint, persistent cache, and launcher-owned cleanup.
- [ ] **Step 2: Document local verification** with `just validate`, `just build`, `just verify`, `just sbom ramalama`, and signature/attestation checks.
- [ ] **Step 3: Document the exact immutable image reference format** consumed by donate-clanker.
- [ ] **Step 4: Run the documentation link/target review** and verify every command and path exists.
- [ ] **Step 5: Commit** with `docs: document ramalama consumer contract`.

---

## Execution Order

Task 1 must finish before the element graph is written. Tasks 2 and 3 are
sequential because verification depends on the OCI image. Task 4 follows a
successful local build/verify. Task 5 is the handoff gate for the
donate-clanker launcher.
