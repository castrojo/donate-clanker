# Guided Goose Onboarding Design

## Goal

Make the supported GitHub plus Goose onboarding path deterministic and
actionable without creating or modifying Goose credentials or configuration.

## Scope

The launcher owns readiness detection. Goose continues to own interactive
configuration through `goose configure`.

The supported local inference configuration is:

- `provider: openai`
- a non-empty HTTP(S) `base_url`
- a non-empty `api_key`
- a non-empty `model`

The launcher must not prompt for providers or models that cannot satisfy this
contract.

## Interaction

Before attempting Hive setup, the launcher evaluates prerequisites in order:

1. GitHub CLI authentication for `github.com`.
2. Goose installation.
3. A valid supported Goose configuration.
4. A valid pinned Hive contributor setup.

It stops at the first failure and prints one concise redacted diagnosis and one
exact next action. For example, an invalid Goose configuration directs the user
to run `goose configure` with the supported local OpenAI-compatible provider,
endpoint, key, and model.

`donate-clanker-doctor` remains the full audit. It reports every failed
prerequisite rather than adopting the launcher's first-failure behavior.

Remembered selections remain only where they affect a supported launch. The
guided onboarding path does not offer legacy provider or model prompts.

## Error Handling

Errors must be actionable in attended and noninteractive runs, must not echo
endpoint or credential values, and must not attempt to repair user
configuration. Hive setup must not begin after an earlier prerequisite fails.

## Verification

Focused tests cover missing GitHub authentication, missing Goose, invalid Goose
configuration, invalid Hive setup, and exact next-action output. Existing doctor
coverage continues to verify the complete multi-failure report. A successful
guided launch reaches Hive setup without offering an incompatible provider
choice.
