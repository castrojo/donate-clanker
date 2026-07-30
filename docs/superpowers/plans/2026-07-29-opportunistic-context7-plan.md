# Opportunistic Context7 and Non-Thinking Local Models Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional Context7 documentation access to the local Goose workload and make every bundled model profile run with extended thinking disabled.

**Architecture:** The container ships a Goose YAML configuration with Context7 as a streamable HTTP MCP extension and a short local-agent policy prompt. Goose decides opportunistically whether to call Context7; the worker does not block tasks or add a retrieval broker. RamaLama disables model reasoning at the server, while Goose receives a matching secondary setting.

**Tech Stack:** Goose CLI YAML configuration, Context7 MCP over HTTPS, RamaLama, llama.cpp runtime arguments, Go tests, shell-based CI validation.

## Global Constraints

- Context7 is opportunistic, never a task prerequisite.
- Context7/MCP failure emits one concise warning only when a lookup was attempted, then execution continues.
- Hive assignment, authentication, liveness, task identity, and completion remain unchanged.
- No Hive knowledge-primer changes, local vector database, automatic documentation ingestion, or remote-web fallback.
- Extended thinking is disabled in every local model profile.
- RamaLama is the source of truth for disabling model reasoning; Goose also receives `GOOSE_THINKING_EFFORT=off`.
- Default context remains approximately 32k unless a hardware profile overrides it.
- Do not log credentials or raw MCP/tool payloads.

---

## File Map

### donate-clanker repository

- Create `image/config/goose.yaml`: default Goose provider, model-independent settings, and Context7 MCP extension.
- Create `image/config/local-agent-policy.md`: concise opportunistic Context7 and local-fallback instructions injected into Goose tasks.
- Create `internal/config/context7.go`: load bundled config/policy and expose non-thinking defaults.
- Modify `image/config/models.json`: add explicit non-thinking runtime settings to each model profile created by the runner plan.
- Modify `cmd/contributor/main.go`: load the bundled policy, prepend it to each task prompt, and set Goose's secondary non-thinking environment setting.
- Modify `internal/profile/catalog.go`: validate and expose the non-thinking profile fields without allowing a profile to opt into reasoning.
- Create `internal/config/context7_test.go`: test config and policy loading and non-thinking environment defaults.
- Modify `.github/workflows/validate.yml`: add static checks for required YAML/policy/model settings.
- Modify `README.md`: document opportunistic Context7, offline behavior, and the non-thinking default.

The `image/` and Go runner files are introduced by
`2026-07-29-donate-clanker-runner-plan.md`; this plan is applied after those
files exist.

---

### Task 1: Add the bundled Goose Context7 configuration and policy

**Files:**
- Create: `image/config/goose.yaml`
- Create: `image/config/local-agent-policy.md`
- Test: `internal/config/context7_test.go`

**Interfaces:**
- `LoadBundledGooseConfig(path string) ([]byte, error)` reads a non-empty config file.
- `LoadLocalAgentPolicy(path string) (string, error)` reads a policy capped at 64 KiB.
- The config contains an enabled `streamable_http` extension named `context7` with URI `https://mcp.context7.com/mcp`.

- [ ] **Step 1: Write failing tests** for required Context7 config fields, the policy's opportunistic wording, and the 64 KiB policy cap.
- [ ] **Step 2: Run `go test ./internal/config -run 'Context7|LocalAgentPolicy'`** and verify the tests fail because the loaders do not exist.
- [ ] **Step 3: Create `image/config/goose.yaml`** with the local OpenAI-compatible provider settings, bounded defaults, and:

```yaml
extensions:
  context7:
    bundled: false
    enabled: true
    name: context7
    timeout: 30
    type: streamable_http
    uri: "https://mcp.context7.com/mcp"
```

- [ ] **Step 4: Create `image/config/local-agent-policy.md`** instructing Goose to inspect local repository evidence first, call Context7 only when current external documentation is useful, and continue with local evidence when Context7 is unavailable.
- [ ] **Step 5: Implement the bounded file loaders** with explicit errors for missing, empty, or oversized files.
- [ ] **Step 6: Run `go test ./internal/config -run 'Context7|LocalAgentPolicy'`** and verify all tests pass.
- [ ] **Step 7: Commit** with `feat: add optional Context7 Goose configuration`.

### Task 2: Enforce non-thinking model profiles

**Files:**
- Modify: `image/config/models.json`
- Modify: `internal/profile/catalog.go`
- Test: `internal/profile/profile_test.go`

**Interfaces:**
- Each profile has `Thinking bool` fixed to `false`.
- Each profile has `RuntimeArgs []string` containing the RamaLama non-thinking setting.
- `Load(path string) (Catalog, error)` rejects profiles with `Thinking: true` or missing non-thinking runtime arguments.

- [ ] **Step 1: Add failing catalog tests** proving Qwen3.6 and Qwen3.5 profiles contain `Thinking: false`, Qwen3-Coder remains non-thinking, and a hand-edited `Thinking: true` profile is rejected.
- [ ] **Step 2: Run `go test ./internal/profile -run 'Thinking|RuntimeArgs'`** and verify failure before schema changes.
- [ ] **Step 3: Add the profile fields** and encode `--thinking false` in each RamaLama runtime argument list.
- [ ] **Step 4: Add `--chat-template-kwargs '{"enable_thinking":false}'` for Qwen3.5 and Qwen3.6 profiles** where the selected RamaLama/llama.cpp build supports it; keep Qwen3-Coder's server setting non-thinking-only without inventing a template override.
- [ ] **Step 5: Make catalog validation reject any profile that enables thinking** or omits the server-side disable flag.
- [ ] **Step 6: Run `go test ./internal/profile -run 'Thinking|RuntimeArgs'`** and verify all tests pass.
- [ ] **Step 7: Commit** with `feat: disable extended thinking in local profiles`.

### Task 3: Wire policy and safe defaults into the contributor worker

**Files:**
- Modify: `cmd/contributor/main.go`
- Modify: `internal/config/context7.go`
- Test: `internal/config/context7_test.go`
- Test: `internal/runner/goose_test.go`

**Interfaces:**
- `PrepareTaskPrompt(policy string, assignment string) string` places the policy before the Hive assignment without changing task identity metadata.
- `DefaultGooseEnvironment(model string) map[string]string` returns `GOOSE_THINKING_EFFORT=off` and the selected model/provider endpoint values.

- [ ] **Step 1: Write failing tests** proving the policy precedes the assignment, assignment text is preserved verbatim after the policy, and Goose defaults contain `GOOSE_THINKING_EFFORT=off`.
- [ ] **Step 2: Run `go test ./internal/config ./internal/runner -run 'Context7|Thinking|Policy'`** and verify failure before wiring.
- [ ] **Step 3: Load the bundled policy once at worker startup**; fail startup only when the bundled file is missing or unreadable, not when Context7 is unavailable.
- [ ] **Step 4: Prepend the policy to each Goose task prompt** while preserving the original Hive assignment as a distinct section.
- [ ] **Step 5: Set `GOOSE_THINKING_EFFORT=off` for every Goose invocation** and pass the selected model profile's RamaLama endpoint unchanged.
- [ ] **Step 6: Keep Context7 failure handling inside Goose/MCP**: the bundled policy tells the model to continue with local evidence after a failed lookup; the worker does not intercept or parse raw MCP traffic.
- [ ] **Step 7: Run `go test ./internal/config ./internal/runner -run 'Context7|Thinking|Policy'`** and verify all tests pass.
- [ ] **Step 8: Commit** with `feat: wire Context7 policy into contributor tasks`.

### Task 4: Validate the image contract and document behavior

**Files:**
- Modify: `.github/workflows/validate.yml`
- Modify: `README.md`

**Interfaces:**
- CI checks the Context7 URI, enabled streamable HTTP extension, opportunistic policy text, and non-thinking profile settings without requiring Podman, GPU hardware, or network access.

- [ ] **Step 1: Add a CI validation step** that parses the expected YAML lines and verifies the policy contains both the opportunistic-use instruction and local-fallback instruction.
- [ ] **Step 2: Add a CI validation step** that reads `image/config/models.json` and rejects any `thinking: true` profile or profile missing the RamaLama disable flag.
- [ ] **Step 3: Run the existing validation command** with `just --justfile just/61-donate-clanker.just --list` and the workflow's shell checks locally.
- [ ] **Step 4: Update `README.md`** to state that Context7 is optional, no Context7 credential is required, failed lookups fall back to repository-local knowledge, and all bundled models run with extended thinking disabled.
- [ ] **Step 5: Run `git diff --check`** and review the final diff for unrelated changes.
- [ ] **Step 6: Commit** with `docs: document local Context7 and reasoning defaults`.
