---
name: donate-clanker-ci-tooling
description: Use when changing GitHub Actions workflows, CI-only validation steps, or test harnesses for donate-clanker.
metadata:
  context7-sources:
    - /websites/github_en_actions
---

# Donate Clanker CI Tooling

## When to Use

Use this when changing files under `.github/workflows/`, adding or removing
validation steps, or changing CI-only test harness behavior.

## When NOT to Use

Do not use this for release ownership or artifact publication decisions; use
`release.md`. Do not use it for ordinary Go tests or launcher behavior unless
the CI harness or workflow itself changes.

## Core Process

1. Read the workflow and the implementation contract it validates.
2. Remove checks for retired product paths instead of preserving stale
   assertions for compatibility.
3. Keep CI steps explicit, fail-closed, and independent of live credentials
   unless the workflow is intentionally an integration gate.
4. Mirror every new CI assertion with a local command or focused test.
5. Parse workflow YAML and run the repository's existing validation commands
   before handoff.

## Common Rationalizations

- "The old check is harmless."  
  Stale checks block current releases and hide the real contract.
- "The hosted runner can exercise the production VM."  
  Keep KVM, credentials, and external artifact checks explicit; use static
  contract validation for hosted CI.
- "A shell test can fetch the real artifact."  
  CI-only harnesses must use deterministic fixtures and never download large
  release assets unnecessarily.

## Red Flags

- A workflow asserts Lima, host workspace mounts, or another retired runtime.
- A CI harness can contact a real registry or download a release artifact
  while testing control flow.
- A release gate silently skips missing environment variables or signatures.
- Workflow changes have no corresponding local validation command.

## Verification

- [ ] Workflow YAML parses.
- [ ] Retired product paths are not asserted by current CI.
- [ ] CI-only tests use deterministic local fakes.
- [ ] `go test ./...` passes.
- [ ] `git diff --check` passes.
- [ ] `just --justfile just/61-donate-clanker.just --list` passes.

## Sources

GitHub Actions trigger syntax, environment variables, and multi-line `run`
steps were checked against `/websites/github_en_actions`.
