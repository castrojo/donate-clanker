# Goose Canary Snapshots Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the contributor image with the current, provenance-verified
Goose canary snapshot.

**Architecture:** The Containerfile downloads the fixed-name archive from the
mutable `canary` release for the build architecture. It uses the already
installed GitHub CLI to verify the archive's signed SLSA provenance, constrained
to the upstream repository and canary workflow, before extracting the binary.
The contract test and README make the mutable-channel tradeoff explicit.

**Tech Stack:** Containerfile, Bash contract test, GitHub CLI attestations,
GitHub Releases, Podman.

## Global Constraints

- Follow `aaif-goose/goose`'s `canary` release, which is rebuilt from `main`.
- Verify each archive with `gh attestation verify` using
  `aaif-goose/goose/.github/workflows/canary.yml` as the signer workflow.
- Keep the FSDK base image and Hive revision pinned.
- Do not change launcher behavior, Hive behavior, Copilot-only configuration,
  or controlled Goose configuration.
- Do not fall back to Goose 1.x or install an unverified archive.

---

### Task 1: Verify canary provenance during the image build

**Files:**
- Modify: `image/Containerfile:17-98`
- Test: `tests/image-contract.sh:45-65`

**Interfaces:**
- Consumes: GitHub release archive URLs at
  `https://github.com/aaif-goose/goose/releases/download/canary/goose-<arch>-unknown-linux-gnu.tar.gz`.
- Produces: `/usr/local/bin/goose` extracted only after
  `gh attestation verify` succeeds.

- [ ] **Step 1: Extend the failing source contract**

  Require the `canary` URL and the provenance verifier:

  ```bash
  require image/Containerfile \
    'ARG GOOSE_CHANNEL=canary' \
    'releases/download/${GOOSE_CHANNEL}/goose-${goose_arch}-unknown-linux-gnu.tar.gz' \
    'gh attestation verify "$workdir/goose.tar.gz" --repo aaif-goose/goose --signer-workflow aaif-goose/goose/.github/workflows/canary.yml'
  ```

- [ ] **Step 2: Run the contract to verify it fails**

  Run: `bash tests/image-contract.sh`

  Expected: failure reporting the missing canary channel and provenance
  verification assertions.

- [ ] **Step 3: Replace static version checksums with signed provenance**

  Remove `ARG GOOSE_VERSION` and both architecture-specific `goose_sha`
  assignments. Add `ARG GOOSE_CHANNEL=canary`; download the archive from that
  release; immediately run:

  ```bash
  gh attestation verify "$workdir/goose.tar.gz" \
    --repo aaif-goose/goose \
    --signer-workflow aaif-goose/goose/.github/workflows/canary.yml
  ```

  Keep archive extraction and `goose --version`/`goose run --help` checks after
  verification.

- [ ] **Step 4: Run the contract to verify it passes**

  Run: `bash tests/image-contract.sh`

  Expected: exit status 0.

- [ ] **Step 5: Commit**

  ```bash
  git add image/Containerfile tests/image-contract.sh
  git commit -m "feat: follow Goose canary snapshots"
  ```

### Task 2: Document mutable snapshot behavior

**Files:**
- Modify: `README.md:135-151`
- Test: `tests/image-contract.sh:45-65`

**Interfaces:**
- Consumes: the `GOOSE_CHANNEL=canary` build contract from Task 1.
- Produces: contributor-facing documentation that describes the mutable build
  input and immutable image-tag alternative.

- [ ] **Step 1: Add a failing documentation source assertion**

  Require the README to describe the upstream canary channel:

  ```bash
  require README.md \
    'Goose canary snapshot' \
    'not byte-reproducible'
  ```

- [ ] **Step 2: Run the contract to verify it fails**

  Run: `bash tests/image-contract.sh`

  Expected: failure reporting the missing README snapshot assertions.

- [ ] **Step 3: Add concise user documentation**

  State that each contributor-image build downloads the current Goose canary
  snapshot and verifies GitHub build provenance. State that rebuilding the same
  source revision at different times may use different Goose binaries; users
  needing a fixed artifact should select an immutable contributor image digest
  or `sha-<commit>` tag.

- [ ] **Step 4: Run the contract to verify it passes**

  Run: `bash tests/image-contract.sh`

  Expected: exit status 0.

- [ ] **Step 5: Commit**

  ```bash
  git add README.md tests/image-contract.sh
  git commit -m "docs: explain Goose canary snapshots"
  ```

### Task 3: Build and validate the local testing image

**Files:**
- Modify: no source files
- Test: `tests/image-contract.sh`, `tests/just-onboarding.sh`

**Interfaces:**
- Consumes: the provenance-verified Containerfile from Task 1.
- Produces: `localhost/donate-clanker:goose-canary` for local
  `DONATE_CLANKER_CONTRIBUTOR_IMAGE` testing.

- [ ] **Step 1: Run source-level validation**

  Run:

  ```bash
  bash tests/image-contract.sh &&
    bash tests/just-onboarding.sh &&
    git diff --check &&
    just --justfile just/61-donate-clanker.just --list
  ```

  Expected: each command exits 0.

- [ ] **Step 2: Build the local testing image**

  Run:

  ```bash
  podman build -f image/Containerfile -t localhost/donate-clanker:goose-canary .
  ```

  Expected: the build finishes successfully after provenance verification and
  the final image contains `/usr/local/bin/goose`.

- [ ] **Step 3: Verify the built Goose version**

  Run:

  ```bash
  podman run --rm --entrypoint /usr/local/bin/goose \
    localhost/donate-clanker:goose-canary --version
  ```

  Expected: Goose prints a canary version.

- [ ] **Step 4: Commit**

  ```bash
  git status --short
  ```

  Expected: only pre-existing unrelated worktree changes remain.
