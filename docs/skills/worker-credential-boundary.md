---
name: donate-clanker-worker-credential-boundary
description: Use when changing donate-clanker worker auth, Goose task launching, Hive credential flow, workspace mounts, or prompt construction around policy and assignment text.
---

# Donate Clanker Worker Credential Boundary

## When to Use

Use this when you touch:

- `internal/app`, `cmd/contributor`, or `internal/runner` auth flow
- worker/container mount wiring
- Hive credential loading or scrubbing
- Goose prompt construction or token injection
- redaction logic for worker output

## When NOT to Use

Do not use this for:

- helper-only RamaLama runtime changes that do not affect worker auth or mounts
- pure UI/docs edits unrelated to worker credentials
- Hive protocol changes that do not alter what the Goose process can see

## Core Process

1. Keep the worker mount set minimal: mount only `/workspace` into Goose; reserve `/cache/ramalama` for the helper container.
2. Validate and load Hive credentials on the host side before worker start.
3. Pass only the minimum worker env needed for the Hive client to reconnect: `HIVE_REGISTRATION_TOKEN`, `HIVE_WS_URL`, `AGENT_BACKEND`, model/provider settings, and `WORKSPACE`.
4. Immediately scrub host auth env from the contributor process after loading credentials: unset Hive config/token paths plus `GH_TOKEN`, `GITHUB_TOKEN`, `GH_CONFIG_DIR`, and `GITHUB_CONFIG_DIR`.
5. Give Goose only the active assignment's GitHub token via `GH_TOKEN`/`GITHUB_TOKEN`; never pass host GitHub config directories into Goose.
6. Keep local policy and Hive assignment as separate prompt sections. Policy must precede assignment with explicit headings, and the assignment body must remain verbatim under `Hive assignment (verbatim):`.
7. Preserve redaction on both command output and surfaced errors. Redact `*_TOKEN=...`, YAML-style secret keys, and the full `Authorization:` line value.
8. Run focused tests for `cmd/contributor`, `internal/app`, `internal/config`, `internal/runner`, and `internal/hive`, then run `go test ./...`.

## Common Rationalizations

- “Mounting the GitHub config dir is easier.”  
  It also hands untrusted assignment text long-lived host auth state. Use the task token instead.
- “The worker can keep Hive env around after startup.”  
  It only needs the parsed credentials in memory; scrubbing the process env shrinks the blast radius.
- “Policy and assignment can just be concatenated.”  
  Without explicit boundaries, untrusted assignment text can blur local policy and task instructions.
- “Redacting `Authorization:` up to the first space is enough.”  
  Bearer-style values often include prefixes; redact the full line value.

## Red Flags

- Worker mounts include `/config/hive` or `/config/github`
- Goose env contains `HIVE_CONFIG_DIR`, `GITHUB_CONFIG_DIR`, or `GH_CONFIG_DIR`
- Host `GH_TOKEN` can flow into Goose without coming from the assignment
- Prompt text lacks separate policy/assignment headings
- Tests only cover success paths and not env/mount exposure or redaction

## Verification

- [ ] `ResolveMounts` returns only workspace and cache mounts
- [ ] Worker env contains Hive connection values but not host auth paths
- [ ] `clearWorkerCredentialEnvironment` removes host Hive/GitHub auth env
- [ ] Goose receives only assignment-scoped GitHub token values
- [ ] Prompt output preserves verbatim assignment text under a dedicated heading
- [ ] Redaction covers `GH_TOKEN`, `GITHUB_TOKEN`, and full `Authorization:` lines
- [ ] `go test ./cmd/contributor ./internal/app ./internal/config ./internal/runner ./internal/hive` passes
- [ ] `go test ./...` passes
