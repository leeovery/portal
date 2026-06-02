---
topic: portal-observability-layer
cycle: 5
total_proposed: 2
---
# Analysis Tasks: Portal Observability Layer (Cycle 5)

## Task 1: Migrate tui previewCaptureSink onto the shared logtest.Sink helper
status: approved
severity: medium
sources: duplication

**Problem**: `internal/tui/preview_attach_test.go:83-150` hand-rolls `previewCaptureSink`, a verbatim, unmigrated copy of the now-shared `logtest.Sink` rendering core (`internal/logtest/capture.go:101-219`). The c3/c4 extraction that moved the capturing `slog.Handler` into `internal/logtest` missed this one call site. The duplicated struct (`mu sync.Mutex; lines []string; shared *previewCaptureSink; bound []slog.Attr`), `owner()`, `Enabled` (returns true unconditionally), `WithAttrs` (append bound+attrs into a fresh derived sink sharing the owner), `Handle` (renders the canonical `<LEVEL> <msg> key=value...` shape — `r.Level.String()` + `" "` + `r.Message`, then range `bound` then `r.Attrs` via `fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Any())`, appended under the owner's mutex), and `body()` (`strings.Join(s.lines, "\n")` under the mutex) are byte-for-byte identical to `logtest.Sink`'s `owner()`/`Enabled`/`WithAttrs`/`Handle`/`Body`. `newTestLogger(t)` (lines 140-144) duplicates `logtest.NewCaptureLogger` plus a `.With("component","preview")`. The rendered-body contract that the preview-attach substring assertions key on is now hand-maintained in parallel with the shared declaration and can drift from it. The one cosmetic deviation — `WithGroup(_ string) slog.Handler { return s }` (line 111) — returns the receiver rather than a sink sharing the owner; it is a latent bug versus `logtest.Sink`'s version that preserves bound+owner, but inert because no preview log path uses `WithGroup`. This is the last text-rendering re-author; after it the text flavor is fully consolidated.

**Solution**: Delete `previewCaptureSink` and its `body()`/`owner()`/`Enabled`/`WithAttrs`/`WithGroup`/`Handle` methods, and route `newTestLogger` through the shared `logtest.NewCaptureLogger` helper. `internal/logtest` is a verified leaf and the test file is `package tui`, so the import carries zero cycle risk. Pure test-scaffolding consolidation onto the existing identical contract — no production change, no behavior change. Fixes the inert `WithGroup` divergence as a side effect.

**Outcome**: `internal/tui/preview_attach_test.go` no longer hand-rolls a capturing `slog.Handler`; it consumes `logtest.NewCaptureLogger`/`logtest.Sink` like every other migrated test surface. The preview-attach rendered-body contract is single-sourced and cannot drift from the shared declaration. The preview-attach test suite passes unchanged.

**Do**:
1. Add `"github.com/leeovery/portal/internal/logtest"` to the imports of `internal/tui/preview_attach_test.go`. Drop now-unused imports (`context`, `fmt`, `strings`, `sync`, `slog` for the handler) only if no other code in the file still needs them — verify before removing.
2. Delete the `previewCaptureSink` struct (lines 83-93) and its methods: `owner()` (95-100), `Enabled` (102), `WithAttrs` (104-109), `WithGroup` (111), `Handle` (113-130), and `body()` (132-136).
3. Rewrite `newTestLogger(t *testing.T)` to return `(*slog.Logger, *logtest.Sink)`: `logger, sink := logtest.NewCaptureLogger(t); return logger.With("component", "preview"), sink`.
4. Retype the `readLog` helper parameter from `*previewCaptureSink` to `*logtest.Sink` and replace the single `sink.body()` call with `sink.Body()`.
5. Retype every other call site that holds the sink (the `newTestLogger`/`readLog` consumers in the file) from `*previewCaptureSink` to `*logtest.Sink`.
6. Run `go test ./internal/tui/...` and confirm the preview-attach substring assertions still pass.

**Acceptance Criteria**:
- `previewCaptureSink` and all its methods are removed from `internal/tui/preview_attach_test.go`.
- `newTestLogger` returns a logger routed through `logtest.NewCaptureLogger(t)` bound to `component=preview`, plus a `*logtest.Sink`.
- `readLog` reads the body via `logtest.Sink.Body()`.
- No production (`*.go` non-`_test.go`) file is modified.
- The rendered-body shape observed by the preview-attach assertions (`<LEVEL> <msg> key=value...`, bound attrs first) is unchanged.

**Tests**:
- `go test ./internal/tui/...` passes — the existing preview-attach tests that assert on the level label, the bound `component=preview` binding, and per-call attrs continue to pass against the shared sink with no assertion edits.
- `go build ./...` and `go vet ./internal/tui/...` are clean (no unused imports, no dangling references to `previewCaptureSink`).

## Task 2: Align restoretest.OpenTestLogger with the production sink's portal.log file contract
status: approved
severity: low
sources: architecture

**Problem**: `restoretest.OpenTestLogger` (`internal/restoretest/logger.go:27-38`) writes a REGULAR FILE at `<stateDir>/portal.log` via a vanilla `slog.NewTextHandler`, but production's `rotatingSink` (`internal/log/sink.go:304-344`) now owns `portal.log` as a SYMLINK pointing at `portal.log.<date>`, and its `reopen()` runs a `migrationGuard` that `os.Remove()`s any regular-file `portal.log` on first write. `internal/restore/exit_closes_pane_integration_test.go` builds and runs the real portal binary (line 293) against the SAME `stateDir` for which it opened a regular-file test logger (line 375). The two writers contend for the same path: the test's restorer appends to the regular file while the spawned helper/daemon's sink deletes that regular file and writes to `portal.log.<date>` through the symlink. This test passes today only because it asserts on tmux state, not on log content. The scaffold is a hand-rolled second implementation of the production sink that no longer matches its file contract — a latent trap for any future test that asserts on `portal.log` content while also spawning the real binary, which would read the regular file the production sink has already unlinked. The production sink is the real owner of the `portal.log` shape.

**Solution**: Route `OpenTestLogger` through the production sink shape rather than a bare regular file — drive it through `internal/log`'s own handler/sink, or have it write `portal.log.<date>` plus the `portal.log` symlink the way the sink does, so test infra and production agree on the `portal.log` contract. At minimum (if routing through the production sink is not cleanly achievable from `restoretest` without a cycle), document on `OpenTestLogger` that it must not share a `stateDir` with a real-binary subprocess, because the migration guard will delete its file. Prefer the real fix (agreeing on the file contract) over the documentation-only fallback.

**Outcome**: Test infrastructure and the production rotating sink agree on the `portal.log` on-disk shape. A test that opens an `OpenTestLogger` against a `stateDir` and also spawns the real binary no longer has two writers silently contending over a regular file the production migration guard will unlink — eliminating the latent trap for any future log-content assertion.

**Do**:
1. Inspect `internal/log/sink.go:304-344` (`rotatingSink.reopen()` / `migrationGuard`) to confirm the production `portal.log` shape: a symlink `portal.log → portal.log.<date>` with the regular-file `portal.log` removed by the migration guard on first write.
2. Determine whether `internal/restoretest` can import `internal/log` to construct the production handler without introducing an import cycle. `internal/log` is a leaf/foundation package, so `restoretest` (a test-only helper package) importing it should be safe — verify by building.
3. Preferred path: rewrite `OpenTestLogger` to obtain its `*slog.Logger` from the production `internal/log` handler/sink targeting `stateDir`, so it writes `portal.log.<date>` + the symlink exactly as production does and is compatible with a co-resident real-binary subprocess. Keep the existing `*testing.T`-first signature and `*slog.Logger` return type, and keep the `t.Cleanup`-based close so promoted call sites compile unchanged.
4. If (and only if) routing through `internal/log` is not cleanly achievable, fall back to making `OpenTestLogger` write the `portal.log.<date>` + `portal.log` symlink shape directly (mirroring the sink), and update the local `portalLogName` comment accordingly.
5. As a backstop in either case, update the `OpenTestLogger` doc comment to state the `portal.log` contract it now honors (and, in the fallback, the constraint that it must not share a `stateDir` with a real-binary subprocess if any residual incompatibility remains).
6. Run the affected integration tests, including `internal/restore/exit_closes_pane_integration_test.go`, and `go test ./internal/restoretest/... ./internal/restore/...`.

**Acceptance Criteria**:
- `OpenTestLogger` no longer writes a bare regular-file `portal.log` that the production migration guard would unlink when a real binary shares the `stateDir`; it either produces the production `portal.log.<date>` + symlink shape or routes through `internal/log`'s handler.
- The `*testing.T`-first signature and `*slog.Logger` return type are preserved; existing call sites (`internal/restore/exit_closes_pane_integration_test.go:375` and any others) compile unchanged.
- The close-on-cleanup behavior via `t.Cleanup` is preserved.
- The `OpenTestLogger` doc comment accurately describes the `portal.log` contract it now honors.
- No production (`internal/log`, `internal/restore`) runtime behavior is changed; only the test-infra logger is modified.

**Tests**:
- `go test ./internal/restoretest/...` passes.
- `internal/restore/exit_closes_pane_integration_test.go` continues to pass (it builds and runs the real binary against the shared `stateDir` at lines 293/375) — confirming the test logger and the production sink no longer contend destructively over `portal.log`.
- `go build ./...` is clean and no new import cycle is introduced (`go vet ./internal/restoretest/...`).
