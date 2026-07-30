# Agent Contract Enforcement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fail closed when repository agent-contract documents are missing, inject their contents and factory self-improvement rules into every native Goose task, and remove unused launcher surfaces.

**Architecture:** A bundled `internal/contract` package will parse a JSON manifest, validate its required repository-relative files, load those files from the current workspace, and render a prompt section. `runner.Goose` owns per-task contract loading and prompt assembly; `cmd/contributor` supplies the bundled manifest at startup. The compatibility image remains unchanged.

**Tech Stack:** Go 1.22, `encoding/json`, `embed`, existing Goose runner tests, existing repository Go/Just validation.

## Global Constraints

- Required documents are `AGENTS.md`, `docs/SKILL.md`, and `docs/skills/skill-improvement.md`.
- Missing, unreadable, empty, absolute, or traversal document paths must fail before Goose starts.
- The prompt order is local policy, agent contract, required documents, then verbatim Hive assignment.
- No network fetch is permitted for factory documentation at task runtime.
- Do not modify the compatibility image's upstream worker protocol.
- Preserve secret redaction and do not include document contents in errors or logs.
- Keep the existing external dependency set unchanged.
- Use the repository checks: `go test ./...`, `git diff --check`, `gofmt -l .`, and `just --justfile just/61-donate-clanker.just --list`.

---

### Task 1: Add and validate the bundled contract manifest

**Files:**
- Create: `image/config/agent-contract.json`
- Modify: `image/config/embed.go`
- Create: `internal/contract/contract.go`
- Test: `internal/contract/contract_test.go`

**Interfaces:**
- Produces `contract.Manifest`.
- Produces `contract.Load([]byte) (Manifest, error)`.
- Produces `contract.LoadBundled() (Manifest, error)`.
- Produces `Manifest.LoadDocuments(workspace string) ([]Document, error)`.
- Produces `Manifest.PromptSection(documents []Document) string`.

- [ ] **Step 1: Write manifest tests**

Add tests covering:

```go
func TestLoadAcceptsBundledShape(t *testing.T)
func TestLoadRejectsMissingRequiredFields(t *testing.T)
func TestLoadRejectsAbsoluteAndTraversalPaths(t *testing.T)
func TestLoadDocumentsRejectsMissingAndEmptyFiles(t *testing.T)
func TestLoadDocumentsPreservesManifestOrder(t *testing.T)
func TestPromptSectionIncludesRulesAndDocumentHeadings(t *testing.T)
```

Use `t.TempDir()` for workspace fixtures and assert errors identify the
relative document path without including document contents.

- [ ] **Step 2: Run the focused tests and verify they fail**

Run:

```bash
go test ./internal/contract
```

Expected: compilation failure because the package and manifest interfaces do
not exist yet.

- [ ] **Step 3: Add the JSON manifest**

Create `image/config/agent-contract.json` with a version, the three required
document paths, explicit headings, mandatory rules for reading `AGENTS.md`
and `docs/SKILL.md` first, the self-improvement update rule, and the existing
validation command family.

- [ ] **Step 4: Implement manifest parsing and document loading**

Define JSON-backed types with only the fields needed by the manifest. Validate
that:

- the version is supported;
- every required document has a non-empty relative path;
- `filepath.Clean(path)` does not escape `"."`;
- no duplicate document paths exist;
- rules and validation commands are non-empty.

`LoadDocuments` must join each path to the cleaned workspace, read it, reject
whitespace-only content, and return documents in manifest order.

- [ ] **Step 5: Embed and expose the bundled manifest**

Extend `image/config/embed.go` with `//go:embed agent-contract.json` and a
copy-returning `BundledAgentContractJSON() []byte` function. Implement
`LoadBundled` in `internal/contract` using that function.

- [ ] **Step 6: Run the focused tests and commit**

Run:

```bash
gofmt -w image/config/embed.go internal/contract/*.go
go test ./internal/contract
```

Commit:

```bash
git add image/config/agent-contract.json image/config/embed.go internal/contract
git commit -m "feat: add bundled agent contract manifest"
```

### Task 2: Enforce and inject the contract in every Goose task

**Files:**
- Modify: `cmd/contributor/main.go`
- Modify: `internal/runner/goose.go`
- Test: `cmd/contributor/main_test.go`
- Test: `internal/runner/goose_test.go`

**Interfaces:**
- `runner.Goose` gains `Contract contract.Manifest`.
- `runner.PrepareTaskPrompt(policy string, contractSection string, assignment string) string`.
- `Goose.Run` loads `g.Contract.LoadDocuments(task.Workspace)` before staging config or invoking the command runner.

- [ ] **Step 1: Add failing runner tests**

Add tests that assert:

```go
func TestGooseRunFailsBeforeCommandWhenContractDocumentMissing(t *testing.T)
func TestGooseRunFailsBeforeCommandWhenContractDocumentEmpty(t *testing.T)
func TestGooseRunInjectsPolicyContractDocumentsAndAssignmentInOrder(t *testing.T)
```

Use a fake command runner and assert its call count remains zero for contract
load failures. For success, assert the final `-t` argument contains the
policy heading, contract checklist, each labeled document, and the original
assignment heading in that exact order.

- [ ] **Step 2: Run the focused runner tests and verify failure**

Run:

```bash
go test ./internal/runner
```

Expected: compile or assertion failures because `Goose` has no contract field
and the prompt function has no contract section.

- [ ] **Step 3: Add contract loading to the runner**

Import `internal/contract`, add the manifest field to `Goose`, and make
`Goose.Run` call `g.Contract.LoadDocuments(filepath.Clean(task.Workspace))`
before `stageBundledConfig`. Return a wrapped execution error for contract
load failures without logging document content.

- [ ] **Step 4: Extend prompt assembly**

Render the loaded documents with `Manifest.PromptSection`, then update
`PrepareTaskPrompt` to emit:

```text
Local execution policy...

Agent contract...

Required repository document: AGENTS.md
...

Hive assignment (verbatim):
...
```

If the policy is empty, the contract must still be present. The assignment
must remain byte-for-byte unchanged.

- [ ] **Step 5: Load the bundled manifest in the contributor**

In `cmd/contributor.run`, call `contract.LoadBundled()` alongside the bundled
Goose config and policy. Pass the result into `runner.Goose{Contract: manifest}`.
A malformed bundled manifest must terminate startup before connecting to Hive.

- [ ] **Step 6: Update existing Goose and contributor fixtures**

Update every existing `runner.Goose` fixture to use a small valid test
manifest and create the required files in its workspace fixture. Update
contributor tests to verify the worker still refreshes tokens and that its
Goose handler carries the contract.

- [ ] **Step 7: Run focused tests and commit**

Run:

```bash
gofmt -w cmd/contributor/main.go internal/runner/goose.go cmd/contributor/main_test.go internal/runner/goose_test.go
go test ./cmd/contributor ./internal/runner
```

Commit:

```bash
git add cmd/contributor/main.go internal/runner/goose.go cmd/contributor/main_test.go internal/runner/goose_test.go
git commit -m "feat: enforce repository agent contract before Goose"
```

### Task 3: Remove unused launcher surfaces

**Files:**
- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/docker.go`
- Modify: `internal/engine/podman.go`
- Modify: `internal/config/options.go`
- Modify: `internal/config/mounts.go`
- Modify: `internal/pod/lifecycle.go`
- Modify: `internal/app/app_test.go`
- Modify: `internal/engine/engine_test.go`
- Modify: `internal/pod/lifecycle_test.go`
- Modify: `internal/config/mounts_test.go`

**Interfaces:**
- `engine.Engine` no longer declares `Logs`.
- `config.Options` no longer declares `GitHubConfigDir` or `ModelContainerBaseURL`.
- `config.Mount` no longer declares `SELinuxRelabel`.

- [ ] **Step 1: Confirm the dead surfaces remain unused**

Run:

```bash
rg -n 'Logs\\(|GitHubConfigDir|ModelContainerBaseURL|SELinuxRelabel' cmd internal
```

Expected: only declarations, test fixtures, and the dead implementations
listed above are present; no runtime call site depends on them.

- [ ] **Step 2: Remove the dead API members and implementations**

Delete `Engine.Logs` plus Docker/Podman methods and test stubs. Remove the
unused option fields, flag, environment default, test fixture assignments,
and the unused mount field and literals. Do not alter actual GitHub token
mounting or worker environment behavior.

- [ ] **Step 3: Run package tests**

Run:

```bash
gofmt -w internal/engine internal/config internal/pod internal/app
go test ./internal/engine ./internal/config ./internal/pod ./internal/app
```

Expected: PASS with no references to the removed surfaces.

- [ ] **Step 4: Commit the simplification**

```bash
git add internal/engine internal/config internal/pod internal/app
git commit -m "refactor: remove unused launcher surfaces"
```

### Task 4: Validate the contract and document the durable workflow

**Files:**
- Modify: `.github/workflows/validate.yml`
- Modify: `docs/skills/worker-credential-boundary.md`
- Modify: `docs/skills/skill-improvement.md`
- Test: `image/image_test.go`
- Test: `internal/config/context7_test.go`

**Interfaces:**
- CI validates the manifest's required paths and required factory clauses.
- The worker skill documents the fail-closed contract injection seam.

- [ ] **Step 1: Add static manifest contract checks**

Extend validation to parse `image/config/agent-contract.json` with `jq` and
assert that the required document paths, mandatory self-improvement clause,
and validation commands are present. Keep this check offline and independent
of Podman, Goose, or Hive credentials.

- [ ] **Step 2: Add image/config fixture assertions**

Add a focused test or static assertion that the bundled manifest is embedded
and non-empty, matching the existing image config tests.

- [ ] **Step 3: Document enforcement**

Update `docs/skills/worker-credential-boundary.md` with the rule that native
tasks fail before Goose unless the repository agent documents are loaded and
injected. Update `docs/skills/skill-improvement.md` with the runtime
verification condition: durable discoveries must update the nearest skill
document in the same change, and the contract manifest must remain aligned
with the required entry points.

- [ ] **Step 4: Run repository validation**

Run:

```bash
go test ./...
git diff --check
test -z "$(gofmt -l .)"
just --justfile just/61-donate-clanker.just --list
```

- [ ] **Step 5: Review the final diff and commit**

Run:

```bash
git --no-pager diff --stat HEAD~3
git --no-pager diff --check
```

Confirm no files under the unrelated untracked `actions/` or `bluefin-lts/`
trees were staged, then commit:

```bash
git add .github/workflows/validate.yml docs/skills/worker-credential-boundary.md docs/skills/skill-improvement.md image/image_test.go internal/config/context7_test.go
git commit -m "test: validate enforced agent contract"
```

## Final Verification

After all task commits, run the complete existing checks once more:

```bash
go test ./...
git diff --check
test -z "$(gofmt -l .)"
just --justfile just/61-donate-clanker.just --list
```

Verify the final prompt behavior with the runner tests: a missing required
document produces no Goose command invocation, while a valid workspace places
the contract documents before the verbatim Hive assignment.
