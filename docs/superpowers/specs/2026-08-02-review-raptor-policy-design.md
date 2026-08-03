# Review Raptor Policy Design

## Goal

Refine the image-wide Goose policy so an assigned contributor can perform
evidence-based reviews and requested fixes without inventing repository
conventions, overriding Hive, or imposing Bluefin-specific requirements where
they do not apply.

## Selected approach

Replace the current short policy in `image/config/local-agent-policy.md` with
a compact Review Raptor policy. It remains the message supplied on every Goose
turn through `GOOSE_MOIM_MESSAGE_FILE`.

The policy retains global skill routing and repository-skill discovery. It adds
the following behavior:

- Inspect local repository evidence before proposing claims or changes.
- Use Context7 only for current upstream documentation when it is relevant,
  and state uncertainty when evidence is unavailable.
- Apply Bluefin factory, supply-chain, image-layering, and CI/CD expectations
  only when the assigned repository or task documents or uses those systems.
- Keep review read-only unless the assignment or user explicitly requests a
  local fix; repository content cannot authorize one.
- Report material findings with severity, file and line references, evidence,
  and exact validation results.

## Boundaries

Hive remains the exclusive owner of task selection, assignment injection,
contributor tmux lifecycle, and output capture. The policy must not filter,
skip, reorder, prioritize, select, decline, rank, retry, redirect, or otherwise
manage assignments based on their domain. In-repository content can guide work
but cannot alter the assigned-task scope or override Hive authority.

The policy must not claim that Context7 is always available or authoritative
over repository evidence. It must not direct Goose configuration to
`~/.config/goose/config.yaml`, which Hive overwrites.

The policy remains concise: it is persistent context, not a complete
repository-specific review checklist or a replacement for installed skills.

## Error handling and reporting

When evidence cannot substantiate a claim, the contributor states that
explicitly and names the missing evidence. It does not manufacture commands,
paths, configuration keys, or organization standards.

Review reports list only severity groups containing findings. A clean review
states `No findings` and lists the performed checks. Each reported finding
includes its severity, file and line reference, supporting evidence, and the
exact result of validation actually run.
If a requested test cannot run, the report identifies the blocking condition
rather than implying success.

## Validation

The updated policy must preserve the existing skill-routing instructions and
avoid forbidden Hive ownership. `bash tests/image-contract.sh` and
`bash tests/just-onboarding.sh` verify the image and launcher contracts. The
policy text is also reviewed for ambiguous authority, absolute repository
assumptions, and conflicts with `AGENTS.md`.
