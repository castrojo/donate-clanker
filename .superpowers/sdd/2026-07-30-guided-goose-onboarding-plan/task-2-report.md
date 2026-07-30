# Task 2 Report — onboarding-readiness-gate

## Status
Completed.

## Changes
- Added `require_supported_goose()` to gate supported Goose onboarding in order: GitHub auth, Goose install, Goose config, then existing Hive config.
- Preserved first-run Hive setup by letting a missing `~/.config/hive/contributor.env` fall through to the existing pinned `contribute-setup goose` path.
- Removed Goose provider/model picker state from the supported path, stopped persisting `LAST_GOOSE_*`, and stopped writing `GOOSE_PROVIDER` / `GOOSE_MODEL` into launcher-managed secrets.
- Extended `tests/just-onboarding.sh` to verify single-action failures, redacted invalid-Hive output, and cleanup of stale Goose picker state.

## Test commands and results
- `bash tests/just-onboarding.sh`
  - Exit status: `0`
  - Result: PASS.
- `just --justfile just/61-donate-clanker.just --list`
  - Exit status: `0`
  - Result: Justfile parses.
- `git --no-pager diff --check -- just/61-donate-clanker.just tests/just-onboarding.sh`
  - Exit status: `0`
  - Result: no whitespace / patch formatting issues in task files.
- Focused static check:
  - `grep -Fq 'require_supported_goose()' just/61-donate-clanker.just`
  - `! grep -Fq 'anthropic openai google ollama openrouter' just/61-donate-clanker.just`
  - Result: passed.

## Commit
- `efc652c8107f88c93b75166bab38b5a590a0be2d` — `fix: guide Goose onboarding readiness`

## Concerns
- `just/61-donate-clanker.just` already had unrelated dirty worktree changes (the runsc/runtime work) before this task. I committed only the onboarding hunks and left the unrelated edits unstaged.
- No skill-doc update was needed for this task; the change adjusts launcher behavior and coverage but did not establish a new durable repository workflow.

## Review follow-up
- Excluded `TOOL=goose` from launcher-managed model overrides entirely: the generic `gum` model prompt no longer runs for Goose, and launcher-managed `AGENT_MODEL` state is cleared before persistence or secrets-file regeneration.
- Added small helper guards in the launcher so only `claude`, `copilot`, and `codex` participate in launcher-managed model prompting/persistence; Goose now relies solely on its validated local config.
- Extended `tests/just-onboarding.sh` with deterministic fake `gum`/`claude` fixtures plus a test-only interactive override so the interactive path is exercised without a real TTY.
- Added regression coverage proving the legacy interactive model prompt still persists for `TOOL=claude`, while explicit `TOOL=goose` skips `gum` entirely and strips stale `AGENT_MODEL` overrides from `secrets.env`.

### Follow-up validation run
- `bash tests/just-onboarding.sh`
  - Exit status: `0`
  - Result: PASS, including the forced-interactive legacy prompt path and the Goose no-prompt/no-persist regression.
- `just --justfile just/61-donate-clanker.just --list`
  - Exit status: `0`
  - Result: Justfile parses.
- `git diff --check -- just/61-donate-clanker.just tests/just-onboarding.sh`
  - Exit status: `0`
  - Result: no whitespace / patch-formatting issues in the changed launcher files.

### Follow-up self-review
- Goose no longer reaches any launcher-managed `gum` or `AGENT_MODEL` path, even when explicitly selected and even when a caller exports `AGENT_MODEL`.
- Legacy/manual backends still use the remembered model flow because the launcher-model guard only admits `claude`, `copilot`, and `codex`.
- The new interactive test seam is deterministic and repo-local; it does not require `/tmp`, a pseudo-terminal, or live model/network calls.
