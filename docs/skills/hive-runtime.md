---
name: hive-runtime
version: "1.2"
last_updated: 2026-08-01
id: hive-runtime
one_line_purpose: Operate inside Hive's tmux, token, and cooldown constraints.
entry_point: docs/skills/hive-runtime.md
category: ci-ops
mcp_compliance_level: partial
optimization_status: draft
status: active
dependencies: []
tags: [hive, tmux, runtime, tokens, cooldown]
description: "Describes Hive's contributor runtime: the tmux session contract, prompt injection, the 15-line output budget, the 55-minute token, the 168-hour cooldown, the agent's GitHub identity, and why selection is Hive's alone. Use when debugging a session."
metadata:
  type: reference
---

# Hive Runtime

## When to Use

Load this when attaching to a running contributor, when a task appears to
hang or produce no result, when reasoning about how long a task may run, or
before writing any code that touches session, prompt, or output handling.

## Core Process

### Hive drives selection; nothing here filters

Hive's `selectTask` is the only thing that chooses work. It reads the hub's
config — the repository pool, the suspend flag, the disabled-repository list,
the title/author/label deny-list and mode pairs — plus the 168-hour per-issue
cooldown and the issues already held by other live connections, then takes
the first admissible issue. There is no ranking and no negotiation: the
contributor accepts what it is handed.

The candidate pool comes exclusively from the repositories listed in the
hub's config; one that is not listed is structurally invisible to the
selector, and in this org admission is expressed hub-side by a queue label.
Both are policy the maintainers set deliberately.

So filtering here would not merely be redundant. It would shadow a lifecycle
Hive already owns, diverge silently from the hub's admission policy the
moment either side changes, and its only visible effect would be hiding work
that was deliberately admitted. Whatever Hive assigns is what the agent works
on — regardless of repository, label, title, author or issue number.

The upstream relay behaves the same way: it accepts the first assignment and
answers `task_failed` only when it already has one. Declining work means
speaking the contributor protocol, which is Hive's, not ours.

### The tmux session contract

Hive's entrypoint creates a tmux session named `contributor` and launches the
agent CLI inside it by keystroke injection. donate-clanker does not create,
name, or manage this session. It only attaches.

Attach with Hive's own documented flow:

```bash
podman exec -it <container> tmux attach -t contributor
# docker exec -it <container> tmux attach -t contributor
```

Detach with the tmux prefix followed by `d`. Detaching leaves the agent
running; the task continues.

### How prompts arrive

Task prompts are injected into the pane the same way the CLI itself is
launched: as keystrokes. There is no API surface, no file drop, and no queue
you can inspect. What you see in the pane is the whole truth of what the
agent was told.

Consequence: if you type into the pane while a prompt is being injected, you
corrupt it. Attach to observe and steer deliberately, not reflexively.

### The 15-line output budget

Hive captures the **last 15 lines** of the pane to report results back. That
is the entire reporting channel.

Practical rules:

- Put the conclusion last. Anything scrolled past line 15 from the bottom is
  invisible to Hive.
- Avoid trailing verbose command output before a report; it evicts the
  summary.
- Do not design flows that assume a full transcript reaches the server.

### The agent's GitHub identity

Hive runs donate-clanker with `HIVE_CONTRIBUTOR_MODE=true`, and the Hive `gh`
wrapper at `/usr/local/bin/gh` injects the hub's App token **only when that is
not `true`** -- "Contributors keep their personal token -- they fork+PR with
their own identity." In contributor mode it injects nothing and blocks every
`gh auth ...` call. So the agent has no identity unless the launcher gives it
one: it picks up a task, runs `gh`, is told to run `gh auth login`, is
forbidden from running it, and stops. Every assigned task dies on arrival.

`donate-clanker-container` therefore passes `GH_TOKEN` **by value**, resolved
in order: `DONATE_CLANKER_GH_TOKEN` (a purpose-made, narrowly scoped PAT), an
exported `GH_TOKEN`, then `gh auth token --hostname github.com`.

By value and not by mounting `~/.config/gh`, for the same reason the Copilot
credential is: one credential for one host, and no view of other accounts,
hosts, or an enterprise login in the same file. Upstream's own
`just contribute-run` passes `-e GH_TOKEN` the same way.

The launcher prints the token's **scopes** -- never its value -- and warns, by
name, on any of `admin:org`, `admin:public_key`, `delete:packages`,
`write:packages`, `workflow`. None is needed to fork, push and open a pull
request; each is a power a runaway or prompt-injected agent could use to do
damage no review would catch -- rewriting org membership, deleting published
packages, dispatching workflows, registering a lasting SSH key.

It warns, never refuses: a desktop `gh` login routinely carries all of those
and must stay usable. Narrow it with `DONATE_CLANKER_GH_TOKEN`.

Scopes come from `gh auth status` with the outgoing token in gh's environment,
so the *right* credential is inspected and not whatever sits in the keyring.
`gh` is already required, so this adds no dependency. Detection is best-effort:
an unreadable scope line prints "reach unknown" and proceeds, rather than
failing the launch or implying the token is narrow.

`donate-clanker-doctor` reports and warns identically, read-only. The VM path
does not carry this yet: its bootstrap envelope is consumed by the
guest image, which lives outside this repository, so a new envelope field
cannot be honoured from here alone.

### The 55-minute token

The scoped GitHub token Hive issues expires 55 minutes after assignment and
is **never refreshed**. It is not a substitute for the identity above:
`task_assign.github_token` reaches `injectGhToken` in `contributor-relay.sh`,
which writes it to `/var/run/hive-metrics/gh-app-token.cache` -- a file the
`gh` wrapper reads **only when not in contributor mode**. In contributor mode
that token is written and never read, so nothing exposes it to the agent's
shell. Plan every task to push or open its pull request well
inside that window. A task that spends 50 minutes exploring and then tries to
push will fail at the last step with an authentication error that looks like
a permissions bug and is not.

### Self-reported completion and the 168-hour cooldown

Completion is self-reported by the agent. Once reported, the issue enters a
**168-hour cooldown**, whether the task passed or failed. There is no retry
loop, and a premature or mistaken completion report costs a week on that
issue.

Report completion only after the verifiable artifact exists — a pushed
branch, an opened pull request, a green check — not after the intent to
create one.

## Red Flags

- Code in this repository that creates a tmux session, injects keystrokes, or
  scrapes pane output. That is Hive's job; deleting the duplicate is the fix.
- Any allow-list, deny-list, or conditional keyed on an assignment's
  repository, label, title, author, or issue number. Selection is Hive's.
- A wrapper around `contributor-agent.sh`, or any code here that reads or
  sends `task_assignment`, `task_failed`, or `task_completed`. Those are the
  only footholds a filter could use.
- "We only want issues from repo X" as a request. That belongs in the hub's
  config, where it is visible to everyone, not in this launcher.
- Renaming or parameterizing the `contributor` session name.
- A summary longer than 15 lines, or a report followed by more output.
- Assuming the GitHub token can be refreshed, rotated, or re-requested.
- Mounting `~/.config/gh` to give the agent an identity. Pass one token by
  value; the directory holds credentials for hosts and accounts the task has
  no business seeing.
- Echoing, logging or persisting a token value anywhere. Scopes and
  presence/absence are the only reportable facts.
- Reporting completion to unblock yourself from a stuck state. That burns 168
  hours.
- Treating a silent pane as a crash without attaching to look first.

## Verification

```bash
podman exec -it <container> tmux ls
podman exec -it <container> tmux attach -t contributor
```

`tmux ls` must show `contributor`. Before reporting completion, confirm the
artifact independently:

```bash
gh pr view --json url,state
git log --oneline -1
```

Confirm the final 15 lines of the pane carry the conclusion.

The no-filtering invariant is enforced statically, so it needs no live Hive:

```bash
bash tests/just-onboarding.sh
```
