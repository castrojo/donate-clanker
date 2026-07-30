# Opportunistic Context7 for Local Goose Agents

## Goal

Improve local Goose agents with current library and API documentation when it
is useful, without making network documentation a prerequisite for running a
Hive contributor task.

## MVP boundary

The FSDK/Goose container owns the integration. Hive is not changed, and
donate-clanker does not duplicate Hive's knowledge system. Context7 is exposed
to Goose as an MCP server using its free endpoint.

The default Goose policy tells the agent to:

- inspect the task and repository first;
- use Context7 when an external library, framework, or API question would
  benefit from current documentation;
- continue with repository-local documentation and tools when Context7 is
  unnecessary.

This is opportunistic, not a mandatory preflight. The worker must not block a
task because the agent did not call Context7.

## Data flow

1. Hive assigns a task to the contributor worker.
2. Goose inspects the repository and task.
3. Goose optionally calls Context7 for relevant documentation.
4. Goose uses the returned documentation with local repository evidence.
5. Goose edits the workspace and runs the repository's existing checks.

Context7 remains available for follow-up lookups during the same task.

## Failure behavior

Context7 failures do not fail task startup or task execution. If Goose calls
Context7 and the request fails, the container emits one concise warning and
continues. If Goose never calls Context7, no warning is emitted.

The MVP does not add an API key requirement, automatic documentation ingestion,
remote web fallback, local vector search, or a second retrieval service.

## Existing behavior to preserve

- Hive owns assignment, authentication, liveness, task identity, and
  completion behavior.
- Goose remains headless and uses the existing local model endpoint.
- Repository-local documentation and existing tools remain authoritative when
  external documentation is unavailable.
- Credentials and raw tool/MCP payloads are not logged.

## Quality settings

Use the existing bounded local-inference defaults, including approximately
32k context unless a profile overrides it. Preserve command failures and
useful diffs rather than silently swallowing or excessively dumping output.
Do not add a new orchestration loop in this MVP.

Extended thinking is disabled for every local model profile. The RamaLama
server is the source of truth and must start with its non-thinking setting;
Goose also receives `GOOSE_THINKING_EFFORT=off` as a secondary preference.
For Qwen chat templates, pass `enable_thinking:false` when the runtime
supports template kwargs. Qwen3-Coder is already non-thinking-only, but keeps
the same Goose setting for consistent behavior.

## Validation

Add the smallest checks that prove:

- the container's Goose configuration exposes Context7;
- the default policy describes opportunistic use;
- an unavailable Context7 endpoint does not prevent a task from starting;
- a successful lookup remains available for follow-up use.

## Deferred work

The following require evidence from real contributor tasks or coordination with
the Hive team:

- integrating Context7 into Hive's knowledge primer;
- hard or adaptive preflight enforcement;
- caching Context7 results;
- local semantic documentation search;
- model-specific retrieval and execution orchestration.
