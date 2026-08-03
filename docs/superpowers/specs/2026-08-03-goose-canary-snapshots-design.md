# Goose Canary Snapshots Design

## Goal

Run the contributor image on Goose's current upstream `canary` snapshot rather
than the stale `v2.0.0-rc-04-27-0` tag. Upstream rebuilds the `canary` GitHub
release from every push to `main`, which is the only actively maintained
future-facing Goose line.

## Scope

The image-only upgrade changes the Goose download source, integrity validation,
contract test, and related user documentation. It does not change launcher
recipes, Hive, task selection, the Copilot-only provider policy, or Goose
configuration.

## Design

The Containerfile will identify the channel as `canary`, not a semantic
`GOOSE_VERSION`. At image build time it will:

1. Download the upstream canary checksum manifest.
2. Select the architecture-specific Goose archive checksum.
3. Download the matching archive from the same `canary` release.
4. Verify the archive against the selected checksum before extraction.

This preserves an integrity check while deliberately accepting a mutable
upstream release: each new local or CI image build follows the latest snapshot
available at that time. The existing binary smoke checks remain the runtime
compatibility gate.

The Containerfile must fail clearly if the manifest does not contain the
expected x86_64 or aarch64 archive. It must also fail if the archive does not
match its manifest checksum. There is no fallback to the old 1.x archive or a
partial unverified install.

## User Documentation

The README will say that contributor images follow Goose's upstream canary
snapshot, explain that builds are not byte-reproducible across time, and point
users needing a fixed artifact to a published image digest or immutable
`sha-<commit>` image tag.

## Tests

`tests/image-contract.sh` will assert the canary source and checksum-manifest
verification contract while retaining its existing checks for a pinned base
image, Hive compatibility, controlled Goose configuration, and the `goose run
--help` smoke check.

Targeted contract validation will run after the change, followed by the
repository's documented launcher test and formatting/pre-commit checks.
