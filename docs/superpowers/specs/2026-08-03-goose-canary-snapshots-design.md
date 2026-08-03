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

1. Download the architecture-specific archive from the upstream `canary`
   release.
2. Verify the archive with GitHub's signed SLSA build provenance, pinning the
   repository and the `canary.yml` signer workflow.
3. Extract only after provenance verification succeeds.

Goose does not publish a checksum manifest for its canary artifacts. GitHub
attestation verification checks the downloaded archive's SHA-256 and verifies
that its provenance was signed by the official release workflow. This preserves
an integrity check while deliberately accepting a mutable upstream release:
each new local or CI image build follows the latest snapshot available at that
time. The existing binary smoke checks remain the runtime compatibility gate.
The GitHub token required by the verifier is a build secret, never a build
argument or image environment variable.

The Containerfile must fail clearly if provenance cannot be verified for the
expected x86_64 or aarch64 archive. There is no fallback to the old 1.x archive
or a partial unverified install.

## User Documentation

The README will say that contributor images follow Goose's upstream canary
snapshot, explain that builds are not byte-reproducible across time, and point
users needing a fixed artifact to a published image digest or immutable
`sha-<commit>` image tag.

## Tests

`tests/image-contract.sh` will assert the canary source and signed-provenance
verification contract while retaining its existing checks for a pinned base
image, Hive compatibility, controlled Goose configuration, and the `goose run
--help` smoke check.

Targeted contract validation will run after the change, followed by the
repository's documented launcher test and formatting/pre-commit checks.
