---
name: donate-clanker-release
description: Use when checking donate-clanker release health or changing VM and compatibility-image release documentation.
metadata:
  context7-sources:
    - /websites/github_en_actions
---

# Donate Clanker Release

## When to Use

Use this when checking release tags and workflow runs, changing
`.github/workflows/publish-vm.yml` or
`.github/workflows/publish-compat-image.yml`, or documenting artifact
ownership and release behavior.

## When NOT to Use

Do not use this for ordinary launcher, worker, or container changes that do
not alter release ownership or release validation. Use `ci-tooling.md` for
general GitHub Actions workflow maintenance.

## Core Process

1. Check the repository tags, GitHub Release objects, and recent workflow runs.
2. Treat the compatibility image and the VM artifact contract as separate
   release surfaces.
3. Remember that `publish-vm.yml` verifies externally published immutable OCI
   artifacts; it does not build or publish the runner, guest, or raw disk.
4. Verify that the external FSDK release contains the exact
   architecture-specific `.raw` file and `.sha256` sidecar expected by the
   launcher before treating automatic local download as available.
5. Keep mutable tags, missing signatures, missing attestations, and missing
   architecture manifests release-blocking.
6. Keep release status out of permanent docs; document the durable workflow and
   ownership boundaries instead.

## Common Rationalizations

- "The VM workflow is named publish, so it publishes artifacts."  
  Read its steps: it only validates references supplied through repository
  variables.
- "The latest tag is the current product release."  
  Check both GitHub Release objects and the tag-triggered workflow results.
- "A raw disk download is good enough without a checksum."  
  The launcher and artifact contract must fail closed on unverifiable inputs.
- "The FSDK release exists, so the raw VM asset exists."  
  Check the release asset list and checksum sidecar explicitly; container image
  publication does not imply raw-disk publication.

## Red Flags

- A version tag is described as a GitHub Release when no release object exists.
- The VM validation workflow is described as an artifact publisher.
- Documentation promises a floating image tag or an unverified raw disk.
- Runner or guest references omit an immutable digest, architecture coverage,
  signature, SBOM, or provenance.

## Verification

- [ ] `gh release list --repo projectbluefin/donate-clanker` was checked.
- [ ] Recent `validate` and release workflow runs were checked.
- [ ] `vm/README.md` matches `vm/manifest.json` and
  `scripts/verify-vm-artifacts.sh`.
- [ ] `git diff --check` passes.

## Sources

GitHub Actions workflow triggers, environment variables, and multi-line `run`
steps were checked against `/websites/github_en_actions`.
