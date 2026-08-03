# Local `ujust review` Routing Design

## Goal

Make `ujust review` execute the `review` recipe from the working checkout at
`$HOME/src/review/justfile`, so it always uses the code on disk rather than a
downloaded or cached launcher recipe.

## Scope

Update the existing local `ujust` shim to route these commands directly to
the checkout:

| `ujust` command | Invoked recipe |
| --- | --- |
| `review` | `just review` |
| `review-container` | `just review-container` |
| `review-doctor` | `just review-doctor` |

Arguments after the command are passed through unchanged. All other `ujust`
commands continue to delegate to the system `ujust`.

## Design

The shim will define the checkout justfile path as
`$HOME/src/review/justfile`. For each supported review command, it will use
`exec /usr/bin/just --justfile "$review_justfile" <recipe> "$@"`.

This makes the shell process become `just`, preserving the foreground-only
launcher behavior and signal handling. It performs no network refresh and
does not retain a copied recipe. If the checkout or its justfile is absent,
`just` reports the explicit local path failure; the shim must not silently
fall back to an installed, remote, or cached recipe.

The obsolete `donate-clanker` routes and their remote-refresh helper will be
removed. They target a retired launcher name and would otherwise leave a
second route to a stale launcher artifact.

## Error Handling

The shim uses its existing strict shell mode. It does not catch or transform
errors from `just`; missing files and recipe failures propagate their original
nonzero statuses and diagnostic output.

## Validation

1. Inspect the shim to confirm it contains only direct `review` routes and a
   default system-`ujust` delegation.
2. Run `ujust review-doctor` and confirm its output comes from the checkout
   recipe without starting a VM or container.
3. Confirm an unrelated `ujust` command still reaches `/usr/bin/ujust`.
