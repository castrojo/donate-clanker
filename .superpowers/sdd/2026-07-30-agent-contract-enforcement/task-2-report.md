# Task 2 Report: Native contributor contract enforcement

## What I changed
- Added failing runner-first tests for the new contract seam:
  - `TestGooseRunFailsBeforeCommandWhenContractDocumentMissing`
  - `TestGooseRunFailsBeforeCommandWhenContractDocumentEmpty`
  - `TestGooseRunInjectsPolicyContractDocumentsAndAssignmentInOrder`
- Extended `runner.PrepareTaskPrompt` to assemble prompt sections in this order:
  1. local execution policy (when present)
  2. rendered agent contract section
  3. verbatim Hive assignment heading and body
- Added `Contract contract.Manifest` to `runner.Goose`.
- Updated `Goose.Run` to load required contract documents from the task workspace before staging Goose config or invoking the command runner, and to fail closed with a wrapped `ExecutionError` when loading fails.
- Loaded the bundled contract manifest in `cmd/contributor/run` and passed it into the native Goose handler so startup fails before Hive connection if the bundled manifest is malformed.
- Updated contributor and runner fixtures to use valid test manifests plus required workspace documents.

## Validation run
- Added the new tests first, then confirmed the expected pre-implementation failure with:
  - `go test ./internal/runner`
- After implementation, ran:
  - `gofmt -w cmd/contributor/main.go internal/runner/goose.go cmd/contributor/main_test.go internal/runner/goose_test.go`
  - `go test ./cmd/contributor ./internal/runner`
  - `git diff --check -- cmd/contributor/main.go cmd/contributor/main_test.go internal/runner/goose.go internal/runner/goose_test.go`

## Self-review
- Contract load failures now happen before Goose config staging and before any command runner invocation.
- Prompt assembly preserves the assignment as the final verbatim section and keeps contract content ahead of it even when policy is empty.
- The contributor token refresh path still retries with the refreshed GitHub token while carrying a non-empty contract manifest.
- Errors surface only relative document paths and standard wrapped failure text; document contents are not logged.

## Concerns
- `runner.Goose` still accepts a zero-value `contract.Manifest`; enforcement depends on the contributor wiring and test fixtures supplying a validated manifest.
- The repository still has unrelated untracked `actions/`, `bluefin-lts/`, and `docs/superpowers/plans/2026-07-30-agent-contract-enforcement.md` entries, which I left untouched per instruction.

## Follow-up fix: review findings 1 and 2
- Made `Manifest.LoadDocuments` validate hand-built manifests before any file reads, so zero-value or otherwise unvalidated manifests fail closed before Goose staging or command execution.
- Reused manifest validation for bundled and hand-built manifests, preserving normalized relative document paths and existing prompt ordering behavior.
- Sanitized non-missing document read failures to report only the manifest-relative path plus the OS failure reason, never the workspace absolute path.
- Added coverage for zero-value manifest rejection in `Goose.Run`, direct `LoadDocuments` rejection of absolute/traversal paths on hand-built manifests, and unreadable-file error sanitization.

### Follow-up validation run
- `gofmt -w internal/contract/contract.go internal/contract/contract_test.go internal/runner/goose_test.go`
- `go test ./internal/contract ./internal/runner ./cmd/contributor`
  - `ok   github.com/projectbluefin/donate-clanker/internal/contract 0.003s`
  - `ok   github.com/projectbluefin/donate-clanker/internal/runner 0.004s`
  - `ok   github.com/projectbluefin/donate-clanker/cmd/contributor 0.003s`
- `git diff --check -- internal/contract/contract.go internal/contract/contract_test.go internal/runner/goose_test.go`

### Follow-up self-review
- `Goose.Run` still preserves policy → contract → verbatim assignment ordering because only manifest validation and load-error sanitization changed.
- Hand-built manifests now fail with the same validation rules as bundled manifests, closing the fail-open path called out in review.
- Unreadable document failures expose only manifest-relative paths, matching missing/empty-file behavior and avoiding workspace path leaks.
- Updated `docs/skills/worker-credential-boundary.md` with the new contract-manifest fail-closed and path-sanitization guidance.
- `git diff --check -- internal/contract/contract.go internal/contract/contract_test.go internal/runner/goose_test.go docs/skills/worker-credential-boundary.md .superpowers/sdd/2026-07-30-agent-contract-enforcement/task-2-report.md`
