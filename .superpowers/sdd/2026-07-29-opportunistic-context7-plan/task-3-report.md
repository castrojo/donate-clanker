# Task 3 Report

Status: DONE

## Summary
Implemented the smallest useful integration seam without inventing the missing contributor runner or Hive protocol worker.

## Files changed
- `internal/config/context7.go`
  - Added pure Goose default environment constants and `DefaultGooseEnvironment(model string) map[string]string`.
- `internal/config/context7_test.go`
  - Added coverage proving Goose defaults include `GOOSE_THINKING_EFFORT=off` and preserve the expected local provider/endpoint settings.
- `internal/runner/goose.go`
  - Added pure `PrepareTaskPrompt(policy string, assignment string) string` helper that prepends policy and keeps the Hive assignment verbatim as a distinct section.
- `internal/runner/goose_test.go`
  - Added coverage proving policy comes first and the original assignment text is preserved unchanged at the end of the prompt.

## Validation
### Required failing test before wiring
Command:
```sh
go test ./internal/config ./internal/runner -run 'Context7|Thinking|Policy'
```
Output:
```text
# github.com/castrojo/donate-clanker/internal/runner [github.com/castrojo/donate-clanker/internal/runner.test]
internal/runner/goose_test.go:12:12: undefined: PrepareTaskPrompt
# github.com/castrojo/donate-clanker/internal/config [github.com/castrojo/donate-clanker/internal/config.test]
internal/config/context7_test.go:81:9: undefined: DefaultGooseEnvironment
FAIL	github.com/castrojo/donate-clanker/internal/config [build failed]
FAIL	github.com/castrojo/donate-clanker/internal/runner [build failed]
FAIL
```

### Passing targeted validation after implementation
Command:
```sh
go test ./internal/config ./internal/runner -run 'Context7|Thinking|Policy'
```
Output:
```text
ok  	github.com/castrojo/donate-clanker/internal/config	0.004s
ok  	github.com/castrojo/donate-clanker/internal/runner	(cached)
```

## Blockers / scope boundaries
- `cmd/contributor/main.go` and the full contributor runner do not exist yet, so I did not fabricate startup wiring, Hive protocol handling, or a fake end-to-end worker.
- Context7 failure handling remains inside Goose/MCP. This change only adds pure prompt/environment helpers for the later runner to call.

## Prompt-preservation fix
- Updated `PrepareTaskPrompt` to use `TrimSpace` only for the empty-input guard, while emitting the original policy text unchanged.
- Added focused tests covering whitespace preservation and empty-policy fallback.
- No cmd/contributor or Hive wiring was added; that scope stays deferred until the runner exists.
