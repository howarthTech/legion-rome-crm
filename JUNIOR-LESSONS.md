# Junior-dev lessons — legion-rome-crm

Read on every `aider` invocation (`--read JUNIOR-LESSONS.md`). Keep it TRUE:
only verified, generalized rules. Correct wrong lessons in place; don't append around them.

## Project conventions
- Go 1.25, module `github.com/howarthTech/legion-rome-crm`. Pure-Go only
  (built `CGO_ENABLED=0`); SQLite is `modernc.org/sqlite` — never a cgo driver.
- Tests: standard `go test`, table-driven, `*_test.go` beside the code in the
  same package. Use `t.TempDir()`, `t.Context()`, `net/http/httptest`. No
  third-party test libs (no testify) — plain `if got != want { t.Errorf(...) }`.
- HTTP handlers live in `internal/handlers/` as `func Name(a *app.App) http.HandlerFunc`;
  store methods hang off `*store.Store` in `internal/store/`.
- SQL migrations: `internal/store/migrations/NNN_name.sql`, embedded and run in
  filename order (tracked in `schema_migrations`). Additive only — no edits to
  applied migrations.
- Templates: one `html/template` set per page (see `pageNames` in
  `internal/app/app.go`); every page template defines its own `"body"`.
- gofmt (tabs). Import order: stdlib group, blank line, then module imports.
  Wrap errors with `fmt.Errorf("context: %w", err)`.
- Handler redirects/flash: use the `redirect(w, r, path, key, msg)` helper;
  flash messages ride the query string (`?ok=` / `?err=`).

## Corrections
<!-- One-line generalized rules from fixed/reverted junior drafts. Empty for now. -->
