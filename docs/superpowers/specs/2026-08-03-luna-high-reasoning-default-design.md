# Luna High-Reasoning Default Design

## Goal

Start both review launch paths immediately with GitHub Copilot model
`gpt-5.6-luna` and high reasoning, without a model-selection prompt.

## Design

The launcher default model changes from `gpt-4.1` to `gpt-5.6-luna`. Its
interactive Gum picker and remembered model selection are removed, so
`just review` and `just review-container` use the default unless
`GOOSE_MODEL` is explicitly set in the launch environment.

The contributor image entrypoint exports `GOOSE_THINKING_EFFORT=high` when
the caller has not already set it. Goose applies this configuration to the
model session before Hive injects work. The direct-image fallback model also
changes to `gpt-5.6-luna`.

The VM bootstrap already carries the selected model to the guest; the image
entrypoint supplies the high reasoning default in both VM and container-only
runs. No Hive protocol changes are required.

## Overrides and Errors

`GOOSE_MODEL` and `GOOSE_THINKING_EFFORT` remain explicit per-launch
overrides. Invalid model or reasoning values are left for Goose to report;
the launcher does not silently substitute a different model or effort.

## Validation

Update launcher and image contract assertions to require the new default and
the high-reasoning environment export. Run the targeted launcher and image
contract tests.
