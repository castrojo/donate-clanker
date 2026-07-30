# Agent Contract Enforcement Design

## Goal

Make every contributed task obey the repository and factory agent contract
before Goose starts. The worker must require the repository's agent entry
points, inject their contents into the model prompt, and make the
self-improvement loop explicit and mandatory.

This change also removes the unused configuration and engine surfaces found
by the repository-wide simplification audit.

## Scope

The enforcement applies to the native contributor worker in
`cmd/contributor` and `internal/runner`. The compatibility image continues to
delegate to the upstream Hive contributor runtime and is not changed by this
contract loader.

Required repository documents:

- `AGENTS.md`
- `docs/SKILL.md`
- `docs/skills/skill-improvement.md`

The worker reads these files from the mounted task workspace for every task.
The files must exist, be readable, and contain non-whitespace content.

## Contract manifest

Add `image/config/agent-contract.json` and embed it with the existing image
configuration. The manifest declares:

- required repository-relative document paths;
- the mandatory instruction that `AGENTS.md` and `docs/SKILL.md` are read
  first;
- the mandatory self-improvement rule: durable discoveries update the
  nearest `docs/skills/*.md` file in the same change;
- the required validation command family from the repository contract.

The manifest is data-only and is validated when loaded. Unknown or empty
required paths are rejected. It is not a replacement for the documents; it
defines the fail-closed checklist and which documents are injected.

## Runtime flow

1. `cmd/contributor` loads the bundled manifest at startup alongside Goose
   configuration and the local policy.
2. Before each task, `runner.Goose.Run` validates the task workspace and loads
   each manifest-required document using repository-relative paths.
3. Missing, unreadable, empty, absolute, or traversal paths return a task
   error before the Goose subprocess is started.
4. The prompt is assembled in this order:
   - the local execution policy;
   - the injected `Agent contract` checklist from the manifest;
   - each required document under a clearly labeled heading;
   - the verbatim Hive assignment.
5. The existing assignment remains unchanged and remains the final section.

This makes the contract visible to the agent while preserving the original
task identity and assignment text. Validation is per task so a contributor
cannot reuse a previously valid contract after switching workspaces.

## Error handling

Contract load errors are surfaced as task failures with the offending
relative path. No warning-only or success-shaped fallback is permitted.
Document contents are treated as prompt text and are not logged in worker
errors or summaries.

## Simplification pass

Remove these unused surfaces:

- `Engine.Logs` and its implementations/test stubs;
- `config.Options.GitHubConfigDir` and its flag/environment default;
- `config.Options.ModelContainerBaseURL`;
- `config.Mount.SELinuxRelabel`.

Consolidate the duplicated env-file parser and secret-redaction helper only
if the shared implementation reduces total code without introducing a new
cross-layer dependency.

## Testing

Add focused tests for:

- valid and invalid contract manifests;
- required-document loading and path traversal rejection;
- missing and empty document failures before command execution;
- prompt section ordering and preservation of the verbatim assignment;
- successful contract injection into a Goose command.

Run the repository's existing Go tests, formatting, diff checks, and Justfile
parse check.

## Non-goals

- Changing the compatibility image's upstream worker protocol;
- fetching factory documentation from the network at task runtime;
- trusting prompt wording without file existence and content checks;
- adding a second policy format to `image/config/models.json`.
