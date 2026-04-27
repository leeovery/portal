---
status: complete
created: 2026-04-27
cycle: 4
phase: Plan Integrity Review
topic: built-in-session-resurrection
---

# Review Tracking: Built-In Session Resurrection - Integrity

## Findings

### 1. Phase 6 task 6-9 cannot detect corrupt-`sessions.json` because Phase 3 `Restore()` swallows the signal

**Severity**: Critical
**Plan Reference**: Phase 6 task 6-9 ("EnsureSaver / Restore failure → soft warning") interacting with Phase 3 task 3-6 (`Orchestrator.Restore()`) and task 3-1 (`ReadIndex`)
**Category**: Dependencies and Ordering / Task Self-Containment
**Change Type**: update-task

**Details**:
Task 6-9's "Do" tells the orchestrator to detect the corrupt-index condition via `errors.Is(err, restore.ErrCorruptIndex)` returned from step 5 (`Restore()`), and then emit `CorruptSessionsJSONWarning()` from the bootstrap orchestrator. Two integrity problems with this:

1. **No sentinel exists.** Task 3-1 (`ReadIndex`) returns wrapped errors with prefixes like `"parse sessions.json: ..."` but exports no sentinel. Task 6-9 hedges with "if it doesn't, this task adds it as part of the Restore reader" — but adding the sentinel only matters if the orchestrator can observe it.

2. **The orchestrator never observes the error.** Task 3-6's `Orchestrator.Restore()` deliberately swallows the corrupt-index signal — it logs to `portal.log`, writes the spec stderr lines directly via `o.Stderr`, and returns `nil`. The Phase 5 orchestrator's `Restore.Restore()` call therefore receives `(nil)` even when `sessions.json` is corrupt; `errors.Is(err, restore.ErrCorruptIndex)` is permanently false; the warning is never appended to `Warnings`.

Net effect: Phase 6 task 6-9's `CorruptSessionsJSONWarning` accumulator path is dead code under the plan-as-built. The corrupt-sessions warning still reaches stderr — but only because task 3-6 does its own direct write inside `Restore()`. That bypasses the sink entirely, which means task 6-10's TUI buffering (alt-screen exit before flush) is also bypassed: the warning is written to stderr while the Bubble Tea loading page is rendering, corrupting the UI on exactly the failure mode the buffering was designed to handle.

The fix has two coordinated parts. Task 3-6 must stop writing to stderr directly and must return a typed sentinel. Task 6-9 then routes the corrupt-index detection through the orchestrator's Restore step as designed.

**Current** (task 3-1 — relevant excerpt from `cmd/state_index_reader.go` description, no exported sentinel):

```
**Do**:
- Create `internal/state/index_reader.go`:
  - `func ReadIndex(dir string) (Index, bool, error)`:
    1. `path := SessionsJSON(dir)`.
    2. `data, err := os.ReadFile(path)`.
    3. If `errors.Is(err, os.ErrNotExist)` → return `Index{}, true, nil`.
    4. If any other `err != nil` → return `Index{}, true, fmt.Errorf("read sessions.json: %w", err)`.
    5. `idx, derr := DecodeIndex(data)`. If `derr != nil` → return `Index{}, true, fmt.Errorf("parse sessions.json: %w", derr)`.
    6. If `idx.Version > SchemaVersion` → return `Index{}, true, fmt.Errorf("sessions.json schema version %d unsupported (current: %d) — skipping restore", idx.Version, SchemaVersion)`.
    7. If `idx.Version < 1` → return `Index{}, true, fmt.Errorf("sessions.json missing or zero version — skipping restore")` (defensive; a v0 file should never exist but reject it explicitly rather than silently treating it as v1).
    8. Return `idx, false, nil`.
- Do NOT log inside `ReadIndex` — the orchestrator in task 3-6 owns the single log line + stderr warning. Keeping `ReadIndex` side-effect-free makes it trivially testable.
```

**Proposed** (task 3-1 — add `ErrCorruptIndex` sentinel and wrap parse / version errors with it):

```
**Do**:
- Create `internal/state/index_reader.go`:
  - Define `var ErrCorruptIndex = errors.New("sessions.json corrupt or unreadable")` — exported sentinel that Phase 5's orchestrator (task 5-2) and Phase 6's warning accumulator (task 6-9) match against via `errors.Is`. Wraps every "we tried to use the file but couldn't" condition; specifically does NOT cover the missing-file case (returned with `err=nil`).
  - `func ReadIndex(dir string) (Index, bool, error)`:
    1. `path := SessionsJSON(dir)`.
    2. `data, err := os.ReadFile(path)`.
    3. If `errors.Is(err, os.ErrNotExist)` → return `Index{}, true, nil`.
    4. If any other `err != nil` → return `Index{}, true, fmt.Errorf("read sessions.json: %w: %w", ErrCorruptIndex, err)`.
    5. `idx, derr := DecodeIndex(data)`. If `derr != nil` → return `Index{}, true, fmt.Errorf("parse sessions.json: %w: %w", ErrCorruptIndex, derr)`.
    6. If `idx.Version > SchemaVersion` → return `Index{}, true, fmt.Errorf("sessions.json schema version %d unsupported (current: %d) — skipping restore: %w", idx.Version, SchemaVersion, ErrCorruptIndex)`.
    7. If `idx.Version < 1` → return `Index{}, true, fmt.Errorf("sessions.json missing or zero version — skipping restore: %w", ErrCorruptIndex)`.
    8. Return `idx, false, nil`.
- Every error path other than "file absent" wraps `ErrCorruptIndex` via `fmt.Errorf("...: %w", ErrCorruptIndex, ...)` (Go 1.20+ multiple-`%w` wrapping). Callers use `errors.Is(err, state.ErrCorruptIndex)` to classify the condition without string matching.
- Do NOT log inside `ReadIndex` — the orchestrator (task 3-6) and the bootstrap orchestrator (task 5-2 / 6-9) own logging and stderr emission respectively. Keeping `ReadIndex` side-effect-free makes it trivially testable.
```

Add to task 3-1 acceptance criteria:

```
- [ ] `state.ErrCorruptIndex` is exported as a sentinel error.
- [ ] Every non-"file absent" error path wraps `ErrCorruptIndex` so `errors.Is(err, ErrCorruptIndex)` returns true.
- [ ] Missing-file path returns `(Index{}, true, nil)` — `ErrCorruptIndex` is NOT wrapped (absence is not corruption).
```

Add to task 3-1 tests:

```
- `"errors.Is(err, ErrCorruptIndex) is true for unparseable JSON"`
- `"errors.Is(err, ErrCorruptIndex) is true for unsupported version"`
- `"errors.Is(err, ErrCorruptIndex) is true for permission errors on a present file"`
- `"errors.Is(err, ErrCorruptIndex) is false for the missing-file case (err is nil)"`
```

**Resolution**: Fixed
**Notes**: Pairs with finding 2 — both parts must land together for the corrupt-index warning to flow through the sink as task 6-9 intends.

---

### 2. Phase 3 task 3-6 writes corrupt-sessions stderr directly, bypassing Phase 6's sink/TUI buffering

**Severity**: Critical
**Plan Reference**: Phase 3 task 3-6 (`Orchestrator.Restore()` corrupt-index branch) interacting with Phase 6 tasks 6-9 (`BootstrapWarningsSink`) and 6-10 (TUI buffering)
**Category**: Dependencies and Ordering / Task Self-Containment
**Change Type**: update-task

**Details**:
Task 3-6's `Restore()` writes the spec's two-line corrupt-sessions warning to `o.Stderr` directly inside the orchestrator and returns nil. Phase 6 task 6-9 introduces `BootstrapWarningsSink` and `CorruptSessionsJSONWarning` carrying the **identical two-line copy**, plus task 6-10 buffers TUI-path warnings to flush after alt-screen exit so they do not corrupt the rendered loading page.

If both wirings ship as written:
- CLI path: warning is double-emitted (once by `Restore()` directly, once by `PersistentPreRunE` draining the sink).
- TUI path: `Restore()`'s direct stderr write fires *while the Bubble Tea loading page is rendering* — exactly the corruption mode 6-10 was designed to prevent. The sink flush via `tea.Sequence(ExitAltScreen, emit, EnterAltScreen)` then emits an empty buffer (or a duplicate, depending on whether the warning was added to the sink at all).

The fix is to remove the direct stderr write from `Restore()` and have the orchestrator surface the corrupt-index condition by **propagating** the wrapped `ErrCorruptIndex` (finding 1) up to the bootstrap orchestrator. The bootstrap orchestrator (task 5-2) appends the warning to its accumulator on `errors.Is(restoreErr, state.ErrCorruptIndex)` — task 6-9's sink + 6-10's buffering then handle CLI vs TUI emission consistently.

The `Stderr io.Writer` field on `Orchestrator` becomes vestigial (only the corrupt-index warning used it). Either delete it or repurpose it for future direct-emit needs; deletion is cleaner.

**Current** (task 3-6, full task body):

```markdown
## built-in-session-resurrection-3-6 | approved

### Task 3-6: Implement top-level `Restore()` orchestrator with per-session error isolation

**Problem**: Tasks 3-1 through 3-5 land single-session primitives (read index, create FIFO, create session, apply geometry, set markers). Step 5 of `PersistentPreRunE` calls one `Restore()` entry point — not six. That entry point must iterate every saved session, skip live-named sessions, skeleton-create each missing one, and handle per-session failures in isolation so one broken session never blocks the remaining ones. The spec enumerates failure modes at the top level: missing `sessions.json` is a silent no-op; unparseable `sessions.json` is a one-line stderr warning + log + skip-all; a session whose `panes` array is empty is a per-session log + skip (Portal refuses to create a session whose pane topology cannot be specified); any per-session creation error logs and continues to the next session; `_`-prefixed session names in the index are skipped defensively (they are Portal internals and should never appear, but be robust). This orchestrator task is where those cross-cutting policies live, decoupled from the single-session primitives they compose.

**Solution**: Create `internal/restore/restore.go` with `type Orchestrator struct { Client *tmux.Client; StateDir string; Logger *log.Logger; Stderr io.Writer }` and `func (o *Orchestrator) Restore() error`. Flow: (1) resolve state dir via `state.EnsureDir()`; (2) call `state.ReadIndex(dir)` from task 3-1 — on `skip=true, err=nil` return nil; on `skip=true, err!=nil` log the error, emit the spec's one-line stderr warning `"Portal state file is corrupt — restoration skipped.\nCheck `portal state status` or ~/.config/portal/state/portal.log."`, return nil; (3) read live sessions via `tmux list-sessions -F '#{session_name}'` into a set; (4) for each `sess` in `idx.Sessions`: (a) if `strings.HasPrefix(sess.Name, "_")` skip with log; (b) if session name is in live set, skip (no log — quiet no-op, this is the common steady-state case); (c) if `len(sess.Windows) == 0` or any window has `len(Panes) == 0`, log warning and skip; (d) else `SessionRestorer{Client, StateDir}.Restore(sess)` + `ApplyWindowGeometry(sess, base, paneBase)` + `ApplySkeletonMarkers(sess, base, paneBase)` — errors from any phase log per-session and continue. No fatal error propagates out of the orchestrator; `Restore()` returns `nil` even when every session failed (bootstrap proceeds). Task 3-7 wraps this with the `@portal-restoring` set/unset discipline.
```

(For brevity, only the type + flow signature change is shown above; the full task includes a code block illustrating `Restore()` with the direct stderr write. Replacement below shows the corrected behaviour.)

**Proposed** (task 3-6 — return wrapped `ErrCorruptIndex` instead of writing stderr; drop the `Stderr` field):

```markdown
## built-in-session-resurrection-3-6 | approved

### Task 3-6: Implement top-level `Restore()` orchestrator with per-session error isolation

**Problem**: Tasks 3-1 through 3-5 land single-session primitives (read index, create FIFO, create session, apply geometry, set markers). Step 5 of `PersistentPreRunE` calls one `Restore()` entry point — not six. That entry point must iterate every saved session, skip live-named sessions, skeleton-create each missing one, and handle per-session failures in isolation so one broken session never blocks the remaining ones. The spec enumerates failure modes at the top level: missing `sessions.json` is a silent no-op; unparseable `sessions.json` is a one-line stderr warning + log + skip-all; a session whose `panes` array is empty is a per-session log + skip (Portal refuses to create a session whose pane topology cannot be specified); any per-session creation error logs and continues to the next session; `_`-prefixed session names in the index are skipped defensively (they are Portal internals and should never appear, but be robust). This orchestrator task is where those cross-cutting policies live, decoupled from the single-session primitives they compose. Stderr emission of the corrupt-index warning is **not** this orchestrator's responsibility — it propagates the wrapped `state.ErrCorruptIndex` (task 3-1) and the bootstrap orchestrator (task 5-2) routes the warning through Phase 6's `BootstrapWarningsSink` (task 6-9) so CLI and TUI paths emit consistently.

**Solution**: Create `internal/restore/restore.go` with `type Orchestrator struct { Client *tmux.Client; StateDir string; Logger *log.Logger }` (no `Stderr` field — emission belongs to Phase 6's sink) and `func (o *Orchestrator) Restore() error`. Flow: (1) resolve state dir via `state.EnsureDir()`; (2) call `state.ReadIndex(dir)` from task 3-1 — on `skip=true, err=nil` return nil; on `skip=true, err!=nil` log the wrapped error to `portal.log` and **return the error verbatim** so `errors.Is(err, state.ErrCorruptIndex)` is observable by the bootstrap orchestrator; (3) read live sessions via `tmux list-sessions -F '#{session_name}'` into a set; (4) for each `sess` in `idx.Sessions`: (a) if `strings.HasPrefix(sess.Name, "_")` skip with log; (b) if session name is in live set, skip (no log — quiet no-op, this is the common steady-state case); (c) if `len(sess.Windows) == 0` or any window has `len(Panes) == 0`, log warning and skip; (d) else `SessionRestorer{Client, StateDir}.Restore(sess)` + `ApplyWindowGeometry(sess, base, paneBase)` + `ApplySkeletonMarkers(sess, base, paneBase)` — errors from any phase log per-session and continue. No fatal error propagates out of the per-session loop; `Restore()` returns `nil` after the loop completes (bootstrap proceeds). Task 3-7 wraps this with the `@portal-restoring` set/unset discipline.

**Outcome**: One callable entry point used by Phase 5's bootstrap integration: `err := restore.New(client, stateDir, logger).Restore()`. Every saved session is either skipped (already live, name-prefixed `_`, empty panes) or skeleton-restored (create → geometry → markers). Per-session failures are logged with session-name context; the loop continues. The corrupt-index condition surfaces as a wrapped `state.ErrCorruptIndex` return; the bootstrap orchestrator (task 5-2) detects it via `errors.Is` and appends `CorruptSessionsJSONWarning()` to its accumulator (task 6-9). Unit tests cover: missing index, corrupt index (returns wrapped sentinel), empty index, single session happy path, live-skip, empty-panes skip, `_`-prefix skip, one-session-errors-others-continue.

**Do**:
- Create `internal/restore/restore.go`:
  ```go
  package restore

  import (
      "errors"
      "log"
      "strings"

      "github.com/leeovery/portal/internal/state"
      "github.com/leeovery/portal/internal/tmux"
  )

  type Orchestrator struct {
      Client   *tmux.Client
      StateDir string
      Logger   *log.Logger
  }

  func New(c *tmux.Client, stateDir string, logger *log.Logger) *Orchestrator {
      return &Orchestrator{Client: c, StateDir: stateDir, Logger: logger}
  }

  func (o *Orchestrator) Restore() error {
      idx, skip, readErr := state.ReadIndex(o.StateDir)
      if skip {
          if readErr != nil {
              o.Logger.Printf("restore: %v", readErr)
              // Return the wrapped error so bootstrap orchestrator (task 5-2)
              // can detect via errors.Is(err, state.ErrCorruptIndex) and emit
              // CorruptSessionsJSONWarning through the sink (task 6-9).
              return readErr
          }
          return nil
      }
      liveSet, err := o.Client.ListSessionNames()
      if err != nil {
          o.Logger.Printf("restore: list-sessions failed: %v", err)
          return nil
      }
      sr := &SessionRestorer{Client: o.Client, StateDir: o.StateDir, Logger: o.Logger}
      for _, sess := range idx.Sessions {
          if strings.HasPrefix(sess.Name, "_") {
              o.Logger.Printf("restore: skipping underscore-prefixed session %q (reserved)", sess.Name)
              continue
          }
          if _, live := liveSet[sess.Name]; live {
              continue // steady-state no-op; no log
          }
          if len(sess.Windows) == 0 {
              o.Logger.Printf("restore: skipping %q: zero windows in sessions.json", sess.Name)
              continue
          }
          emptyPaneWindow := false
          for _, w := range sess.Windows {
              if len(w.Panes) == 0 { emptyPaneWindow = true; break }
          }
          if emptyPaneWindow {
              o.Logger.Printf("restore: skipping %q: at least one window has zero panes", sess.Name)
              continue
          }
          baseIdx, paneBaseIdx, err := sr.PredictLiveIndices(sess)
          if err != nil {
              o.Logger.Printf("restore: %q: predict indices failed: %v", sess.Name, err)
              continue
          }
          if err := sr.Restore(sess, baseIdx, paneBaseIdx); err != nil {
              o.Logger.Printf("restore: %q: create failed: %v", sess.Name, err)
              continue
          }
          if err := sr.ApplyWindowGeometry(sess, baseIdx, paneBaseIdx); err != nil {
              o.Logger.Printf("restore: %q: geometry failed: %v", sess.Name, err)
          }
          if err := sr.ApplySkeletonMarkers(sess, baseIdx, paneBaseIdx); err != nil {
              o.Logger.Printf("restore: %q: markers failed: %v", sess.Name, err)
              continue
          }
      }
      return nil
  }
  ```
- Add `tmux.Client.ListSessionNames() (map[string]struct{}, error)` if absent — wraps `tmux list-sessions -F '#{session_name}'` and returns a set.
- Tests in `internal/restore/restore_test.go`:
  - Missing `sessions.json` → zero tmux calls, zero log lines, zero stderr writes, returns nil.
  - Corrupt `sessions.json` → one log line, returns a non-nil error wrapping `state.ErrCorruptIndex` (verified via `errors.Is`); zero session-creation tmux calls; zero stderr writes (the warning is emitted by Phase 6's sink, not here).
  - Valid empty index (`sessions: []`) → `list-sessions` called, zero session-creation calls, returns nil.
  - Valid index with one session, not live: full create → geometry → markers pipeline runs.
  - Valid index with one session, already live: `list-sessions` is called, no create/geometry/markers calls, zero log lines (steady-state silent).
  - Valid index with one session, name starts with `_` (defensive, spec says index should never contain these but the filter must exist): logged as skip, no create.
  - Valid index with one session, `windows: []`: logged as skip, no create.
  - Valid index with one session, one window with `panes: []`: logged as skip, no create.
  - Valid index with two sessions, first fails at `Restore`, second succeeds: first logs error, second completes; overall return is nil.
  - Valid index with two sessions, first fails at `ApplyWindowGeometry`, second proceeds normally: first logs geometry error (markers still applied for first), second unaffected.
  - `list-sessions` fails: log line, return nil (bootstrap continues without per-session restore work).
  - `predictLiveIndices` fails for one session: logged, skipped, next session proceeds.

**Acceptance Criteria**:
- [ ] Missing `sessions.json` is a silent no-op — no logs, no stderr, no tmux calls beyond the initial `list-sessions` (which itself is skipped when no file exists per the `skip=true, err=nil` branch).
- [ ] Unparseable `sessions.json` logs the parse error to `portal.log` and **returns** the wrapped error verbatim so the bootstrap orchestrator (task 5-2) can detect via `errors.Is(err, state.ErrCorruptIndex)`.
- [ ] `Restore()` does NOT write to stderr directly — stderr emission of the corrupt-index warning is task 6-9's sink path.
- [ ] `Orchestrator` struct has no `Stderr io.Writer` field (removed; emission is Phase 6's responsibility).
- [ ] `_`-prefixed session names in the index are defensively skipped with a log.
- [ ] Live session (name exists in `list-sessions`) is silently skipped (no log — steady-state case).
- [ ] Session with zero windows is skipped with a log.
- [ ] Session with any zero-pane window is skipped with a log.
- [ ] Each surviving session runs create → geometry → markers in that order.
- [ ] Error in `Restore` (create-session step) logs with session name and continues to the next session.
- [ ] Error in `ApplyWindowGeometry` logs and still runs `ApplySkeletonMarkers` (markers are independently useful for signal-hydrate coverage).
- [ ] Error in `ApplySkeletonMarkers` logs and continues to the next session.
- [ ] Per-session loop never aborts; the only non-nil return from `Restore()` is the wrapped `ErrCorruptIndex` from `ReadIndex`.

**Tests**:
- `"it is a silent no-op when sessions.json is absent"`
- `"it returns a wrapped ErrCorruptIndex when sessions.json is unparseable (no stderr write)"`
- `"it logs the parse error to portal.log on the corrupt-index path"`
- `"it does nothing beyond list-sessions when the index is empty"`
- `"it skeleton-restores a single missing session end-to-end"`
- `"it silently skips sessions whose name is already live (no log)"`
- `"it defensively skips underscore-prefixed session names in the index"`
- `"it logs and skips sessions with zero windows"`
- `"it logs and skips sessions with any zero-pane window"`
- `"it isolates per-session errors (one fails, next continues)"`
- `"it continues to ApplySkeletonMarkers after ApplyWindowGeometry fails for a session"`
- `"it logs and returns nil when list-sessions itself fails"`
- `"it returns nil when every session errors (per-session isolation; only ErrCorruptIndex propagates)"`

**Edge Cases**:
- Missing index → common first-ever-bootstrap case; must be silent.
- Unparseable index → rare but observable; one log line, wrapped `ErrCorruptIndex` returned. Bootstrap orchestrator (task 5-2) detects via `errors.Is` and appends `CorruptSessionsJSONWarning` (task 6-9). The sink emits to stderr (CLI) or buffers for TUI flush (task 6-10).
- `_`-prefixed session in the index → spec says daemon filters these from capture, so they should never appear; but be defensive and skip.
- Live-session name collision → already-live sessions are authoritative per spec; silent skip (no log) avoids noise on every `portal open`.
- `panes` empty array per spec: "Portal never creates a session whose pane topology cannot be specified." Log, skip, continue.
- Per-session error isolation: ensures one corrupt saved session does not block a user's remaining 9 from restoring.
- `list-sessions` failure: unusual but possible. Log, return nil — bootstrap continues with whatever live tmux has.
- `sessions.json` present but schema `version` is future: handled by task 3-1's `ReadIndex` wrapping the error with `ErrCorruptIndex`. Same propagation path; same Phase 6 warning surfaces to the user.

**Context**:
> Spec "Restore-Side Architecture → Restoration Trigger": "For each entry in `sessions.json`: If a live tmux session already exists with that name → skip. ... If no live session with that name → skeleton-restore it (structure only; scrollback lazy). If a saved session's `panes` array is empty (corrupt or invalid `sessions.json`) → log a warning, skip that window/session entirely, and continue restoring the remaining sessions. Portal never creates a session whose pane topology cannot be specified."
>
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 5. `Restore()`":
> - If `sessions.json` does not exist → no-op; continue to step 6.
> - If unparseable → log warning, print one-line stderr warning, skip restoration entirely; continue to step 6.
> - Otherwise, parse `sessions.json`. For each saved session: has-session → skip if live; else skeleton-create.
> - On `select-layout` failure for a window: fall back to `select-layout tiled`, log, continue.
> - On any per-session error: log, skip that session, continue with the next.
>
> The "print one-line stderr warning" step is owned by Phase 6 task 6-9's `BootstrapWarningsSink` (CLI) / task 6-10's TUI buffering, not by this orchestrator. The orchestrator's job is to make the corrupt-index condition observable to the bootstrap layer via `errors.Is(err, state.ErrCorruptIndex)`.
>
> Spec "Observability & Diagnostics → Proactive Health Signals → Exception: genuinely broken states detected during `PersistentPreRunE`": the corrupt-sessions warning text is `"Portal state file is corrupt — restoration skipped.\nCheck `portal state status` or ~/.config/portal/state/portal.log."` — emitted by task 6-9's `CorruptSessionsJSONWarning()` constructor.
>
> Spec "Failure Modes & Recovery → Consolidated Failure-Handling Table → `sessions.json` corrupt / unparseable": "Log warning, emit one-line stderr warning (see Observability), skip restoration entirely, continue bootstrap. User sees an empty picker. Diagnosable via log file or file inspection. Next successful save overwrites with valid content."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Restore-Side Architecture → Restoration Trigger", "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 5.", "Observability & Diagnostics → Proactive Health Signals", "Failure Modes & Recovery".
```

**Resolution**: Fixed
**Notes**: Pairs with finding 1. Together they unify the corrupt-index warning path — orchestrator surfaces the sentinel; bootstrap layer routes it through the sink; sink handles CLI vs TUI emission. Without both fixes, the warning either double-emits (CLI) or corrupts the loading page (TUI).

---

### 3. Phase 5 task 5-2 must update its handling of step 5 to detect `ErrCorruptIndex` and append the soft warning

**Severity**: Important
**Plan Reference**: Phase 5 task 5-2 (`Orchestrator.Run` step 5) — closing the loop with findings 1 and 2
**Category**: Dependencies and Ordering
**Change Type**: update-task

**Details**:
With findings 1 and 2 applied, `Restore()` returns a non-nil error wrapping `state.ErrCorruptIndex` on the corrupt-sessions path. Task 5-2's step 5 currently captures `restoreErr := o.Restore.Restore()` and surfaces it as a fatal at the end of `Run` ("Return `(serverStarted, restoreErr)` — if `Restore()` errored, surface it"). That is wrong for `ErrCorruptIndex` — corrupt-sessions is **soft** per spec, not fatal. Task 5-2 must classify the returned error: if it wraps `ErrCorruptIndex`, append `CorruptSessionsJSONWarning()` (added in 6-9) to the warnings accumulator and continue; otherwise the error is genuinely exceptional (the orchestrator's per-session loop already swallowed per-session failures, so a non-`ErrCorruptIndex` error from `Restore()` indicates an internal Restore bug worth surfacing).

This update is small: a single `errors.Is` check in step 5's branch, mirroring the existing `EnsureSaver` soft-warning treatment in step 4.

**Current** (task 5-2 — relevant excerpt of `Run` step 5):

```
5. `restoreErr := o.Restore.Restore()`; capture — do not return yet. Per spec "Restore-Side Architecture → Restoration Trigger": "a missing or unparseable `sessions.json` is a non-fatal no-op warning" — so `Restore()` should already swallow its own per-session errors; an error surfacing here is exceptional.
```

**Proposed** (task 5-2 — classify `ErrCorruptIndex` as soft, others as exceptional):

```
5. `restoreErr := o.Restore.Restore()`; capture — do not return yet.
   - If `errors.Is(restoreErr, state.ErrCorruptIndex)` → append `CorruptSessionsJSONWarning()` to the accumulator (task 6-9 introduces this), log WARN to `portal.log` via `ComponentBootstrap`, set `restoreErr = nil` (the condition is soft, not exceptional). Spec "Restore-Side Architecture → Restoration Trigger": "a missing or unparseable `sessions.json` is a non-fatal no-op warning."
   - Any other non-nil `restoreErr` is genuinely exceptional — the per-session loop already swallowed per-session failures, so a remaining error indicates an internal `Restore()` bug. Log ERROR via `ComponentBootstrap` and return as a `FatalError` per task 6-8 ("Portal restore failed: <underlying>"). This branch should never fire under correct task 3-6 implementation; the fatal wrapping is defensive.
   - Step 6 (`Restoring.Clear`) and subsequent steps still execute regardless of whether step 5 produced a soft warning.
```

Add to task 5-2 acceptance criteria (replacing the existing `EnsureSaver`-only warning row):

```
- [ ] Step 5 (`Restore`) classifies returned errors: `errors.Is(err, state.ErrCorruptIndex)` produces a `CorruptSessionsJSONWarning` accumulator entry and is treated as soft; any other non-nil error is wrapped as `FatalError` and surfaced as fatal exit.
- [ ] `EnsureSaver` failure (step 4) and corrupt-index (step 5) are the two soft-warning paths in v1; both flow through the same `Warnings []bootstrap.Warning` accumulator pattern (task 6-9).
```

Add to task 5-2 tests:

```
- `"it appends CorruptSessionsJSONWarning to Warnings when Restore returns ErrCorruptIndex"`
- `"it does not return a FatalError when Restore returns ErrCorruptIndex (soft path)"`
- `"it still runs Restoring.Clear / SweepOrphanFIFOs / CleanStale after a soft corrupt-index"`
- `"it returns a FatalError when Restore returns a non-ErrCorruptIndex error (defensive)"`
```

**Resolution**: Fixed
**Notes**: Mechanical follow-on once findings 1 and 2 land. The change pattern mirrors task 5-2's existing `EnsureSaver` soft-warning treatment.

---

### 4. Task 6-7 contains a stale "(Removed: ...)" parenthetical inside the `Do` section's numbered steps

**Severity**: Minor
**Plan Reference**: Phase 6 task 6-7 (`--purge` flag), `Do` section step 3
**Category**: Task Template Compliance / Polish
**Change Type**: update-task

**Details**:
Task 6-7's `Do` section step 3 reads: `"3. (Removed: the prior `EvalSymlinks` strict-equality check is dropped — intermediate symlinks in the path resolution are tolerated; only the leaf being a symlink triggers refusal. This avoids a false-positive on macOS legacy installs whose intermediate paths route through other symlinked directories.)"`. This is a leftover commentary from a prior cycle's revision — it documents what was removed rather than describing what to do. The Do list reads as `1, 2, (removed), 4, 5` which is confusing for an implementer following the steps. The substantive content (only the leaf symlink check, no full `EvalSymlinks`) is already correctly described in step 2 and in the Edge Cases section.

Cleanup: drop step 3 entirely and renumber, OR fold the rationale into step 2 as a single-sentence comment. Either resolves the visual inconsistency.

**Current**:

```
- Implement `purgeStateDir(dir string, logger *state.Logger) error`:
    1. `info, err := os.Lstat(dir)` — if `ENOENT`, return nil (idempotent).
    2. If `info.Mode()&os.ModeSymlink != 0`, return an error: `"refusing to purge symlinked state dir: %s"` with a `logger.Warn` line. The caller can `readlink` the path and `rm -rf` the target manually if intentional.
    3. (Removed: the prior `EvalSymlinks` strict-equality check is dropped — intermediate symlinks in the path resolution are tolerated; only the leaf being a symlink triggers refusal. This avoids a false-positive on macOS legacy installs whose intermediate paths route through other symlinked directories.)
    4. `if err := os.RemoveAll(dir); err != nil { logger.Error(state.ComponentDaemon, "purge failed: %v", err); return err }`.
    5. Log Info: `"purged state directory %s"`.
```

**Proposed**:

```
- Implement `purgeStateDir(dir string, logger *state.Logger) error`:
    1. `info, err := os.Lstat(dir)` — if `ENOENT`, return nil (idempotent).
    2. If `info.Mode()&os.ModeSymlink != 0`, return an error: `"refusing to purge symlinked state dir: %s"` with a `logger.Warn` line. The caller can `readlink` the path and `rm -rf` the target manually if intentional. Only the **leaf** is checked — intermediate symlinks in the path resolution are tolerated (avoids false-positives on macOS legacy installs whose intermediate paths route through other symlinked directories like `~/Library/Application Support`).
    3. `if err := os.RemoveAll(dir); err != nil { logger.Error(state.ComponentDaemon, "purge failed: %v", err); return err }`.
    4. Log Info: `"purged state directory %s"`.
```

**Resolution**: Fixed
**Notes**: Pure cleanup — no behavioural change. Improves readability for the implementer.

---
