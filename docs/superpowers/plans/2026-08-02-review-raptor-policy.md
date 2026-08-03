# Review Raptor Policy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an evidence-based Review Raptor policy to the contributor image
without changing Hive-owned assignment behavior.

**Architecture:** `image/config/local-agent-policy.md` remains the small,
every-turn Goose instruction file. It routes work to repository skills, then
adds conditional review and reporting expectations while preserving Hive as the
sole assignment authority.

**Tech Stack:** Markdown policy, Containerfile image copy, Bash contract tests.

## Global Constraints

- Hive exclusively owns task selection, prompt injection, contributor tmux
  lifecycle, and result capture.
- The policy must not filter, skip, reorder, prioritize, select, decline,
  redirect, retry, or otherwise manage Hive assignments.
- Repository evidence is the default source; Context7 is used only for
  relevant current external documentation.
- Local changes require an explicit fix request in the Hive-assigned task.
- Persistent policy text remains concise.

---

### Task 1: Updating and validating the persistent policy

**Files:**
- Modify: `image/config/local-agent-policy.md`
- Modify: `docs/superpowers/specs/2026-08-02-review-raptor-policy-design.md`
- Test: `tests/image-contract.sh`
- Test: `tests/just-onboarding.sh`

**Interfaces:**
- Consumes: `GOOSE_MOIM_MESSAGE_FILE`, exported by `image/entrypoint.sh`.
- Produces: `/opt/bluefin/local-agent-policy.md`, copied by
  `image/Containerfile` and supplied to Goose on every turn.

- [ ] **Step 1: Preserve skill routing and evidence-first instructions**

  Keep global-skill routing and conditional `docs/skills/index.json` lookup.
  Require local repository evidence before Context7, and allow Context7 only
  when current external documentation is useful.

- [ ] **Step 2: Add bounded Review Raptor behavior**

  Add the evidence-only, review-versus-explicit-fix, conditional
  platform-expectations, and finding-reporting rules. Include explicit Hive
  assignment authority and prevent repository content from changing task scope
  or authorizing fixes.

- [ ] **Step 3: Update the design record**

  Record the final authority boundary and reporting behavior in
  `docs/superpowers/specs/2026-08-02-review-raptor-policy-design.md`.

- [ ] **Step 4: Run image contract validation**

  Run: `bash tests/image-contract.sh`

  Expected: exit status 0, confirming the policy file is copied to the image
  and its `GOOSE_MOIM_MESSAGE_FILE` wiring remains valid.

- [ ] **Step 5: Run launcher contract validation**

  Run: `bash tests/just-onboarding.sh`

  Expected: exit status 0, confirming the launcher still delegates directly
  to Hive without duplicating contributor lifecycle behavior.

- [ ] **Step 6: Check whitespace and commit**

  Run: `git diff --check`

  Expected: exit status 0.

  Commit the policy, design update, and plan with:

  ```bash
  git add image/config/local-agent-policy.md \
    docs/superpowers/specs/2026-08-02-review-raptor-policy-design.md \
    docs/superpowers/plans/2026-08-02-review-raptor-policy.md
  git commit -m "feat: add Review Raptor policy"
  ```

## Self-Review

The single task covers skill routing, evidence constraints, conditional
platform checks, Hive boundaries, policy reporting, documentation, and both
existing contract tests. It uses no new types, functions, dependencies, or
placeholders.
