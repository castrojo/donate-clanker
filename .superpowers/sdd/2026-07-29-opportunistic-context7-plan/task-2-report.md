# Task 2 report

## Files
- `image/config/models.json`
- `internal/profile/catalog.go`
- `internal/profile/profile_test.go`

## Tests
- `go test ./internal/profile -run 'Thinking|RuntimeArgs'` → pass
- `go test ./internal/profile` → pass

## Output
- Added a standalone profile catalog loader with `Thinking` and `RuntimeArgs` validation.
- Added the approved Qwen3.5-4B, Qwen3.5-9B, Qwen3-Coder-30B-A3B, and Qwen3.6-35B-A3B entries.
- Kept `--thinking false` as the server-side disable flag and preserved Qwen template kwargs as separate runtime arguments.

## Concerns
- The catalog is intentionally minimal and does not implement hardware detection, profile selection, or Hive/Goose runtime wiring.
- Model IDs are stored as checked-in catalog keys only; later runner work may need to attach richer selection metadata.

## Commit
- `3e5d28f67a47b46e267762b755fc883ddc7eb2ce`
