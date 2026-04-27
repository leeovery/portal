---
phase: 3
phase_name: Skeleton restore and lazy scrollback hydration
total: 13
---

## built-in-session-resurrection-3-1 | approved

### Task 3-1: Add `sessions.json` reader for bootstrap consumption

**Problem**: Phase 3's `Restore()` (task 3-6) and Phase 6's `portal state status` both need to read `sessions.json` from disk, hand the parsed contents to the skeleton-restore loop, and behave gracefully when the file is absent, unparseable, or carries an unknown future `version`. Task 2-3 landed the encoder/decoder primitives; what is missing is a single well-defined reader that composes them with filesystem access and the spec-mandated failure semantics: missing file is a non-fatal no-op, unparseable JSON logs a warning and returns a skip-all sentinel (no partial restore), future `version` values are also skipped with a log. Introducing this as a distinct task keeps the orchestrator in 3-6 focused on per-session orchestration rather than file-format concerns, and makes the "file absent vs corrupt vs future-version" three-way branch independently testable.

**Solution**: Add `internal/state/index_reader.go` exposing `ReadIndex(dir string) (idx Index, skip bool, err error)`. The function reads `sessions.json` under the state directory, classifies the result into three disjoint outcomes, and returns them explicitly: (a) file absent → `skip=true, err=nil` (caller treats as "nothing saved, continue"); (b) unparseable JSON or schema `version > SchemaVersion` → `skip=true, err=<warning error>` (caller logs the error and continues, but runs no per-session skeleton work); (c) parseable v1 → `skip=false, err=nil, idx=<parsed>`. Any unexpected I/O error (permission denied on a present file, etc.) returns `skip=true, err=<wrapped>` so the caller can log the underlying cause and still proceed to step 6 of bootstrap.

**Outcome**: A caller (`Restore()` in task 3-6) writes `idx, skip, err := state.ReadIndex(dir)`; logs `err` if non-nil; iterates `idx.Sessions` only when `!skip`. Task 3-6 remains free of JSON parsing and directly-on-disk file semantics. Unit tests cover all four cases (missing, unparseable, future-version, valid) plus permission error — no need to repeat them in the orchestrator's tests.

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
- Tests in `internal/state/index_reader_test.go` using `t.TempDir`:
  - Missing file → `skip=true, err=nil, idx=zero`.
  - File exists with valid v1 JSON → `skip=false, err=nil, idx=populated`.
  - File exists with invalid JSON (e.g., truncated) → `skip=true, err!=nil` and error message contains `"parse sessions.json"`.
  - File exists with `"version": 2` → `skip=true, err!=nil` and error message mentions unsupported version.
  - File exists with `"version": 0` or missing version → `skip=true, err!=nil`.
  - File exists with unknown extra fields → `skip=false, err=nil` (task 2-3 already tolerates unknown fields; this test documents the behaviour as load-bearing here too).
  - File exists with empty `sessions` array → `skip=false, err=nil, idx.Sessions` is an empty non-nil slice.
  - File exists but unreadable (chmod 000 on test file) → `skip=true, err!=nil` with wrapped permission error.

**Acceptance Criteria**:
- [ ] Missing `sessions.json` returns `(Index{}, true, nil)` — orchestrator treats as silent no-op.
- [ ] Unparseable `sessions.json` returns `(Index{}, true, non-nil err)` with `"parse sessions.json"` prefix in the error message.
- [ ] `version` greater than `SchemaVersion` returns `(Index{}, true, err)` with a message naming both the observed and current versions.
- [ ] `version` less than 1 returns `(Index{}, true, err)` (guards against v0 / missing).
- [ ] Valid v1 JSON returns `(populatedIndex, false, nil)`.
- [ ] Permission / I/O errors on a present file return `(Index{}, true, wrapped err)` — never panic.
- [ ] `ReadIndex` performs no logging, no stderr writes — purely returns values for the caller to act on.
- [ ] Empty `sessions[]` round-trips cleanly (`idx.Sessions` is a zero-length slice, not `nil`, not treated as error).

**Tests**:
- `"it returns skip=true with nil err when sessions.json is absent"`
- `"it returns the parsed index for a valid v1 file"`
- `"it returns skip=true with parse error for truncated JSON"`
- `"it returns skip=true with parse error for non-JSON content"`
- `"it returns skip=true with version error when schema version exceeds SchemaVersion"`
- `"it returns skip=true with version error when version is zero or missing"`
- `"it tolerates unknown fields in a valid v1 document"`
- `"it handles an empty sessions array"`
- `"it returns skip=true with a wrapped permission error when the file is unreadable"`
- `"it performs no stdout/stderr writes (purity check via captured output)"`

**Edge Cases**:
- Missing file is the dominant first-ever-bootstrap case: must not be an error and must not log.
- Unparseable JSON: user manually corrupted the file, or a prior crash left a half-written rename (extremely unlikely given `AtomicWrite`, but defensive). Single log line + stderr one-liner happen in task 3-6; this reader stays silent.
- Future `version` value: forward-compatibility scenario where the user downgrades Portal. Task 2-3 deliberately deferred schema migration; v1 reader must refuse to process `version > 1` rather than guess field semantics.
- Permission error on a present file: the user manually `chmod 000`'d it. Surface the underlying error; let the caller print the warning.
- Known `version = 1` with unknown extra fields: tolerated — the `encoding/json` default is lenient; verified here as load-bearing.

**Context**:
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 5. `Restore()` — skeleton-only restoration": "If `~/.config/portal/state/sessions.json` does not exist → no-op; continue to step 6. If `sessions.json` is unparseable (corrupt JSON) → log warning, print one-line stderr warning, skip restoration entirely; continue to step 6. Otherwise, parse `sessions.json`. For each saved session: ..."
>
> Spec "Restore-Side Architecture → Restoration Trigger": "For each entry in `sessions.json`: If a live tmux session already exists with that name → skip. ... If no live session with that name → skeleton-restore it (structure only; scrollback lazy)."
>
> Spec "Failure Modes & Recovery → Consolidated Failure-Handling Table → sessions.json corrupt / unparseable": "Log warning, emit one-line stderr warning (see Observability), skip restoration entirely, continue bootstrap. User sees an empty picker. Diagnosable via log file or file inspection. Next successful save overwrites with valid content."
>
> Task 2-3 (Phase 2) already landed `DecodeIndex(data []byte) (Index, error)`. This task composes that helper with filesystem access and the version-gate semantics.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → step 5", "Restore-Side Architecture → Restoration Trigger", "Failure Modes & Recovery".

## built-in-session-resurrection-3-2 | approved

### Task 3-2: Add per-pane FIFO create helper (`os.Remove` + `syscall.Mkfifo 0600`)

**Problem**: Every skeleton-restored pane needs a per-pane FIFO at `~/.config/portal/state/hydrate-<paneKey>.fifo` that the hydrate helper (task 3-8) will open for reading and `signal-hydrate` (task 3-11) will open for writing. The spec pins the creation sequence explicitly: `os.Remove(path)` (ignore `ENOENT`) followed by `syscall.Mkfifo(path, 0600)` — this defensive pattern replaces any stale FIFO left by a prior crashed bootstrap without needing a separate sweep step. Getting mode bits or error handling wrong here produces either permissive FIFOs (owner-privacy bug — other users on the machine could peek at the hydrate signal) or flaky restores (pre-existing regular file at the path makes `mkfifo` fail with `EEXIST`). This helper is called from task 3-3 inside a per-pane loop, so the primitive needs to be boringly correct in isolation before the orchestrator depends on it.

**Solution**: Add `func CreateFIFO(path string) error` in `internal/state/fifo.go`. The helper: (1) calls `os.Remove(path)` and tolerates `os.ErrNotExist`; (2) calls `syscall.Mkfifo(path, 0o600)`; (3) wraps any error with the path. No retry, no fallback — a `Mkfifo` failure propagates as the caller's problem. Skeleton-restore's per-session orchestrator (task 3-6) will log and skip that session on error; other sessions continue.

**Outcome**: `state.CreateFIFO("/home/u/.config/portal/state/hydrate-work__0.0.fifo")` creates a POSIX FIFO with mode `0600`. If a regular file or stale FIFO exists at the path, it is removed first. If the state directory is missing, the caller gets an error naming the missing path — directory creation is the caller's responsibility (handled once in `Restore()` via `state.EnsureDir()`, not per-FIFO). Callers in task 3-3 invoke this once per pane before `new-session`/`split-window`.

**Do**:
- Create `internal/state/fifo.go`:
  ```go
  package state

  import (
      "errors"
      "fmt"
      "os"
      "syscall"
  )

  func CreateFIFO(path string) error {
      if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
          return fmt.Errorf("remove existing fifo %s: %w", path, err)
      }
      if err := syscall.Mkfifo(path, 0o600); err != nil {
          return fmt.Errorf("create fifo %s: %w", path, err)
      }
      return nil
  }
  ```
- Tests in `internal/state/fifo_test.go` using `t.TempDir`:
  - Path does not exist → FIFO created, mode is `0600`, `os.Stat` reports it is a named pipe (`fi.Mode()&os.ModeNamedPipe != 0`).
  - Stale FIFO at path → replaced cleanly, new FIFO exists with mode `0600`.
  - Regular file at path → removed, FIFO created in its place.
  - Symlink at path → symlink itself is removed (`os.Remove` follows POSIX semantics — removes the link, not its target) and FIFO created at the link path.
  - Non-existent parent directory → `Mkfifo` fails, error wraps the path and mentions "create fifo".
  - Existing FIFO with mode 0600 (no-op scenario): the spec says replace anyway; test asserts the FIFO is recreated (inode differs) — this guarantees stale-helper-blocked FIFOs from a prior run are unblocked for the new reader.
  - Stat the created FIFO: `os.ModeNamedPipe` is set, permission bits are exactly `0600` (accounting for umask: `syscall.Mkfifo` does NOT apply umask on all platforms; on Linux it does, so tests should chmod-verify with explicit `os.Chmod` fallback if needed — verify real-world behaviour in the test and pick whichever is correct for the CI target).
- Do NOT log inside the helper; orchestrator logs per-session failures.
- Do NOT create the parent directory — assumption is `state.EnsureDir()` was called in bootstrap before this helper runs.

**Acceptance Criteria**:
- [ ] Creates a FIFO with mode `0600` at the given path when no file exists.
- [ ] Removes and replaces any pre-existing file at the path (FIFO, regular file, or symlink).
- [ ] Ignores `ENOENT` from `os.Remove` (fresh-bootstrap case).
- [ ] Wraps `os.Remove` failures (non-ENOENT) with the path.
- [ ] Wraps `syscall.Mkfifo` failures with the path.
- [ ] Does not attempt to create the parent directory — surfaces the error from `Mkfifo` if the directory is missing.
- [ ] Re-running on an existing FIFO replaces it (inode changes) so stale-helper-held FIFOs cannot block a new reader.
- [ ] Does not log or write to stderr.

**Tests**:
- `"it creates a FIFO with mode 0600 at a fresh path"`
- `"it replaces a stale FIFO cleanly"`
- `"it replaces a regular file with a FIFO"`
- `"it replaces a symlink with a FIFO (removing the link, not the target)"`
- `"it tolerates ENOENT from os.Remove"`
- `"it wraps mkfifo errors with the path when the parent directory is missing"`
- `"it wraps os.Remove errors (non-ENOENT) with the path"`
- `"it verifies os.ModeNamedPipe is set on the created file"`
- `"it recreates (new inode) even when the existing file is already a FIFO with mode 0600"`

**Edge Cases**:
- `os.Remove` returns `ENOENT` on a fresh bootstrap — ignored. Any other error (EPERM, EACCES) is wrapped and propagated.
- Pre-existing regular file at the path — must be removed and replaced; users who manually created such a file see their file disappear on the next skeleton restore (documented as "state directory is Portal-owned").
- Pre-existing FIFO from a prior crashed bootstrap where the helper is still blocked in its `open(O_RDONLY)` — removing and recreating the path unblocks that helper's `open` with an `ENOENT` on its next access, or the helper may already be blocked and the FIFO inode change doesn't affect its existing fd. Helpers have a 3s timeout anyway (task 3-9), so eventually they clean up.
- Umask interaction: the spec says "mode `0600`"; on Linux `mkfifo(2)` applies umask (so mode may end up `0600 & ~umask`). The helper additionally calls `os.Chmod(path, 0o600)` defensively after `Mkfifo` to force the exact permission bits regardless of umask. Update the test to assert `0600` after the chmod. (Add this chmod step to the `Do` list if not already included.)
- Parent directory missing — not this helper's job to create; `Restore()` calls `state.EnsureDir()` once before the per-pane loop.

**Context**:
> Spec "Scrollback Restore Mechanics → Signal Mechanism: FIFO Per Pane → Creation (bootstrap, before creating the pane)":
> 1. `os.Remove(path)` — ignore `ENOENT`; defensive sweep of any stale FIFO from a prior crashed bootstrap or dead helper.
> 2. `syscall.Mkfifo(path, 0600)` — create the FIFO with owner-only permissions.
>
> "This defensive pattern eliminates the need for a separate stale-FIFO sweep step. Stale FIFOs only exist when no live helper holds them (helpers die with the tmux server, same lifetime as the FIFOs they block on)."
>
> Spec "Save Format & Schema → FIFO Files": "Per-pane FIFOs for hydration (`hydrate-<paneKey>.fifo`) live in the state directory during the restoration window. They are created just before pane creation, unlinked by the helper on signal (or timeout), and swept defensively by `os.Remove + syscall.Mkfifo` on each bootstrap."
>
> Spec "Scrollback Restore Mechanics → Implementation Notes": "FIFOs are POSIX primitives; `syscall.Mkfifo` is the Go entry point. Supported on Linux and macOS, consistent with Portal's existing platform targets."
>
> Task 3-12 additionally lands a state-dir sweep of orphan `hydrate-*.fifo` files at bootstrap. This task owns the per-pane primitive; task 3-12 owns the pre-loop sweep.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Scrollback Restore Mechanics → Signal Mechanism: FIFO Per Pane", "Save Format & Schema → FIFO Files".

## built-in-session-resurrection-3-3 | approved

### Task 3-3: Skeleton-create one saved session (`new-session` + `set-environment` + windows/panes with hydrate command in saved structural order)

**Problem**: The heart of skeleton restoration is building one live tmux session from one `state.Session` record — correct session name, correct per-session environment applied at the right moment, correct windows in saved order, correct panes within each window, and every pane launched with the exact `sh -c 'portal state hydrate --fifo F --file S --hook-key K; exec $SHELL'` command line. Several ordering constraints are load-bearing: `set-environment` must run *after* `new-session` but *before* any `new-window`/`split-window` so subsequent panes inherit the saved env; the per-pane hydrate command must carry the *saved* `--file` and `--hook-key` values (not live re-derivation) so base-index / pane-base-index drift between save and restore preserves both scrollback hook-up and resume-hook lookup. Window layout, active pane, zoom, and marker-setting are pulled out into task 3-4 and 3-5; this task owns the structural creation plus environment application plus hydrate-command wiring.

**Solution**: Add `func RestoreSession(client *tmux.Client, stateDir string, sess state.Session) error` in `internal/restore/session.go`. Implementation outline (subject to the `[needs-info]` resolution below — Option A, prediction-before-creation, is the working assumption):

1. Read `base-index` and `pane-base-index` server options once via `show-options -gv`, defaulting to 0 if unset. Predict the live paneKey for each saved (window, pane) position: `liveWin = base-index + savedWindowOffset`, `livePane = pane-base-index + savedPaneOffset`.
2. For every saved pane, create the FIFO at `state.FIFOPath(stateDir, livePaneKey)` *before* creating the pane (so the helper can block on `open(O_RDONLY)` immediately when it starts).
3. `tmux new-session -d -s <name> -c <root_cwd> '<hydrate cmd for window0.pane0>'` — the hydrate command embeds the **live** FIFO path, the **saved** scrollback file path (`pane.ScrollbackFile` from `sessions.json`), and the **saved** hook key (`fmt.Sprintf("%s:%d.%d", sess.Name, win.Index, pane.Index)`) — never live re-derivation for `--file` or `--hook-key`.
4. Apply `set-environment -t <name> <KEY> <VAL>` for each key/value in `sess.Environment` *after* `new-session` but *before* any `new-window` / `split-window` (load-bearing — subsequent panes inherit the saved env).
5. For each remaining window, run `tmux new-window -t <name>: -n <name> -c <cwd> '<hydrate cmd>'` then `tmux split-window` for the additional panes.
6. Return to the caller. Layout / zoom / active selection (task 3-4) and `@portal-skeleton-<paneKey>` markers via re-queried live paneKey (task 3-5) run as sequenced follow-ups orchestrated by task 3-6.

Task 3-5 is the defensive re-alignment point: it re-queries `list-panes -t <session>` after creation and verifies the prediction matched, logging a warning if not. Under Option A, the prediction is correct in every realistic scenario; the re-query is belt-and-braces only.

**[needs-info]**: Task 3-3's "predict live indices via `base-index` / `pane-base-index` server options" is a planning invention — the spec describes a re-query approach but does not mandate prediction-before-creation. The spec is compatible with three approaches:
- **Option A** (planning's working assumption): predict via `base-index` / `pane-base-index` server options before pane creation; FIFO + hydrate command use the predicted live paneKey; task 3-5 is the defensive re-alignment point.
- **Option B**: pass the saved-paneKey FIFO path to the helper; after pane creation, re-query live paneKey and either (b1) symlink live → saved or (b2) have `signal-hydrate` consult the saved paneKey from the index.
- **Option C**: decouple FIFO naming from paneKey — use a UUID stored in the index.

This decision is BLOCKED on user confirmation. Until pinned, task 3-3 (this task) and task 3-5 (drift detection vs. authoritative re-query) cannot be implemented. The Do / Acceptance / Tests sections below describe Option A; if the user picks B or C, those sections are rewritten.

**Outcome**: After `RestoreSession` returns, one live tmux session exists with the saved name, saved environment applied (inheritable by subsequent pane creation), every window in saved order, every pane in each window with its hydrate command running as the initial process blocking on a created FIFO. Task 3-4 (layout/active/zoom) and task 3-5 (live paneKey re-query + marker) run *after* this task's work completes for the session.

**Do**:
- Create `internal/restore/session.go` with:
  - `type SessionRestorer struct { Client *tmux.Client; StateDir string; Logger *log.Logger }` — `Logger` is the standard-library logger Phase 3 uses; Phase 6 task 6-2 retrofits to `*state.Logger` as part of the cross-component logger migration.
  - `func (r *SessionRestorer) PredictLiveIndices(saved state.Session) (baseIdx, paneBaseIdx int, err error)` — exported because the orchestrator in task 3-6 calls it before invoking `Restore`. Reads `@base-index` and `@pane-base-index` via `tmux.Client.GetServerOption` (or `show-options -gv` with fallback to defaults 0, 0 if unset). Predict: window_live_index[N] = baseIdx + N (0-based N across saved windows in order); pane_live_index[M] = paneBaseIdx + M within each window.
  - `func (r *SessionRestorer) Restore(sess state.Session, baseIdx, paneBaseIdx int) error` — takes the predicted indices as arguments rather than re-reading them, so task 3-6's orchestrator can pass the same `(baseIdx, paneBaseIdx)` pair to `Restore` → `ApplyWindowGeometry` → `ApplySkeletonMarkers` without three separate option reads.
  - `func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string` — returns the `sh -c '...'` invocation with every interpolated value POSIX-shell-safe. Implementation: use the "close-quote / escaped-quote / re-open-quote" pattern (`'` → `'\''`) on every argument before interpolation. The fifoPath and scrollbackAbs are paneKey-sanitized so they never contain `'`, but the hookKey carries the **raw** session name per spec "Save Format & Schema → Helper hook lookup under index drift" — session names can contain `'` (tmux permits it). A naive single-quote concatenation breaks the outer `sh -c '...'` body and either fails the helper launch or, worse, executes shell fragments from the session name. Concrete shape:
    ```go
    func quoteForSingleQuoted(s string) string {
        return strings.ReplaceAll(s, "'", `'\''`)
    }
    func (r *SessionRestorer) buildHydrateCommand(fifoPath, scrollbackAbs, hookKey string) string {
        return fmt.Sprintf("sh -c 'portal state hydrate --fifo %s --file %s --hook-key %s; exec $SHELL'",
            quoteForSingleQuoted(fifoPath),
            quoteForSingleQuoted(scrollbackAbs),
            quoteForSingleQuoted(hookKey))
    }
    ```
    The `quoteForSingleQuoted` helper is shared with task 4-3's `migrate-rename` body if that task ever needs it (it does not in v1; document the helper here).
- Flow of `Restore(sess state.Session, baseIdx, paneBaseIdx int)` (the orchestrator in task 3-6 is responsible for calling `PredictLiveIndices` once per session and threading the result into `Restore`, `ApplyWindowGeometry`, and `ApplySkeletonMarkers`):
  1. For each saved window `win` at saved position `wi` (0-based) and each saved pane `pn` at saved position `pj`:
     - `liveWin := baseIdx + wi`, `livePane := paneBaseIdx + pj`.
     - `livePaneKey := state.SanitizePaneKey(sess.Name, liveWin, livePane)`.
     - `fifoPath := state.FIFOPath(r.StateDir, livePaneKey)`.
     - `state.CreateFIFO(fifoPath)` → on error, log and return wrapped error.
     - Store `(liveWin, livePane, fifoPath)` in an in-memory plan keyed by `(wi, pj)` for use in step 2.
  2. Build hydrate command for window 0 / pane 0 using the plan and the saved `scrollback_file` + saved hook-key (`fmt.Sprintf("%s:%d.%d", sess.Name, sess.Windows[0].Index, sess.Windows[0].Panes[0].Index)`).
  3. Compute `rootCWD` = `sess.Windows[0].Panes[0].CWD`.
  4. `tmux.Client.NewSession(sess.Name, rootCWD, hydrateCmdW0P0)` — wraps `tmux new-session -d -s <name> -c <cwd>` with the command. If an existing `NewSession` method does not accept a command argument, extend `tmux.Client` with `NewSessionWithCommand(name, cwd, cmd string) error` that calls `tmux new-session -d -s <name> -c <cwd> <cmd>`.
  5. If `len(sess.Environment) > 0`: for each `(k, v)` in deterministic (sorted) order, run `tmux set-environment -t <sess.Name> <k> <v>`. Deterministic order ensures tests can assert call sequences. Add `tmux.Client.SetSessionEnvironment(session, key, value string) error` if absent. On any error here, log and continue (per spec's "degrade locally, log, continue" — a missing env key does not block window/pane creation).
  6. For remaining panes in window 0 (indices `pj = 1..len(panes)-1`): `tmux split-window -t <sess.Name>:<liveWin0> -c <pane.CWD> <hydrateCmd>`. Add `tmux.Client.SplitWindow(target, cwd, cmd string) error` if absent.
  7. For each subsequent window `wi = 1..len(windows)-1`:
     - `tmux new-window -t <sess.Name>: -n <win.Name> -c <pane0.CWD> <hydrateCmdWiP0>` (add `tmux.Client.NewWindow(target, name, cwd, cmd string) error`).
     - For each pane `pj = 1..len(panes)-1`: `tmux split-window -t <sess.Name>:<liveWin> -c <pane.CWD> <hydrateCmd>`.
  8. Return `nil`. Layout/active/zoom (task 3-4) and marker-setting (task 3-5) are the orchestrator's next step in task 3-6.
- Error handling:
  - Session creation failure: return wrapped error; `RestoreSession` has no cleanup (the orchestrator in 3-6 will see the error, log, and move to the next session).
  - `set-environment` per-key failure: log the specific key/value and continue. Partial env is better than no session.
  - `split-window` / `new-window` failure: return wrapped error; orchestrator skips the rest of this session.
  - FIFO creation failure on any pane: return wrapped error (no partial pane creation — the whole session is abandoned for this bootstrap; the next bootstrap will retry since `sessions.json` is unchanged and `has-session` will still report absent).
- Tests in `internal/restore/session_test.go` using a `tmux.Commander` mock that records every call:
  - Session with one window, one pane, empty environment: mock receives `new-session -d -s <name> -c <cwd> <cmd>` and no `set-environment` or `split-window` / `new-window`.
  - Session with one window, three panes, empty env: mock receives `new-session` + 2× `split-window -t <name>:<win0>`.
  - Session with three windows, two panes each, empty env: mock receives `new-session` (pane 0,0), `split-window` (pane 0,1), `new-window` (window 1 pane 0), `split-window` (window 1 pane 1), `new-window` (window 2 pane 0), `split-window` (window 2 pane 1) — exactly 6 pane-creations across 3 windows.
  - Session with non-empty environment: `set-environment` calls fire after `new-session` and before the first `split-window` / `new-window`, in sorted-key order.
  - Session with environment only (single-pane window): `new-session` then `set-environment` calls, in sorted order.
  - Multibyte UTF-8 session name (`"汉字-work"`): name passes through unchanged to `new-session`; scrollback file path uses sanitized paneKey; hook-key uses raw name.
  - Sanitized-collision name (`"a/b"`): live paneKey uses the hashed form (from task 2-1's sanitizer); hook-key uses raw `"a/b"`.
  - base-index=1, pane-base-index=1 predicted from `GetServerOption`: live paneKeys for window 0 / pane 0 are `<name>__1.1`, not `__0.0`. Hydrate command's `--file` still uses saved `scrollback_file` (from `sessions.json`), not the live paneKey.
  - base-index unset (option not present): defaults to 0. Asserted by mocking the option read to return an empty string.
  - Hydrate command format: snapshot-matches `sh -c 'portal state hydrate --fifo <fifoAbs> --file <scrollbackAbs> --hook-key <rawHookKey>; exec $SHELL'`.
  - `set-environment` failure for one key: logged, subsequent keys and window creation still run.
  - `split-window` failure on pane 2 of window 0: returns wrapped error; no `new-window` calls for window 1 (orchestrator abandons this session).
  - FIFO creation failure on pane 1 of window 0: returns wrapped error; `new-session` was still called (the first-pane FIFO succeeded) but no further tmux calls — task 3-6 will log and skip.

**Acceptance Criteria**:
- [ ] `RestoreSession` creates the session via `new-session -d -s <name> -c <root_cwd> <hydrate cmd>`.
- [ ] `set-environment -t <name> <k> <v>` is called for every key in `sess.Environment` *after* `new-session` and *before* any `new-window` / `split-window`.
- [ ] `set-environment` keys are applied in deterministic (sorted) order so tests can assert sequences.
- [ ] Empty `environment` map results in zero `set-environment` calls.
- [ ] Each remaining pane in window 0 is created via `split-window -t <name>:<liveWin0> -c <cwd> <hydrate cmd>`.
- [ ] Each subsequent window is created via `new-window -t <name>: -n <win.Name> -c <pane0.CWD> <hydrate cmd>` followed by N-1 `split-window` calls.
- [ ] Hydrate command for each pane is `sh -c 'portal state hydrate --fifo <live FIFO path> --file <saved scrollback_file abs> --hook-key <raw saved hook-key>; exec $SHELL'`.
- [ ] `--file` is absolute (joined with state dir) and corresponds to the saved `pane.ScrollbackFile`.
- [ ] `--hook-key` is the saved structural identifier `<raw-session-name>:<saved-window-index>.<saved-pane-index>` — NOT the live-index form and NOT the sanitized-paneKey form.
- [ ] FIFO paths use the *live* paneKey (base-index + N / pane-base-index + M predicted from saved structural position).
- [ ] Live indices are predicted by `PredictLiveIndices` (exported, callable from the task 3-6 orchestrator) which reads `tmux show-options -gv base-index` / `pane-base-index` once per session, defaulting to 0 when unset.
- [ ] `Restore(sess, baseIdx, paneBaseIdx)` accepts the predicted indices as arguments — does NOT re-read tmux options internally — so the orchestrator can pass the same pair to `ApplyWindowGeometry` and `ApplySkeletonMarkers` without redundant reads.
- [ ] `SessionRestorer` carries a `Logger *log.Logger` field (standard library logger for Phase 3; migrated to `*state.Logger` in Phase 6 task 6-2).
- [ ] Multibyte UTF-8 session names survive round-trip to `new-session` unchanged.
- [ ] Sanitized-collision session names produce hash-suffixed paneKeys (via `state.SanitizePaneKey` from task 2-1); hook-key uses the raw unsanitized name.
- [ ] `set-environment` failure for one key logs and continues to the next key; does not abort session build.
- [ ] `new-window` / `split-window` failure returns wrapped error; orchestrator (task 3-6) handles session-level degradation.
- [ ] Returns `nil` on full successful creation; return error points out the failing step.
- [ ] [BLOCKED — needs planning decision on live-index source] User pins Option A, B, or C; subsequent implementation steps are rewritten to match the chosen route.

**Tests**:
- `"it creates a single-pane session with no environment"`
- `"it creates a multi-pane single-window session using new-session + split-window"`
- `"it creates a multi-window multi-pane session using new-window and split-window"`
- `"it applies set-environment after new-session and before the first new-window"`
- `"it applies set-environment in sorted-key order"`
- `"it skips set-environment entirely for an empty environment map"`
- `"it passes the saved scrollback_file path (not live-index form) via --file"`
- `"it passes the saved hook-key (raw session:window.pane) via --hook-key"`
- `"it uses the live paneKey for the FIFO path when base-index differs"`
- `"it predicts live indices from tmux show-options -gv base-index / pane-base-index"`
- `"it defaults base-index and pane-base-index to 0 when unset"`
- `"it handles a multibyte UTF-8 session name unchanged on new-session"`
- `"it uses a hash-suffixed live paneKey for a session name that sanitizes to a collision"`
- `"it logs and continues when one set-environment call fails"`
- `"it returns a wrapped error when split-window fails mid-session"`
- `"it returns a wrapped error when CreateFIFO fails for any pane"`
- `"it builds the hydrate command in the exact sh -c form specified by the spec"`

**Edge Cases**:
- Empty `environment` map — skip `set-environment` entirely; no empty-loop, no defensive no-op.
- `set-environment` runs after `new-session` and before any `new-window` / `split-window` — absolutely load-bearing. Tests assert call-order by inspecting the Commander mock's recorded sequence.
- Multibyte UTF-8 session names pass through to tmux unchanged on the wire; scrollback paneKey is sanitized by `state.SanitizePaneKey` (task 2-1); hook-key is raw.
- Sanitized-collision names — `"a/b"` sanitizes to `"a_b-<hash>"` in the paneKey but stays `"a/b"` in the hook-key. Tests use both shapes to verify they coexist.
- base-index drift: saved = 0, live = 1 → live paneKey `<name>__1.0` is distinct from saved `<name>__0.0`. The helper's `--file` still points at the saved file `scrollback/<name>__0.0.bin`; only the FIFO path and skeleton marker (task 3-5) use live.
- `saved --file` / `--hook-key` flag values are taken from `sessions.json` (not live re-derivation) — enforced by test that asserts hydrate command strings are byte-for-byte equal when the saved index differs from the live index.
- `set-environment` per-key failure logs and continues — a single bad env key (e.g., containing a `=` in the wrong place) must not block the session build.

**Context**:
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 5. `Restore()` → Else, skeleton-create it":
> 1. For each pane: compute FIFO path; `os.Remove(path)` (ignore `ENOENT`); `syscall.Mkfifo(path, 0600)`.
> 2. `new-session -d -s <name> -c <root_cwd> "sh -c 'portal state hydrate --fifo <F> --file <scrollback> --hook-key <K>; exec $SHELL'"` for the first pane, where `<K>` is the saved structural identifier `<raw-session>:<saved-window>.<saved-pane>`.
> 3. **Apply captured session environment** before creating any additional windows/panes: for each key/value in the saved `environment` map, run `tmux set-environment -t <name> <KEY> <VAL>`. This happens **after** `new-session` but **before** any subsequent `new-window` or `split-window`, so every subsequent pane inherits the saved per-session env at creation time.
> 4. `new-window` / `split-window` for remaining windows and panes, each created with its own `hydrate` command as the pane's initial process.
>
> Spec "Save Format & Schema → Canonical paneKey (sanitization reference) → Indices used in paneKey are always *live* indices (post-restoration)": "Passing the **saved scrollback file path** directly to each helper as `--file <path>`, read from `sessions.json` at bootstrap time. The helper does not compute the path from its own environment — so it reads from the saved-indexed file regardless of any index drift. Setting `@portal-skeleton-<paneKey>` and creating `hydrate-<paneKey>.fifo` using **live** paneKey (re-queried via `list-panes` after pane creation). So the daemon's enumeration, `signal-hydrate`, and the FIFO signal path all agree on live indices."
>
> Spec "Save Format & Schema → Canonical paneKey → Helper hook lookup under index drift": "The helper is invoked with a `--hook-key \"<raw-session>:<saved-window>.<saved-pane>\"` flag populated from `sessions.json` at bootstrap. The helper uses that flag (not its own live position) to look up hooks in `hooks.json`. This preserves hooks across `base-index`/`pane-base-index` changes between save and restore — the hook stays addressable by its saved identity regardless of how live tmux has numbered the recreated pane."
>
> Spec clarification on FIFO timing: the spec says "create FIFO before pane creation" but also says FIFO paths use the live paneKey. Resolution: predict live indices from `base-index` / `pane-base-index` server options before pane creation. This task implements that prediction. Task 3-5 re-queries the actual live index after creation and verifies the prediction matched; if it didn't (extremely unlikely unless the user changed `base-index` mid-restore), task 3-5 is the defensive re-alignment point.
>
> Task 2-1 `SanitizePaneKey`, task 2-3 `state.Index/Session/Window/Pane`, task 3-2 `CreateFIFO` are all pre-requisites.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 5.", "Save Format & Schema → Canonical paneKey", "Layout Restoration → Per-Window Restoration Order", "Restore-Side Architecture → Skeleton-Eager + Scrollback-Lazy".

## built-in-session-resurrection-3-4 | approved

### Task 3-4: Apply per-window layout, active pane, and zoom with `tiled` fallback

**Problem**: After task 3-3 creates the session's structural windows/panes in saved order, each window is at tmux's default geometry — not the user's saved layout. The spec pins the replay order to (a) `select-layout "<saved>"`, (b) `select-pane -t <active>`, (c) `resize-pane -Z` if saved zoom was true. Zoom *must* come after layout because `resize-pane -Z` operates on current geometry; applying it pre-layout produces wrong results. A bad `select-layout` (corrupt saved string, unexpected pane count) must fall back to `select-layout tiled` with a warning log and continue. This is a per-window loop that belongs in its own task because (a) it is trivially unit-testable with a Commander mock, (b) the fallback branch needs independent coverage, and (c) the saved-active-pane-index → live-pane-id translation is a small self-contained concern that benefits from its own tests.

**Solution**: Add `func (r *SessionRestorer) ApplyWindowGeometry(sess state.Session) error` in `internal/restore/session.go`. For each saved window (iterating in saved order, using predicted live window indices from task 3-3's prediction): (1) call `tmux select-layout -t <name>:<liveWin> "<saved layout>"` — on error, log and fall back to `tmux select-layout -t <name>:<liveWin> tiled`; (2) call `tmux select-pane -t <name>:<liveWin>.<livePaneIdxForSavedActive>` using the live-index translation; (3) if `win.Zoomed`, call `tmux resize-pane -Z -t <name>:<liveWin>.<livePaneIdxForSavedActive>`. Returns an error only on completely-unexpected tmux failures (e.g., `select-pane` fails even on `tiled` fallback); per-window soft failures (layout fallback, zoom miss) log and continue. The active-pane translation reuses task 3-3's predicted (baseIdx, paneBaseIdx) pair — no new server-option read.

**Outcome**: After `ApplyWindowGeometry` returns, every skeleton-restored window has its saved layout applied (or `tiled` as fallback), its saved active pane selected, and its zoom state re-applied. Task 3-5 runs next to set `@portal-skeleton-<paneKey>` markers; task 3-6 orchestrates the whole pipeline (session create → geometry → markers).

**Do**:
- Add to `internal/restore/session.go`:
  - `func (r *SessionRestorer) ApplyWindowGeometry(sess state.Session, baseIdx, paneBaseIdx int) error`:
    1. For each `(wi, win)` in `sess.Windows`:
       - `liveWin := baseIdx + wi`.
       - Find the saved pane at index `win.Panes[*].Active == true`; record its saved position `savedActivePj` (0-based within `win.Panes`). If no pane is marked active, default to 0 (first pane).
       - `liveActivePane := paneBaseIdx + savedActivePj`.
       - Call `client.SelectLayout(sess.Name, liveWin, win.Layout)`. On error: log `"select-layout failed for %s:%d (%v); falling back to tiled"` and call `client.SelectLayout(sess.Name, liveWin, "tiled")`. On second failure: log `"tiled fallback also failed for %s:%d (%v)"` and continue (do NOT abort remaining windows).
       - Call `client.SelectPane(sess.Name, liveWin, liveActivePane)`. Log-on-error and continue (a selected-pane failure is cosmetic; user can re-select manually).
       - If `win.Zoomed`: call `client.ResizePaneZoom(sess.Name, liveWin, liveActivePane)`. Log-on-error and continue.
    2. Return `nil` — soft failures never propagate.
  - Add `tmux.Client.SelectLayout(session string, window int, layout string) error` if absent — wraps `tmux select-layout -t <session>:<window> <layout>`.
  - Add `tmux.Client.SelectPane(session string, window, pane int) error` if absent — wraps `tmux select-pane -t <session>:<window>.<pane>`.
  - Add `tmux.Client.ResizePaneZoom(session string, window, pane int) error` if absent — wraps `tmux resize-pane -Z -t <session>:<window>.<pane>`.
- Active pane detection: scan `win.Panes` for `p.Active == true`; record the *structural position* (0-based slice index), not `p.Index` (which is the saved tmux index — may differ from live). Translate to live via `paneBaseIdx + structuralPosition`.
- Tests in `internal/restore/session_test.go` (extending task 3-3's test file):
  - Single-pane single-window session with active pane and zoom false: `select-layout` called with saved string, `select-pane` on index 0 (or base-adjusted), no `resize-pane -Z`.
  - Multi-pane single-window session with saved active pane at structural position 2, zoom true: `select-layout` + `select-pane -t <name>:0.2` + `resize-pane -Z -t <name>:0.2`.
  - Multi-window session with different active panes per window: correct `select-pane` per window.
  - Corrupt layout string: mock returns error on first `select-layout` call, success on second (tiled); warn logged, session continues.
  - Both `select-layout` saved and `tiled` fail: both errors logged, session continues (no abort).
  - No pane marked active (all `Active: false`): default to structural position 0.
  - Single-pane window still applies `select-pane` (verifies no special-case skip for single-pane windows).
  - `resize-pane -Z` skipped when `Zoomed: false` even on a multi-pane window.
  - Zoom applied *after* layout: assert call-order via mock (layout index precedes zoom index in recorded call list).
  - base-index=1, pane-base-index=1: live coordinates adjusted throughout — `select-layout -t <name>:1`, `select-pane -t <name>:1.1`.

**Acceptance Criteria**:
- [ ] `select-layout` is called with the saved layout string for every window.
- [ ] On `select-layout` error, falls back to `select-layout tiled` with a warning log and continues.
- [ ] Both-paths-fail: logs both errors and continues to next window — no abort.
- [ ] `select-pane` is called for every window using live-index coordinates.
- [ ] `resize-pane -Z` is called on the active pane only when `Zoomed: true`.
- [ ] Zoom is applied strictly *after* layout (call-order asserted in tests).
- [ ] Default structural-position-0 is used when no pane is marked active.
- [ ] Live indices for `select-pane` / `resize-pane -Z` come from the same `(baseIdx, paneBaseIdx)` pair task 3-3 predicted.
- [ ] Single-pane windows still receive `select-pane` (no special-case skip).
- [ ] `Zoomed: false` always skips `resize-pane -Z` even on multi-pane windows.
- [ ] Function never propagates per-window errors to caller — all soft failures are logged and swallowed.

**Tests**:
- `"it applies the saved layout to every window"`
- `"it calls select-pane with the live pane index for the saved active pane"`
- `"it applies resize-pane -Z after layout when zoomed is true"`
- `"it skips resize-pane -Z when zoomed is false"`
- `"it falls back to tiled when select-layout fails"`
- `"it logs and continues when both select-layout and tiled fallback fail"`
- `"it defaults to structural position 0 when no pane is marked active"`
- `"it applies layout then zoom in that exact order"`
- `"it handles a single-pane window by calling select-pane on the only pane"`
- `"it uses live indices derived from base-index and pane-base-index"`
- `"it does not abort the remaining windows when one window fails"`

**Edge Cases**:
- Corrupt layout string — tmux's `select-layout` returns non-zero. Fall back to `tiled`; log the original error and the fallback attempt.
- Zoomed window with `select-layout` fallback: after `tiled` replaces the layout, `resize-pane -Z` on the active pane is still attempted — this is a well-defined operation on a tiled layout (zooms the one active pane), consistent with user expectation of "the saved active pane is maximised."
- Saved `active: true` appears on multiple panes (corrupt input): use the first structural position where `Active == true` — `state.DecodeIndex` does not validate this invariant, so the restorer must be tolerant.
- No pane marked active: default to structural position 0. Equally valid; user never sees an error.
- Single-pane window: `select-pane` is still called. tmux tolerates `select-pane` on an already-selected pane.
- `Zoomed: false` with `select-layout` fallback: no zoom — matches user state (layout was broken, at least geometry is visible).

**Context**:
> Spec "Layout Restoration → Per-Window Restoration Order":
> 1. Create the window (`new-window` — the first pane is created implicitly).
> 2. Create remaining panes via `split-window` to reach the saved pane count. Direction arguments are arbitrary — the next step rearranges.
> 3. `select-layout "<saved layout string>"` — tmux parses the string and fits the panes to the saved geometry.
> 4. `select-pane -t <saved active pane index>` — set the active pane within the window.
> 5. `resize-pane -Z` on the active pane if `window_zoomed_flag` was true at save time.
>
> "Zoom **must come after** layout. `resize-pane -Z` operates on the current layout geometry, and applying it before `select-layout` would produce incorrect results. This ordering matches tmux-resurrect's proven approach."
>
> Spec "Layout Restoration → Pane-Count Mismatch / `select-layout` Failure":
> 1. Log a warning to `portal.log` identifying the session, window, and the mismatch.
> 2. Fall back to `select-layout tiled` — tmux's built-in auto-balanced tiled layout. Panes are visible in a sane default arrangement.
> 3. Continue restoring the remaining windows and sessions. One broken layout does not block other restorations.
>
> Spec "Failure Modes & Recovery → Consolidated Failure-Handling Table → `select-layout` fails": "Log warning, fall back to `select-layout tiled`. Panes visible in a sane default arrangement; structure approximated."
>
> Task 3-3 produced the predicted (baseIdx, paneBaseIdx) pair; this task consumes it for live-index translation without re-reading tmux server options.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Layout Restoration → Per-Window Restoration Order", "Layout Restoration → Pane-Count Mismatch / `select-layout` Failure", "Failure Modes & Recovery → Consolidated Failure-Handling Table".

## built-in-session-resurrection-3-5 | approved

### Task 3-5: Re-query live paneKey after pane creation and set `@portal-skeleton-<paneKey>` via `set-option -s`

**Problem**: Task 3-3 *predicts* live indices from `base-index` / `pane-base-index` server options in order to create FIFOs and build hydrate command-line arguments. Those predictions are correct under normal conditions but brittle to any assumption violation (user changes `base-index` mid-restore, hooks that renumber, out-of-order `split-window` placement). The spec is explicit: the live paneKey is what the daemon's `show-options -sv` enumeration agrees on, and the marker is `@portal-skeleton-<paneKey>` set via `set-option -s` (server scope — load-bearing; `-g` would be wrong because the daemon enumerates server-scope options). This task re-queries the *actual* live pane list via `tmux list-panes -t <session>` after all panes are created, maps each live pane back to its saved structural position, and sets the marker against the live paneKey. If the prediction in task 3-3 matched (the common case), the live and predicted paneKeys are identical and FIFOs at the predicted path are already at the right live-path. If the prediction was wrong (pathological case), the mismatch is detected and the task logs a clear warning plus still sets the marker at the live paneKey so signal-hydrate and the daemon's save-skip logic continue to work.

**Solution**: Add `func (r *SessionRestorer) ApplySkeletonMarkers(sess state.Session, predictedBase, predictedPaneBase int) error` in `internal/restore/session.go`. For each saved session: (1) call `tmux list-panes -a -t <session> -F '#{window_index}:#{pane_index}'` to enumerate every live window/pane in structural order; (2) compare the live count to the saved count (sanity check — log a warning if they differ but do not abort); (3) pair each live pane with its saved structural position (assumption: tmux returns panes in window-index, pane-index order, so the structural pairing is the sorted live list's first entry = saved (0,0), second = saved (0,1), etc.); (4) for each pair, compute `livePaneKey := state.SanitizePaneKey(sess.Name, liveWin, livePane)` and call `tmux set-option -s @portal-skeleton-<livePaneKey> 1`; (5) if any `livePaneKey` differs from the predicted paneKey used for FIFO creation in task 3-3, log a warning naming both (FIFO signal may race; hydrate helper may time out and re-signal works on next attach). Marker setting uses `-s` flag (server scope); `-g` is explicitly rejected.

**Outcome**: After `ApplySkeletonMarkers` returns, every skeleton-restored pane has a server-scope option `@portal-skeleton-<livePaneKey>` set to `1`. The daemon's `show-options -sv` enumeration in task 2-11 sees all markers on its next tick and skips those panes from capture. `signal-hydrate` (task 3-11) enumerates these markers on client-attached / client-session-changed and writes to each matching FIFO. Task 3-6 orchestrates session creation → geometry → markers in that order.

**Do**:
- Add to `internal/restore/session.go`:
  - `func (r *SessionRestorer) ApplySkeletonMarkers(sess state.Session, predictedBase, predictedPaneBase int) error`:
    1. `livePanes, err := client.ListPanesInSession(sess.Name)` → wraps `tmux list-panes -t <session> -F '#{window_index}:#{pane_index}'`. On error: log and return the error — the orchestrator in 3-6 will decide session-level handling.
    2. Sort `livePanes` by (window_index, pane_index). tmux usually returns them in this order already, but do not assume.
    3. Sanity check: total saved pane count = sum over `sess.Windows[*]` of `len(window.Panes)`. If `len(livePanes) != savedCount`, log `"live pane count %d differs from saved %d for session %s"` but continue.
    4. Iterate paired (savedPane, livePane) — zip by structural order (saved windows in order, panes in each window in order):
       - `livePaneKey := state.SanitizePaneKey(sess.Name, livePane.Window, livePane.Pane)`.
       - `predictedPaneKey := state.SanitizePaneKey(sess.Name, predictedBase+wi, predictedPaneBase+pj)` (where wi, pj are the saved structural position).
       - If `livePaneKey != predictedPaneKey`, log `"paneKey drift for %s saved (w%d,p%d): predicted %s, live %s"`.
       - Call `client.SetServerOption("@portal-skeleton-"+livePaneKey, "1")` — uses `-s` flag (server scope). If the existing `tmux.Client` method defaults to `-s` (it should, per Phase 1 decisions), use it; otherwise add `SetServerOption(name, value string) error` that wraps `tmux set-option -s <name> <value>`.
    5. Return `nil`. Marker-set failures per-pane are logged inside the helper and do not abort the loop.
- Add `tmux.Client.ListPanesInSession(session string) ([]PaneCoord, error)` if absent — wraps `tmux list-panes -t <session> -F '#{window_index}:#{pane_index}'` and parses lines into `PaneCoord{Window, Pane int}`.
- Tests in `internal/restore/session_test.go`:
  - Happy path: 2 windows × 2 panes; prediction matches live; 4 `set-option -s @portal-skeleton-<key>` calls with live paneKeys; no drift warning.
  - base-index drift: predicted 0, live 1 (user changed `base-index` between predict and list-panes — pathological but testable). Marker calls use live paneKey `<name>__1.0` etc.; drift warning logged with both names.
  - live pane count < saved (one pane failed to create earlier): sanity warning logged; only live panes get markers.
  - live pane count > saved (user manually split a pane before marker step): sanity warning logged; all live panes get markers (extras included — harmless; daemon will skip them until clear).
  - `list-panes` fails: error returned; orchestrator 3-6 handles.
  - `set-option -s` for one pane fails: error logged, remaining panes still marked.
  - Server scope assertion: test verifies the Commander call includes `-s` flag and NOT `-g`.
  - Sanitized-collision session (`"a/b"`): live paneKey is hash-suffixed; `@portal-skeleton-<hashedKey>` is what gets set.
  - `set-option` call argument is literally `@portal-skeleton-<paneKey>` and value `"1"` (not empty, not boolean form — spec's exact shape).

**Acceptance Criteria**:
- [ ] `list-panes -t <session>` is called once per session to enumerate live panes.
- [ ] Live panes are paired with saved panes by structural order (window-index then pane-index sort).
- [ ] Each pair computes `livePaneKey` via `state.SanitizePaneKey(sess.Name, liveWin, livePane)`.
- [ ] `tmux set-option -s @portal-skeleton-<livePaneKey> 1` is called for each pane.
- [ ] `-s` flag is used (server scope); `-g` is never used for skeleton markers.
- [ ] Live vs predicted paneKey mismatch logs a warning with both names but does not abort.
- [ ] Live vs saved pane count mismatch logs a warning but does not abort.
- [ ] Per-pane `set-option` failure logs and continues; does not abort the loop.
- [ ] `list-panes` failure returns wrapped error to the orchestrator.
- [ ] Marker value is the literal string `"1"` — not empty, not a boolean.

**Tests**:
- `"it calls list-panes -t <session> and sets one marker per live pane"`
- `"it uses the live paneKey (not predicted) when prediction and live disagree"`
- `"it logs a drift warning when predicted and live paneKeys differ"`
- `"it logs a sanity warning when live pane count differs from saved"`
- `"it uses the -s flag for set-option (server scope), never -g"`
- `"it continues setting markers for remaining panes when one set-option fails"`
- `"it returns an error when list-panes fails"`
- `"it sets the marker value to the literal string '1'"`
- `"it applies markers for a sanitized-collision session using the hashed paneKey"`
- `"it enumerates live panes sorted by (window_index, pane_index)"`

**Edge Cases**:
- base-index / pane-base-index changed between task 3-3's predict and task 3-5's list-panes (pathological race): live paneKey wins; predicted FIFO path may be stale; drift warning logged; hydrate helper will time out on the stale FIFO and the next attach re-signals (per signal-hydrate idempotency in task 3-11).
- Marker name uses `-s` (server scope), not `-g`. Empty-string value would NOT remove (`-u` is required — helper uses that in task 3-8). Here we're setting, not unsetting; the value is `"1"`.
- Live pane count equals saved pane count — sanity check passes, no warning.
- Live pane count differs — sanity warning logged; still set markers on live panes (excess or deficient).
- Markers set per-pane before any signal path can fire: this task runs inside `Restore()`'s per-session loop while `@portal-restoring` is still set (cleared only in step 6 of bootstrap, task 3-7). `signal-hydrate` doesn't fire until `client-attached` — well after bootstrap returns.
- Live paneKey differs from saved paneKey: both coexist until first post-hydration capture. The saved-indexed scrollback `.bin` is untouched because the daemon's `show-options -sv` enumeration finds the marker at the *live* paneKey but the scrollback file is at the saved paneKey path — the daemon's per-pane capture writes under live paneKey (task 2-9 writes at live paneKey, since capture happens via `list-panes -a` live enumeration). GC in task 2-10 removes the stale saved-indexed file after the first post-hydration capture. This task only sets markers; the drift-to-convergence happens downstream.

**Context**:
> Spec "Restore-Side Architecture → Marker Coordination → `@portal-skeleton-<paneKey>`": "Set by: skeleton-restore, on each pane it creates, via `tmux set-option -s @portal-skeleton-<paneKey> 1` — the `-s` flag is load-bearing (server scope, matching the daemon's `show-options -sv` enumeration). Key is the structural position `session:window.pane`."
>
> Spec "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 5. → 6.": "For each created pane: `tmux set-option -s @portal-skeleton-<paneKey> 1` — the `-s` flag targets the server-option scope (load-bearing; matches the daemon's `show-options -sv` enumeration). Server-option scope is volatile — markers clear automatically on tmux server restart."
>
> Spec "Save Format & Schema → Canonical paneKey (sanitization reference) → Indices used in paneKey are always *live* indices (post-restoration)": the saved scrollback file coexists with the live paneKey until the first post-hydration capture writes the scrollback under the live paneKey and GC removes the stale saved file.
>
> Spec "Save Format & Schema → Index Semantics and base-index / pane-base-index": "On restore, Portal creates windows and panes in saved-structural order, but **does not assume the created tmux indices match the saved indices**. After creating each window via `new-window` and each pane via `split-window`, Portal re-queries `list-panes -t <session>` to map saved-structure position → actual live tmux index."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Restore-Side Architecture → Marker Coordination", "Bootstrap Flow → 5. → 6.", "Save Format & Schema → Canonical paneKey", "Save Format & Schema → Index Semantics and base-index / pane-base-index".

## built-in-session-resurrection-3-6 | approved

### Task 3-6: Implement top-level `Restore()` orchestrator with per-session error isolation

**Problem**: Tasks 3-1 through 3-5 land single-session primitives (read index, create FIFO, create session, apply geometry, set markers). Step 5 of `PersistentPreRunE` calls one `Restore()` entry point — not six. That entry point must iterate every saved session, skip live-named sessions, skeleton-create each missing one, and handle per-session failures in isolation so one broken session never blocks the remaining ones. The spec enumerates failure modes at the top level: missing `sessions.json` is a silent no-op; unparseable `sessions.json` is a one-line stderr warning + log + skip-all; a session whose `panes` array is empty is a per-session log + skip (Portal refuses to create a session whose pane topology cannot be specified); any per-session creation error logs and continues to the next session; `_`-prefixed session names in the index are skipped defensively (they are Portal internals and should never appear, but be robust). This orchestrator task is where those cross-cutting policies live, decoupled from the single-session primitives they compose.

**Solution**: Create `internal/restore/restore.go` with `type Orchestrator struct { Client *tmux.Client; StateDir string; Logger *log.Logger; Stderr io.Writer }` and `func (o *Orchestrator) Restore() error`. Flow: (1) resolve state dir via `state.EnsureDir()`; (2) call `state.ReadIndex(dir)` from task 3-1 — on `skip=true, err=nil` return nil; on `skip=true, err!=nil` log the error, emit the spec's one-line stderr warning `"Portal state file is corrupt — restoration skipped.\nCheck `portal state status` or ~/.config/portal/state/portal.log."`, return nil; (3) read live sessions via `tmux list-sessions -F '#{session_name}'` into a set; (4) for each `sess` in `idx.Sessions`: (a) if `strings.HasPrefix(sess.Name, "_")` skip with log; (b) if session name is in live set, skip (no log — quiet no-op, this is the common steady-state case); (c) if `len(sess.Windows) == 0` or any window has `len(Panes) == 0`, log warning and skip; (d) else `SessionRestorer{Client, StateDir}.Restore(sess)` + `ApplyWindowGeometry(sess, base, paneBase)` + `ApplySkeletonMarkers(sess, base, paneBase)` — errors from any phase log per-session and continue. No fatal error propagates out of the orchestrator; `Restore()` returns `nil` even when every session failed (bootstrap proceeds). Task 3-7 wraps this with the `@portal-restoring` set/unset discipline.

**Outcome**: One callable entry point used by Phase 5's bootstrap integration: `err := restore.New(client, stateDir, logger, stderr).Restore()`. Every saved session is either skipped (already live, name-prefixed `_`, empty panes) or skeleton-restored (create → geometry → markers). Per-session failures are logged with session-name context; bootstrap continues. Unit tests cover the full matrix: missing index, corrupt index, empty index, single session happy path, live-skip, empty-panes skip, `_`-prefix skip, one-session-errors-others-continue.

**Do**:
- Create `internal/restore/restore.go`:
  ```go
  package restore

  import (
      "fmt"
      "io"
      "log"
      "strings"

      "github.com/leeovery/portal/internal/state"
      "github.com/leeovery/portal/internal/tmux"
  )

  type Orchestrator struct {
      Client  *tmux.Client
      StateDir string
      Logger  *log.Logger
      Stderr  io.Writer // task 3-7 / observability may buffer instead of writing directly
  }

  func New(c *tmux.Client, stateDir string, logger *log.Logger, stderr io.Writer) *Orchestrator {
      return &Orchestrator{Client: c, StateDir: stateDir, Logger: logger, Stderr: stderr}
  }

  func (o *Orchestrator) Restore() error {
      idx, skip, readErr := state.ReadIndex(o.StateDir)
      if skip {
          if readErr != nil {
              o.Logger.Printf("restore: %v", readErr)
              fmt.Fprintln(o.Stderr, "Portal state file is corrupt — restoration skipped.")
              fmt.Fprintln(o.Stderr, "Check `portal state status` or ~/.config/portal/state/portal.log.")
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
              // continue anyway — markers may still be useful
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
  - Corrupt `sessions.json` → one log line + exactly the two-line stderr warning, returns nil, no session-creation tmux calls.
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
- [ ] Unparseable `sessions.json` logs the parse error to `portal.log` and emits exactly the two-line stderr warning specified by the spec.
- [ ] `_`-prefixed session names in the index are defensively skipped with a log.
- [ ] Live session (name exists in `list-sessions`) is silently skipped (no log — steady-state case).
- [ ] Session with zero windows is skipped with a log.
- [ ] Session with any zero-pane window is skipped with a log.
- [ ] Each surviving session runs create → geometry → markers in that order.
- [ ] Error in `Restore` (create-session step) logs with session name and continues to the next session.
- [ ] Error in `ApplyWindowGeometry` logs and still runs `ApplySkeletonMarkers` (markers are independently useful for signal-hydrate coverage).
- [ ] Error in `ApplySkeletonMarkers` logs and continues to the next session.
- [ ] Orchestrator always returns `nil` (bootstrap never aborts on restore failure).
- [ ] Stderr writes use the injected `Stderr` writer (task 3-7 / Phase 6 may buffer in TUI path).

**Tests**:
- `"it is a silent no-op when sessions.json is absent"`
- `"it emits the corrupt-sessions stderr warning and logs the error when JSON is unparseable"`
- `"it does nothing beyond list-sessions when the index is empty"`
- `"it skeleton-restores a single missing session end-to-end"`
- `"it silently skips sessions whose name is already live (no log)"`
- `"it defensively skips underscore-prefixed session names in the index"`
- `"it logs and skips sessions with zero windows"`
- `"it logs and skips sessions with any zero-pane window"`
- `"it isolates per-session errors (one fails, next continues)"`
- `"it continues to ApplySkeletonMarkers after ApplyWindowGeometry fails for a session"`
- `"it logs and returns nil when list-sessions itself fails"`
- `"it always returns nil even when every session errors"`

**Edge Cases**:
- Missing index → common first-ever-bootstrap case; must be silent.
- Unparseable index → rare but observable; one log, one two-line stderr warning, no partial restore.
- `_`-prefixed session in the index → spec says daemon filters these from capture, so they should never appear; but be defensive and skip.
- Live-session name collision → already-live sessions are authoritative per spec; silent skip (no log) avoids noise on every `portal open`.
- `panes` empty array per spec: "Portal never creates a session whose pane topology cannot be specified." Log, skip, continue.
- Per-session error isolation: ensures one corrupt saved session does not block a user's remaining 9 from restoring.
- `list-sessions` failure: unusual but possible. Log, return nil — bootstrap continues with whatever live tmux has.
- `sessions.json` present but schema `version` is future: handled by task 3-1's `ReadIndex`, which returns `skip=true, err!=nil`. This orchestrator logs and emits the same stderr warning (reuses the "corrupt" branch — the user-facing distinction is not material for v1).

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
> Spec "Observability & Diagnostics → Proactive Health Signals → Exception: genuinely broken states detected during `PersistentPreRunE`": the corrupt-sessions warning text is `"Portal state file is corrupt — restoration skipped.\nCheck `portal state status` or ~/.config/portal/state/portal.log."` — use it verbatim.
>
> Spec "Failure Modes & Recovery → Consolidated Failure-Handling Table → `sessions.json` corrupt / unparseable": "Log warning, emit one-line stderr warning (see Observability), skip restoration entirely, continue bootstrap. User sees an empty picker. Diagnosable via log file or file inspection. Next successful save overwrites with valid content."
>
> Spec "Save-Side Architecture: Execution Model → Session Visibility and Filtering": "Portal filters sessions whose names begin with `_` ... from: `sessions.json` capture (the save process skips `_*` sessions when enumerating live state)." The filter applies on the save side (task 2-8), but defensive skipping on the restore side is belt-and-braces.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Restore-Side Architecture → Restoration Trigger", "Bootstrap Flow (Integrated) → `PersistentPreRunE` Sequence → 5.", "Observability & Diagnostics → Proactive Health Signals", "Failure Modes & Recovery".

## built-in-session-resurrection-3-7 | approved

### Task 3-7: Wrap `Restore()` with `@portal-restoring` set-before / clear-after using `set-option -s`

**Problem**: `Restore()` fires `session-created`, `window-linked`, `window-layout-changed`, `pane-focus-out` in bursts as it creates skeleton sessions. Phase 1 registered those events to invoke `portal state notify`, which bumps the dirty flag. Phase 2's daemon checks `@portal-restoring` at the top of every tick and skips capture entirely while set. Without the wrapper enforced in this task, the daemon would race the restore: the first post-bootstrap tick could fire mid-skeleton-build, catching half-constructed state. Per spec, `@portal-restoring` is set *before* `_portal-saver` is even created in step 4 of bootstrap, so the daemon's very first tick is already suppressed. Clearing is equally critical: if `Restore()` panics mid-flight and the marker is never cleared, the daemon stays silent forever (until server restart clears the volatile server-option). This task owns the set-before / defer-clear discipline around the orchestrator, plus the fatal-error handling when `set-option -s` fails (per spec: `@portal-restoring set-option failure` is a fatal bootstrap error, not a soft one).

**Solution**: Add `SetRestoring()` and `ClearRestoring()` as standalone primitives on `Orchestrator` in `internal/restore/restore.go` (or a thin wrapper file `restore_marker.go`). Each is a single-purpose method: `SetRestoring` calls `client.SetServerOption("@portal-restoring", "1")` and returns the wrapped error; `ClearRestoring` calls `client.UnsetServerOption("@portal-restoring")` and returns the wrapped error. Do NOT introduce a composite `RestoreWithMarker()` wrapper — the spec's bootstrap flow interleaves `_portal-saver` creation between the set and clear (step 3 → step 4 → step 5 → step 6), and a composite wrapper would mask that ordering invariant for any future caller. Phase 5's bootstrap orchestrator (task 5-2) calls the primitives directly in spec order. Task 3-13's integration test calls the same primitives directly with explicit set / restore / clear lines — the extra line is worth the documentation value.

**Outcome**: `RestoreWithMarker()` guarantees the marker is set before any session-building tmux call and cleared when restore returns (even on panic via defer). `SetRestoring` / `ClearRestoring` are independently callable for Phase 5's bootstrap. Set failure is fatal — caller propagates the error so the user sees the spec-mandated fatal stderr line. Clear uses `-su` (remove option), not empty-string assignment. The marker is idempotent-set (already-set from a prior crashed bootstrap produces no error; tmux's `set-option` overwrites).

**Do**:
- Add to `internal/restore/restore.go` (or new `restore_marker.go`):
  ```go
  func (o *Orchestrator) SetRestoring() error {
      if err := o.Client.SetServerOption("@portal-restoring", "1"); err != nil {
          return fmt.Errorf("set @portal-restoring: %w", err)
      }
      return nil
  }

  func (o *Orchestrator) ClearRestoring() error {
      if err := o.Client.UnsetServerOption("@portal-restoring"); err != nil {
          return fmt.Errorf("unset @portal-restoring: %w", err)
      }
      return nil
  }
  ```
- Do NOT introduce a composite `RestoreWithMarker()` wrapper. The spec's bootstrap flow interleaves `_portal-saver` creation between set and clear (step 3 → step 4 → step 5 → step 6); a composite would mask that ordering invariant for any future caller. Phase 5's bootstrap orchestrator (task 5-2) calls the primitives directly. Task 3-13's integration test calls the primitives directly with explicit set / restore / clear lines.
- Add `tmux.Client.UnsetServerOption(name string) error` wrapping `tmux set-option -su <name>` (the `-u` flag is load-bearing — empty-string assignment leaves the option set). `SetServerOption(name, value string) error` wraps `tmux set-option -s <name> <value>`. Match Phase 1 / Phase 2's existing patterns; if those methods already exist on `tmux.Client`, reuse them verbatim.
- Tests in `internal/restore/restore_test.go`:
  - `SetRestoring` calls `SetServerOption("@portal-restoring", "1")` and propagates the error.
  - `ClearRestoring` calls `UnsetServerOption("@portal-restoring")` and propagates the error.
  - Idempotent set: already-set marker from a prior bootstrap — `SetServerOption("@portal-restoring", "1")` succeeds regardless (tmux overwrites); test by pre-populating the marker state in the mock and verifying no-error path.
  - `-s` / `-u` flag discipline: the underlying `SetServerOption` uses `-s` (not `-g`); `UnsetServerOption` uses `-u` (not empty-string `set-option -s @portal-restoring ""`). Assert via mock call records.

**Acceptance Criteria**:
- [ ] `SetRestoring` calls `tmux set-option -s @portal-restoring 1`.
- [ ] `ClearRestoring` calls `tmux set-option -su @portal-restoring` (uses `-u` flag — empty-string assignment is forbidden).
- [ ] Set failure returns wrapped error to caller — fatal-bootstrap-error path per spec.
- [ ] Clear failure returns wrapped error to caller — caller (Phase 5 task 5-2) decides handling per its soft/fatal contract.
- [ ] No composite `RestoreWithMarker()` wrapper exists — only the two primitives.
- [ ] Set is idempotent — already-set marker from prior crashed bootstrap does not error.
- [ ] Marker uses `-s` flag (server scope), matching daemon's `show-options -sv` enumeration in task 2-11.
- [ ] Clear uses `set-option -su`, never `set-option -s @portal-restoring ""` (empty string would leave the option present).

**Tests**:
- `"SetRestoring calls set-option -s @portal-restoring 1"`
- `"ClearRestoring calls set-option -su @portal-restoring"`
- `"SetRestoring wraps the underlying tmux error"`
- `"ClearRestoring wraps the underlying tmux error"`
- `"it uses -s for set (server scope) and -u for unset (option removal)"`
- `"it tolerates a pre-existing @portal-restoring marker (idempotent set)"`
- `"it never issues set-option with an empty-string value"`

**Edge Cases**:
- Set failure is fatal per spec "Observability & Diagnostics → Fatal Bootstrap Errors": `@portal-restoring set-option failure` → log, emit stderr, exit non-zero. This task returns the error; Phase 5's PersistentPreRunE integration treats it as fatal.
- Clear failure is non-fatal but observable — log only. The marker being stuck means the daemon stays silent until the next tmux server restart (volatile server-option). Not ideal, but better than aborting bootstrap at the end of otherwise-successful restore.
- Defer semantics: Go's `defer` fires even on panic. Test asserts this explicitly by injecting a panicking mock.
- Idempotent set: a prior bootstrap crashed after setting the marker but before clearing (server still alive). The marker is already `1`. `SetServerOption("@portal-restoring", "1")` is a no-op (tmux accepts the overwrite silently). Task 3-6 runs normally; the eventual clear removes the stuck marker.
- Marker is server-scope (volatile): cleared automatically on tmux server restart. So a crashed-mid-bootstrap that also crashes tmux self-heals on next restart.
- `-su` is load-bearing (per spec "Restore-Side Architecture → Marker Coordination → `@portal-skeleton-<paneKey>`"): the -u flag removes the option. Empty-string assignment leaves it present with an empty value, which the daemon's show-options -sv enumeration still reports as "present".

**Context**:
> Spec "Restore-Side Architecture → Marker Coordination → `@portal-restoring`":
> - Set by: bootstrap, at the start of the skeleton-restore phase.
> - Unset by: bootstrap, after skeleton-restore completes.
> - Semantic: "bootstrap is mid-skeleton-build; save captures would see half-built state."
> - Effect on save: the hosted daemon's tick loop honours this marker (skip the entire tick if set).
>
> Spec "Bootstrap Flow (Integrated) → Ordering Rationale": "The critical ordering — `@portal-restoring` is set in step 3 **before** `_portal-saver` is created in step 4 — exists because step 4 fires `session-created`, which the hook pipeline would otherwise use to dirty the flag. Without `@portal-restoring` set first, the daemon's first tick could attempt to capture while the restoration is still building structure."
>
> Spec "Observability & Diagnostics → Fatal Bootstrap Errors": "`@portal-restoring` set-option fails: same as `set-hook` failure. log, emit one-line stderr warning if on CLI path; on TUI path, dismiss loading page cleanly, emit error, exit non-zero."
>
> Spec "Restore-Side Architecture → Marker Coordination → `@portal-skeleton-<paneKey>`": "Unset by: the hydrate helper, via `tmux set-option -su @portal-skeleton-<paneKey>` — the `-u` flag **removes** the user option, so the daemon's enumeration sees it gone. Assigning an empty string (`set-option -s <key> \"\"`) does *not* remove the option and would be a bug ... Everywhere this spec mentions clearing the marker ... the intended semantics is `set-option -su`." The same rule applies to `@portal-restoring`.
>
> Phase 5 will split set/clear across steps 3 and 6 of `PersistentPreRunE` with `_portal-saver` creation in between; this task exposes only the two primitives so the bootstrap orchestrator (and task 3-13's integration test) call them directly in spec order.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Restore-Side Architecture → Marker Coordination → `@portal-restoring`", "Bootstrap Flow (Integrated) → Ordering Rationale", "Observability & Diagnostics → Fatal Bootstrap Errors".

## built-in-session-resurrection-3-8 | approved

### Task 3-8: Implement `portal state hydrate` signal-arrived success path

**Problem**: Every skeleton-restored pane runs `sh -c 'portal state hydrate --fifo F --file S --hook-key K; exec $SHELL'` as its initial process. The helper's job on the *signal-arrived* path is a precise sequence the spec pins down in detail: (1) open the FIFO `O_RDONLY` blocking with a 3-second timeout; (2) read a single byte; (3) close and `os.Remove` the FIFO; (4) emit the reset preamble `\033[?25h\033[?1049l\033[0m` (cursor visible + exit alt-screen + SGR reset); (5) copy the scrollback file's bytes to stdout — streamed, not buffered in memory for large files; (6) emit the reset postamble `\033[?25h\033[?1049l\033[0m\r\n`; (7) sleep 100ms (settle against tmux's VT-parser lag); (8) clear the skeleton marker via `tmux set-option -su @portal-skeleton-<livePaneKey>` (uses `-u` — empty-string assignment would NOT remove); (9) `exec $SHELL` (Phase 4 will replace this with `exec sh -c 'HOOK; exec $SHELL'` when a hook matches `--hook-key`). This task owns the signal-arrived path; task 3-9 owns the 3-second timeout path; task 3-10 owns the scrollback-file-missing path. Splitting keeps each failure mode's semantics independently testable.

**Solution**: Replace the stub `RunE` in `cmd/state_hydrate.go` (landed in task 1-1) with the full signal-arrived flow. Non-signal paths (timeout, file missing) are implemented in tasks 3-9 / 3-10 — this task lands the success branch and leaves stubs for the others that default to `exec $SHELL` with a log. The live paneKey used for the marker unset is derived from the `--fifo` flag: the FIFO path is `~/.config/portal/state/hydrate-<livePaneKey>.fifo`, so `livePaneKey` is the basename with prefix/suffix stripped. Alternative: pass `--live-pane-key` as a fourth flag to the helper. Re-reading the spec, the helper unsets `@portal-skeleton-<paneKey>` where `paneKey` is derivable from the FIFO name — Portal's helper knows its own FIFO path via the flag. Add a small `paneKeyFromFIFOPath(fifoAbs string) string` utility that `strings.TrimPrefix(basename, "hydrate-")` + `strings.TrimSuffix(_, ".fifo")`. Hook firing (step 9) is deferred to Phase 4; this task ends with `exec $SHELL`.

**Outcome**: `portal state hydrate --fifo /path/to/hydrate-work__0.0.fifo --file /path/to/scrollback/work__0.0.bin --hook-key work:0.0`, run as a pane's initial process, blocks on the FIFO, receives a byte from `signal-hydrate` (task 3-11), dumps scrollback with ANSI preserved, sleeps 100ms, clears the skeleton marker, and `exec`s `$SHELL` so the user lands in their shell with their pre-reboot scrollback intact. Unit tests with in-process FIFOs (or mocked reader/writer) cover the full sequence; task 3-13 integration-tests the end-to-end behaviour on a real tmux socket.

**Do**:
- Implement `cmd/state_hydrate.go`:
  - Extract the core logic into a testable function: `func runHydrate(cfg HydrateConfig) error` where `HydrateConfig` bundles the flags + injectable dependencies (`Stdout io.Writer`, `Client tmuxClient`, `Logger *log.Logger`, `FIFO fifoOpener` — an interface for opening the FIFO with timeout — so tests can substitute a fake reader).
  - `runHydrate` for this task implements ONLY the signal-arrived path and leaves TODO-stubs for timeout (task 3-9) and file-missing (task 3-10). Call sites are shaped so task 3-9 and 3-10 fill in branches without rewriting the signal-arrived flow.
  - Flow:
    1. Compute `livePaneKey := paneKeyFromFIFOPath(cfg.FIFO)`.
    2. Open FIFO `O_RDONLY` via a goroutine, with a 3s `time.After` race. For this task, only the signal-arrived branch runs fully; timeout branch delegates to a stub `handleTimeout()` that task 3-9 replaces.
    3. On signal arrival:
       - Read 1 byte (or up to 1 byte — content of the byte is irrelevant, any successful read completes the signal).
       - Close the FIFO file.
       - `os.Remove(cfg.FIFO)` — unlink; ignore `ENOENT`.
       - Emit preamble bytes `\033[?25h\033[?1049l\033[0m` to `cfg.Stdout`.
       - Stream `cfg.File` to `cfg.Stdout` via `io.Copy(cfg.Stdout, f)` (not `ioutil.ReadFile` — avoids buffering large scrollback in memory; per spec, scrollback up to `history-limit × avg-line-bytes` which can be MBs). If `os.Open(cfg.File)` fails with ENOENT → delegate to task 3-10's `handleFileMissing()`. If mid-`io.Copy` read error → also delegate to task 3-10 (partial-read is treated as the file-missing path, per task 3-10's edge cases).
       - Emit postamble bytes `\033[?25h\033[?1049l\033[0m\r\n`.
       - `time.Sleep(100 * time.Millisecond)`.
       - Call `cfg.Client.UnsetServerOption("@portal-skeleton-" + livePaneKey)` — uses the same `UnsetServerOption` method from task 3-7 (wraps `tmux set-option -su <name>`). Log errors but do not abort the exec.
       - Phase 4 hook-firing is NOT performed here; defer the hook lookup and `exec sh -c 'HOOK; exec $SHELL'` to Phase 4. In this phase, the helper continues to step 4.
    4. `exec $SHELL` via `syscall.Exec(shell, []string{shell}, os.Environ())` where `shell := os.Getenv("SHELL")` (defaulting to `/bin/sh` if unset). This replaces the helper process atomically with the shell — no child process remains. Because `syscall.Exec` replaces the process image, the `defer`s registered above do not fire — so file closes must happen before the `exec`.
- Add `cmd/state_hydrate_helpers.go`:
  - `func paneKeyFromFIFOPath(fifoAbs string) string` → `strings.TrimSuffix(strings.TrimPrefix(filepath.Base(fifoAbs), "hydrate-"), ".fifo")`.
  - `func openFIFOWithTimeout(path string, timeout time.Duration) (*os.File, error)` → spawns a goroutine to `os.OpenFile(path, os.O_RDONLY, 0)` and selects against `time.After(timeout)`. On signal-arrived path, returns the opened file. On timeout, returns `ErrHydrateTimeout` (new sentinel error in the package). The goroutine's blocked-open FIFO leaks until a writer opens or the process exits; acceptable because this process is about to `exec` anyway. Alternative: `syscall.Open` with `O_RDONLY|O_NONBLOCK` and poll — simpler for this task, same effect. Prefer the goroutine+`time.After` approach the spec mentions: "Use a goroutine + `time.After` channel + `select` (or equivalent I/O-with-deadline pattern)."
- Preamble / postamble byte constants live in `cmd/state_hydrate.go`:
  ```go
  const (
      resetPreamble  = "\x1b[?25h\x1b[?1049l\x1b[0m"
      resetPostamble = "\x1b[?25h\x1b[?1049l\x1b[0m\r\n"
      hydrateTimeout = 3 * time.Second
      settleSleep    = 100 * time.Millisecond
  )
  ```
- Tests in `cmd/state_hydrate_test.go`:
  - Happy signal-arrived path: stub FIFO returns a byte immediately; scrollback file contains `"hello\x1b[31mred\x1b[0m\n"`; verify stdout receives preamble + scrollback + postamble; `UnsetServerOption("@portal-skeleton-work__0.0")` called after 100ms sleep (assert via mock clock + time-travel helper); `exec` is stubbed to record the target so test asserts `$SHELL` target rather than really replacing the process.
  - ANSI preservation: scrollback bytes flow through `io.Copy` without escape processing — assert bytes in stdout contain the raw ESC sequences.
  - FIFO `os.Remove` is called after the read and before scrollback dump.
  - Marker-unset uses `-su` flag (verify via Commander mock that the underlying tmux call includes `-u`).
  - `livePaneKey` is correctly derived from the FIFO path basename.
  - Large scrollback file (10 MB): `io.Copy` streams without OOM; test with a mock writer asserts zero backpressure errors and full byte count.
  - 100ms settle sleep is observed (mock clock + assertion that marker-unset fires after 100ms of wall-time or mock-time).
  - Hook firing is NOT performed (no hooks.json read, no `exec sh -c 'HOOK'`). Assert by providing a panicking `hooks` dependency; panic must not fire. Document test comment: "Phase 4 adds hook firing; this assertion enforces the phase-boundary."
  - `$SHELL` defaulting: `SHELL` unset → `/bin/sh` used.

**Acceptance Criteria**:
- [ ] Helper opens FIFO with `O_RDONLY` and blocks with a 3-second timeout.
- [ ] On signal arrival: reads one byte, closes, `os.Remove`s the FIFO (ignore `ENOENT`).
- [ ] Emits `\x1b[?25h\x1b[?1049l\x1b[0m` (cursor show + exit alt-screen + SGR reset) to stdout before the dump.
- [ ] Streams the scrollback file to stdout via `io.Copy` (not buffered in memory).
- [ ] Emits `\x1b[?25h\x1b[?1049l\x1b[0m\r\n` to stdout after the dump.
- [ ] Sleeps exactly 100ms after the postamble (mock-clock verified).
- [ ] Calls `tmux set-option -su @portal-skeleton-<livePaneKey>` — uses `-u` flag, never empty-string assignment.
- [ ] `livePaneKey` is extracted from the `--fifo` flag's basename (`hydrate-<livePaneKey>.fifo` → `<livePaneKey>`).
- [ ] `exec $SHELL` replaces the helper process (test asserts the target via stubbed exec).
- [ ] `$SHELL` defaults to `/bin/sh` when unset.
- [ ] Large scrollback files stream without buffering the whole file in memory.
- [ ] ANSI escape sequences in scrollback pass through untouched.
- [ ] Hook firing is explicitly NOT performed (deferred to Phase 4) — verified by panicking-hooks-dependency test.

**Tests**:
- `"it opens the FIFO O_RDONLY and blocks for the signal"`
- `"it reads a single byte from the FIFO on signal arrival"`
- `"it closes and unlinks the FIFO after reading the signal"`
- `"it emits the reset preamble before the scrollback dump"`
- `"it streams the scrollback file bytes to stdout verbatim"`
- `"it emits the reset postamble with CRLF after the dump"`
- `"it sleeps 100ms before unsetting the skeleton marker"`
- `"it unsets the skeleton marker using tmux set-option -su"`
- `"it derives the live paneKey from the --fifo flag basename"`
- `"it preserves ANSI escape sequences in the dumped bytes"`
- `"it streams large scrollback files without buffering in memory"`
- `"it execs $SHELL when no hook applies (hook firing deferred to Phase 4)"`
- `"it defaults $SHELL to /bin/sh when unset"`
- `"it does not read hooks.json in this phase"`

**Edge Cases**:
- FIFO open uses `O_RDONLY` — blocking until writer connects; the goroutine+`time.After` gives us the timeout.
- Single byte read: content irrelevant — any successful `Read` (even zero bytes on `EOF` if the writer somehow closed without writing) completes the signal. For safety, treat any successful `Read` return (including `n == 1`) as signal-arrived.
- Marker-unset uses `-su` (empty-string assignment is explicitly forbidden — would leave the option present with empty value, blocking daemon capture).
- Large scrollback: `io.Copy`'s default 32KB buffer is sufficient; no custom buffer needed.
- `exec $SHELL` replaces the process — registered defers do NOT fire. All cleanup (FIFO unlink, marker-unset) must happen before the exec.
- `io.Copy` error mid-dump: delegate to task 3-10's file-missing / partial-read handler (same degradation path per task 3-10 edge cases).
- Hooks.json is NOT read in this phase — Phase 4 adds that step. This task's helper invokes `$SHELL` directly on success; Phase 4 replaces the `exec $SHELL` line with the hook-or-shell chain.
- Mocked exec: tests substitute a `execReplacer func(name string, argv []string, env []string) error` that records the target; production code uses `syscall.Exec`.

**Context**:
> Spec "Scrollback Restore Mechanics → Helper Behavior on Startup → 1.–2.":
> 1. Open FIFO for reading, block with 3-second timeout.
> 2. On signal arrival:
>    a. Close + os.Remove the FIFO.
>    b. Emit reset preamble to stdout: `\033[?25h\033[?1049l\033[0m` (cursor visible + exit alt-screen defensively + SGR reset).
>    c. Copy the scrollback file's bytes to stdout.
>    d. Emit reset postamble + CRLF: `\033[?25h\033[?1049l\033[0m\r\n`.
>    e. `time.Sleep(100 * time.Millisecond)`.
>    f. Read hooks.json; look up this pane's resume hook using the --hook-key argument ... [DEFERRED TO PHASE 4]
>    g. tmux set-option -su @portal-skeleton-<paneKey> (remove marker via -u flag — load-bearing; empty-string assignment does NOT remove).
>    h. If hook exists: exec sh -c 'HOOK; exec $SHELL'. Else: exec $SHELL. [PHASE 3: ALWAYS exec $SHELL]
>
> Spec "Scrollback Restore Mechanics → The 100ms Settle Sleep (Why the Helper Owns Marker-Unset)": "The helper's `write()` to stdout returning does **not** mean tmux has finished parsing the written bytes. ... If `signal-hydrate` unset the marker immediately (right after writing the FIFO byte), the daemon's next tick could run while the helper is still mid-dump. `capture-pane` would return partial scrollback; content-hash dedup would compute a hash based on the partial state and overwrite the full saved scrollback file with a truncated version. Transferring marker-unset ownership to the helper makes 'marker cleared' synonymous with 'helper's output is complete and the pane's scrollback is in its final form.' The 100ms sleep before the unset is a safe margin against tmux's PTY-parser lag (typical lag is ~1ms; 100ms is generous without being user-perceptible against the 500–1500ms dump)."
>
> Spec "Scrollback Restore Mechanics → Implementation Notes": "The `; exec $SHELL` chain is a shell construct; the pane's command must be invoked as `sh -c '...'` so the shell parses the `;` correctly. The helper's blocking FIFO read must implement a timeout. Go's standard `os.File.Read` on a FIFO does not time out on its own. Use a goroutine + `time.After` channel + `select` (or equivalent I/O-with-deadline pattern)."
>
> Phase 3 deliberately leaves hook firing for Phase 4 per the phase-planning docs: Phase 4 does the `ExecuteHooks` deletion + helper-based firing cutover in a single change.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Scrollback Restore Mechanics → Helper Behavior on Startup", "Scrollback Restore Mechanics → The 100ms Settle Sleep", "Scrollback Restore Mechanics → Implementation Notes".

## built-in-session-resurrection-3-9 | approved

### Task 3-9: Implement `portal state hydrate` 3-second timeout path

**Problem**: If `signal-hydrate` never fires (user never attaches, `client-attached` hook errored, FIFO writer retries exhausted), the helper's FIFO `open(O_RDONLY)` blocks forever. Shell never starts; pane is permanently stuck on the `sh -c '...'` wrapper command. The spec's guardrail is a 3-second timeout — long enough for typical attach latency (~10–50ms) plus slow-but-legit cases (NFS, heavy load, ~1–2s tail) with 2× margin. On timeout the helper takes a distinctly different course from the signal-arrived path: (a) emit the reset preamble (no dump, no postamble); (b) skip the 100ms sleep (nothing was dumped, nothing to settle); (c) LEAVE the skeleton marker set (next attach will re-signal and retry — key distinction from the file-missing path which clears the marker); (d) log the timeout with the `--hook-key` for diagnosis; (e) still `os.Remove` the FIFO so it doesn't leak into the state directory (the next bootstrap's task-3-12 sweep would clean it, but doing it inline is tidier); (f) `exec $SHELL`. This task lands the timeout branch of `runHydrate` from task 3-8.

**Solution**: Extend `runHydrate` in `cmd/state_hydrate.go` with the timeout branch: when `openFIFOWithTimeout` returns `ErrHydrateTimeout`, emit preamble only, unlink the FIFO, log a warning carrying `--hook-key`, and `exec $SHELL`. Critically the skeleton marker stays set — do NOT call `UnsetServerOption` on the timeout path. The spec is explicit: "Do NOT unset the skeleton marker — next attach will re-signal and retry."

**Outcome**: A helper that never receives a signal degrades to an empty shell after 3 seconds. The pane is usable but scrollback is blank. The `@portal-skeleton-<paneKey>` marker remains set, so the daemon's capture loop skips this pane, preserving the saved scrollback file on disk. The next `client-attached` / `client-session-changed` event triggers `signal-hydrate`, which re-writes to the FIFO (now gone — will fail with `ENOENT`; `signal-hydrate`'s retry ladder exhausts and logs). Since the FIFO is gone, the retry is moot for *this* specific pane in *this* server lifetime; however the marker-remains-set property is load-bearing for the save loop's skip behaviour.

Wait — re-read spec more carefully: "timeout stays set so next attach re-signals". But if the FIFO is unlinked on timeout, the next `signal-hydrate` write will fail (no FIFO). The spec's phrase "next attach re-signals and retries" applies primarily to the case where the helper is still running (FIFO still open for read) on a slow open — not our post-timeout case. After the helper's `exec $SHELL`, there is no reader; signalling a non-existent FIFO is a no-op anyway. The intent of "marker stays set" on timeout is: (a) the save loop continues to skip the pane, preserving the saved scrollback file on disk; (b) the *saved* data is preserved for a potential next-bootstrap retry (user reboots or kills tmux → new skeleton restore will set up a fresh FIFO and helper). So "retry on next attach" for this task effectively means "retry on next bootstrap" — the marker-set property is what matters, not the FIFO path. Therefore: unlink the FIFO on timeout to prevent stale FIFO accumulation; leave the marker set; move on. The spec's edge-case note "FIFO unlinked on timeout too (prevent orphan)" matches this interpretation.

**Do**:
- Extend the `openFIFOWithTimeout`-returning-error branch in `runHydrate` (from task 3-8) to cover `ErrHydrateTimeout`:
  ```go
  file, err := openFIFOWithTimeout(cfg.FIFO, hydrateTimeout)
  if errors.Is(err, ErrHydrateTimeout) {
      return handleHydrateTimeout(cfg, livePaneKey)
  }
  // ... signal-arrived path (task 3-8)
  ```
- Implement `handleHydrateTimeout(cfg HydrateConfig, livePaneKey string) error`:
  1. `io.WriteString(cfg.Stdout, resetPreamble)` — cursor + alt-screen + SGR reset. No content dump. No postamble (there's nothing to follow).
  2. `_ = os.Remove(cfg.FIFO)` — best-effort; ignore errors including `ENOENT`. Prevents orphan FIFO in state dir.
  3. `cfg.Logger.Printf("hydrate: timeout waiting for signal on --hook-key=%s --fifo=%s", cfg.HookKey, cfg.FIFO)` — identify the pane by saved hook-key for operational diagnosis.
  4. Do NOT call `UnsetServerOption` — the marker STAYS set per spec.
  5. Do NOT sleep 100ms — nothing was dumped; no settle window required.
  6. Return nil; main helper loop falls through to `execShell(cfg)`.
- Ensure `execShell(cfg)` is factored out of task 3-8 so both signal-arrived and timeout paths end identically (both `exec $SHELL`).
- Tests in `cmd/state_hydrate_test.go`:
  - Timeout simulation: inject a FIFO opener that returns `ErrHydrateTimeout` after mock-clock-advance of 3s. Assert: preamble written to stdout, no postamble, no scrollback dump, no 100ms sleep, FIFO removed (assert via test filesystem observer), marker unset is NOT called, warning log includes `--hook-key` value, exec target is `$SHELL`.
  - Mocked `os.Remove` of FIFO with `ENOENT`: no error, no log about the removal (it's silent best-effort).
  - Mocked `os.Remove` with permission error: no error returned from `handleHydrateTimeout`, warning optionally logged but not fatal — document in test whether we log permission errors or swallow them; safer to swallow since the process is about to `exec`.
  - Timeout fires after exactly 3 seconds (not less): mock-clock test with `time.After(3 * time.Second)` — verify no premature fire at t=2.99s; fires at t≥3s.
  - Marker-unset NEVER called on timeout: Commander mock verifies zero `set-option -su` calls on this path. This is the single most important assertion — it's what the spec pins as "next attach re-signals and retries".
  - Hook firing NOT performed on timeout path (Phase 4 will also observe this invariant).

**Acceptance Criteria**:
- [ ] Timeout fires at 3 seconds (measured via `time.After` in the goroutine race).
- [ ] On timeout: reset preamble written to stdout.
- [ ] On timeout: no scrollback dump.
- [ ] On timeout: no reset postamble (no dump → no closing bracket).
- [ ] On timeout: no 100ms settle sleep.
- [ ] On timeout: FIFO unlinked via `os.Remove` (ignore `ENOENT`).
- [ ] On timeout: `@portal-skeleton-<livePaneKey>` marker is NOT unset (stays set for save-loop skip + potential next-bootstrap retry).
- [ ] On timeout: warning log line includes the `--hook-key` flag value for operational diagnosis.
- [ ] On timeout: helper ends with `exec $SHELL` (bare shell).
- [ ] On timeout: hook firing is NOT performed (matches Phase 4's timeout-path contract).

**Tests**:
- `"it fires the timeout branch after 3 seconds of FIFO block"`
- `"it writes the reset preamble on timeout"`
- `"it writes no scrollback bytes on timeout"`
- `"it writes no reset postamble on timeout"`
- `"it does not sleep 100ms on timeout"`
- `"it removes the FIFO on timeout (best-effort, ignores ENOENT)"`
- `"it does NOT call set-option -su on timeout (marker stays set)"`
- `"it logs a warning naming the --hook-key on timeout"`
- `"it execs $SHELL after the timeout path"`
- `"it does not read or fire hooks on timeout"`
- `"it tolerates os.Remove permission error silently"`

**Edge Cases**:
- Timeout measured via `time.After` in the goroutine that raced with the FIFO open — Go's `os.File.Read` on a FIFO does not have a native deadline.
- No content dump, no postamble — helper produces only the 20-ish bytes of preamble before execing the shell. User sees an empty pane that prompts immediately after the preamble.
- Skip the 100ms sleep — nothing was dumped; the PTY parser has nothing to catch up on.
- Marker stays set — preserves the saved scrollback file on disk (save loop keeps skipping) and signals to the *next bootstrap* that this pane still needs hydration. Within this server lifetime, the FIFO is gone and no further signal will arrive; the "next attach re-signals" phrasing in the spec is best understood as "if the user reboots tmux and the marker persists" (the marker is volatile, so it doesn't — but at bootstrap, task 3-3 recreates the skeleton including setting the marker fresh). The net effect is: this server lifetime = empty pane; next bootstrap = fresh retry.
- Log entry identifies `--hook-key` so operators running `grep hydrate portal.log` can match timeouts to saved panes.
- FIFO unlinked on timeout — prevents the state dir from accumulating stale FIFOs between bootstraps. Task 3-12 also sweeps them at bootstrap, so this is defence-in-depth.
- No error returned from `handleHydrateTimeout` — helper proceeds to `exec $SHELL` regardless.

**Context**:
> Spec "Scrollback Restore Mechanics → Helper Behavior on Startup → 3.":
> 3. On 3-second timeout (no signal arrived):
>    a. Emit reset preamble only (no content dump).
>    b. Skip the 100ms sleep.
>    c. Do NOT unset the skeleton marker — next attach will re-signal and retry.
>    d. Log a warning to portal.log.
>    e. exec $SHELL (bare shell; no hook firing on this path).
>
> Spec "Scrollback Restore Mechanics → Timeout: 3 Seconds":
> - Normal signal latency: ~10–50ms.
> - Slow-but-legit upper bound (NFS home, heavy system load, slow hook script): ~1–2s.
> - 3s ≈ 2× the slow-legit tail. Fast enough to degrade snappily on real failures without cutting off rare slow-legit cases.
>
> Spec "Scrollback Restore Mechanics → Marker Lifecycle Summary → Helper does NOT unset marker on FIFO timeout — next attach re-signals, retry happens naturally."
>
> Spec "Failure Modes & Recovery → Consolidated Failure-Handling Table → Hydrate signal never arrives (hook failure, FIFO issue)": "3-second timeout; helper degrades to empty shell + logs warning. `@portal-skeleton-<key>` marker stays set; next attach re-signals and retries."
>
> Task 3-13 edge case confirmation: "FIFO unlinked on timeout too (prevent orphan)" — align implementation with this edge-case note.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Scrollback Restore Mechanics → Helper Behavior on Startup", "Scrollback Restore Mechanics → Timeout: 3 Seconds", "Scrollback Restore Mechanics → Marker Lifecycle Summary", "Failure Modes & Recovery".

## built-in-session-resurrection-3-10 | approved

### Task 3-10: Implement `portal state hydrate` scrollback-file-missing path

**Problem**: If the saved scrollback file is missing (deleted by user, GC race, permission error, first-ever bootstrap with no prior save), the helper cannot dump content. The spec calls out a third distinct degradation path separate from signal-arrived and timeout: on file-missing, the helper (a) emits preamble only (no dump, no postamble); (b) logs a warning distinguishing ENOENT vs permission error for operator diagnosis; (c) skips the 100ms sleep (nothing was dumped); (d) clears the skeleton marker (this is the key difference from the timeout path — here we want the save loop to resume capturing this now-empty pane, rather than preserving the absent file); (e) `exec $SHELL` (or hook in Phase 4). Partial-read I/O errors mid-dump (e.g., disk flipped into read-only during `io.Copy`) are treated as this same file-missing path since the dump is already broken and the cleanest recovery is "clear marker, move on". This task lands the file-missing branch.

**Solution**: Extend `runHydrate` in `cmd/state_hydrate.go` with a `handleHydrateFileMissing` branch that runs on (a) `os.Open(cfg.File)` returning any error prior to dump start, or (b) `io.Copy(cfg.Stdout, f)` returning an error mid-stream. Both feed into the same handler because the user-visible outcome is identical: partial (or absent) content, pane is empty-ish, clear marker so save resumes normally, log with error cause. Use `errors.Is(err, os.ErrNotExist)` vs other error types to pick the log prefix (`"file not found"` vs `"read error"`). Clear the marker via `UnsetServerOption` same as the signal-arrived path — once cleared, the daemon captures this pane on its next tick (overwriting any residual saved-indexed `.bin`). The FIFO is also removed (since the signal-arrived path didn't reach the `os.Remove` — the failure happens after FIFO-read but before/during file-open).

Re-read spec for FIFO unlink semantics on file-missing path: the spec says "2. On signal arrival: a. Close + os.Remove the FIFO" — this happens before the scrollback open attempt. So by the time we detect file-missing (step 2c's dump), the FIFO is already gone. Implementation: `handleHydrateFileMissing` only needs to worry about preamble, log, marker-unset, shell-exec — not FIFO cleanup.

**Outcome**: A helper whose saved scrollback file has vanished degrades to an empty shell and clears its marker so the save loop resumes. Operators see a distinct `"file not found"` or `"read error"` log entry naming the `--file` and `--hook-key` for diagnosis. The next daemon tick captures this pane fresh (empty scrollback, since the shell just started) — matching the spec's "empty pane, not stuck" principle.

**Do**:
- Add `handleHydrateFileMissing(cfg HydrateConfig, livePaneKey string, cause error) error` in `cmd/state_hydrate.go`:
  1. `io.WriteString(cfg.Stdout, resetPreamble)` — only emitted if not already emitted. If `handleHydrateFileMissing` is called from the post-signal-arrival file-open step, the preamble was NOT yet emitted → emit now. If called from mid-`io.Copy`, the preamble WAS already emitted → do not re-emit. Pass a `preambleEmitted bool` parameter.
  2. Distinguish the log prefix:
     - If `errors.Is(cause, os.ErrNotExist)` → `"hydrate: scrollback file not found for --hook-key=%s --file=%s"`.
     - Else if `errors.Is(cause, os.ErrPermission)` → `"hydrate: scrollback file unreadable (permission denied) for --hook-key=%s --file=%s"`.
     - Else → `"hydrate: scrollback file I/O error for --hook-key=%s --file=%s: %v"`.
  3. Log via `cfg.Logger.Printf(...)`.
  4. Do NOT sleep 100ms — nothing was dumped / dump was truncated, the VT parser has nothing definitive to settle on.
  5. `cfg.Client.UnsetServerOption("@portal-skeleton-" + livePaneKey)` — uses `-su`, removes the option (per spec; empty-string assignment does NOT remove). Log the underlying tmux error if this fails; non-fatal.
  6. Do NOT perform hook firing in this phase (Phase 4 will add it here AND on the signal-arrived success path, but NOT on the timeout path).
  7. Return nil; helper's main loop proceeds to `execShell(cfg)`.
- Wire into `runHydrate`:
  ```go
  // After signal-arrived read + FIFO cleanup + preamble emission:
  f, openErr := os.Open(cfg.File)
  if openErr != nil {
      return handleHydrateFileMissing(cfg, livePaneKey, openErr, true /* preamble already emitted */)
  }
  defer f.Close()
  if _, copyErr := io.Copy(cfg.Stdout, f); copyErr != nil {
      return handleHydrateFileMissing(cfg, livePaneKey, copyErr, true /* preamble emitted */)
  }
  ```
  (The `true` for `preambleEmitted` matches the spec's flow — preamble is emitted at step 2b, before the open at step 2c.)
- Tests in `cmd/state_hydrate_test.go`:
  - File missing (ENOENT): preamble written, log contains `"file not found"` + hook-key + file path, marker unset via `-su`, shell execed, no 100ms sleep (assert via mock clock).
  - File unreadable (permission denied, `EACCES`): preamble written, log contains `"permission denied"` + hook-key, marker unset, shell execed.
  - File I/O error mid-copy (simulate via a file that fails `Read` after N bytes — use a wrapper `io.Reader` that returns bytes then error): partial content written to stdout + preamble, log contains `"I/O error"`, marker unset, shell execed. Preamble is not re-emitted (already out).
  - Marker-unset uses `-su` flag (Commander mock verifies).
  - No hook firing (panicking-hooks test).
  - `execShell` runs (asserted via stubbed exec target).
  - No 100ms sleep — assert via mock clock.

**Acceptance Criteria**:
- [ ] File-not-found (`errors.Is(err, os.ErrNotExist)`) triggers the missing-file path with a `"file not found"` log prefix.
- [ ] Permission error (`errors.Is(err, os.ErrPermission)`) triggers the path with a distinct `"permission denied"` log prefix.
- [ ] Generic I/O error (e.g., `EIO` during copy) triggers the path with a `"I/O error"` log prefix.
- [ ] Log entry includes both `--hook-key` and `--file` values for diagnosis.
- [ ] Marker `@portal-skeleton-<livePaneKey>` is unset via `tmux set-option -su` (uses `-u` flag).
- [ ] 100ms sleep is skipped (nothing to settle).
- [ ] Reset preamble is NOT re-emitted if the failure happens after the initial preamble write (e.g., mid-`io.Copy`). Preamble IS emitted if the failure is the open-step itself (though in practice the spec flow has preamble first, so the flag is always `true` in production code; still — parameterised).
- [ ] Mid-copy partial-read is treated identically to file-missing (same log prefix choice via `errors.Is`, same marker-unset, same shell-exec).
- [ ] `exec $SHELL` at end (hook firing deferred to Phase 4).
- [ ] Partial-read leaves whatever bytes made it to stdout already visible (no rollback); user sees truncated-but-real content instead of a rolled-back empty pane.

**Tests**:
- `"it triggers the file-missing path when os.Open returns ENOENT"`
- `"it triggers the file-missing path when os.Open returns permission denied"`
- `"it triggers the file-missing path when io.Copy fails mid-stream"`
- `"it logs distinctly for ENOENT vs permission vs generic I/O"`
- `"it includes --hook-key and --file in the file-missing warning log"`
- `"it unsets the skeleton marker on file-missing via set-option -su"`
- `"it skips the 100ms sleep on file-missing"`
- `"it execs $SHELL after the file-missing branch"`
- `"it does not re-emit the reset preamble when called after initial preamble write"`
- `"it does not fire hooks on file-missing (deferred to Phase 4)"`
- `"it leaves already-written partial bytes on stdout without rollback"`

**Edge Cases**:
- ENOENT vs permission: both degrade but produce distinct log prefixes. Operators triaging know which to look at first.
- Partial-read mid-dump: the user sees however much content did stream before the error, followed by the postamble (actually — we do NOT emit the postamble on this path; the handler skips directly from the copy failure to marker-unset). Decision: treat the copy-failure as if dump never completed — do not emit postamble. The reset preamble already set the display to a sane state.

  Clarification for implementation: `io.Copy` succeeds partially (returns `(n, err)` with n > 0, err != nil). In that case, `n` bytes are already on stdout. We do NOT emit postamble (since the dump is incomplete). We do emit a terminal-sanitizing sequence — reuse the same preamble string (`\x1b[?25h\x1b[?1049l\x1b[0m`) as a "postamble-equivalent" to reset state. Safer than leaving user in a possibly-corrupted state. Document in test: "it emits a reset sequence after partial-dump to leave terminal sane".

  Actually, reviewing the spec again: "On scrollback file missing / unreadable (detected at step 2c of the signal path): a. Emit reset preamble only (no content dump)." The spec does NOT specify behaviour for mid-copy partial-read explicitly. The spec's edge-case for this task says "partial-read I/O error mid-dump treated as this path". Treating as "this path" means emitting preamble only — but preamble was already emitted at step 2b. Net: the mid-copy path does NOT emit an additional sequence. Any partial bytes remain on stdout; user sees them; shell starts. This is slightly worse than adding a belt-and-braces reset but aligns with the spec's "no double preamble" flow. Document the decision in a code comment.

- Permission error is *permanent* for this attempt — the helper does not retry. File-becomes-readable-later does not trigger a fresh dump.
- Missing file may be expected on the *first-ever* bootstrap where `sessions.json` was just written but the scrollback file wasn't (crash between write steps 2 and 3 of atomic commit from task 2-10). The marker-clear + save-resumes flow handles this automatically — next tick captures the pane fresh.

**Context**:
> Spec "Scrollback Restore Mechanics → Helper Behavior on Startup → 4.":
> 4. On scrollback file missing / unreadable (detected at step 2c of the signal path):
>    a. Emit reset preamble only (no content dump).
>    b. Log a warning.
>    c. Skip the 100ms sleep (nothing was dumped).
>    d. tmux set-option -su @portal-skeleton-<paneKey> — remove the marker inline so the save loop resumes capturing this empty pane.
>    e. Continue to step h (hook/shell exec). Hook runs if registered; else bare shell. [PHASE 3: shell only]
>
> Spec "Failure Modes & Recovery → Consolidated Failure-Handling Table → Scrollback file missing at hydrate time": "Helper logs a warning, emits reset preamble only (no dump), exec's shell or hook. Empty pane, not stuck."
>
> Task 3-8's signal-arrived path delegates file-open failures to this handler. Task 3-13 integration tests cover end-to-end the case where a scrollback file is deleted between save and restore.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Scrollback Restore Mechanics → Helper Behavior on Startup → 4.", "Failure Modes & Recovery → Consolidated Failure-Handling Table".

## built-in-session-resurrection-3-11 | approved

### Task 3-11: Implement `portal state signal-hydrate <session>` with `O_WRONLY | O_NONBLOCK` retry ladder

**Problem**: `signal-hydrate` is the attach-time trigger that unblocks every skeleton-restored pane's helper. Phase 1 registered `client-attached` and `client-session-changed` hooks to invoke `portal state signal-hydrate #{session_name}`. The command must: (1) enumerate panes in the given session via `tmux list-panes -t <session>`; (2) for each pane with `@portal-skeleton-<livePaneKey>` set, open the pane's FIFO for writing with `O_WRONLY | O_NONBLOCK` and write a single byte; (3) retry with 10/20/40/…/≤500ms ladder on `ENXIO`/`EAGAIN` (the helper's `open(O_RDONLY)` might not have completed yet for very fast attaches); (4) on retry exhaustion, log a warning and move on — do not touch the marker (helper owns that timing); (5) be fully idempotent across double-fires (both `client-attached` and `client-session-changed` may fire for one logical attach — the second write hits a closed/unlinked FIFO and just fails, which is harmless); (6) be silent on non-skeleton sessions (zero markers → zero writes → zero noise); (7) be tolerant of non-existent target sessions (log and return 0 so the tmux hook's `run-shell` doesn't produce error noise).

**Solution**: Replace the stub `RunE` in `cmd/state_signal_hydrate.go` (from task 1-1) with: resolve state dir via `state.EnsureDir()`; read the server-scope option dump via `tmux show-options -sv`; filter for `@portal-skeleton-` prefixed keys; enumerate `tmux list-panes -t <session> -F '#{window_index}:#{pane_index}'` into live pane coordinates; for each live pane, compute `livePaneKey` via `state.SanitizePaneKey` and check set-membership in the filtered marker map; for each match, call `writeFIFOSignal(fifoPath)` which implements the 10/20/40/80/160/320/≤500ms retry ladder on `ENXIO`/`EAGAIN`; log retry exhaustion but return 0 overall. Argument validation inherits from task 1-1's `cobra.ExactArgs(1)`. This command never touches skeleton markers — that's the helper's job (tasks 3-8, 3-10).

**Outcome**: `client-attached` and `client-session-changed` hooks reliably unblock every skeleton-restored pane in the attached session. Idempotent double-fires are harmless. Non-skeleton sessions no-op silently. Retry ladder covers the very-fast-attach race where the helper's `open(O_RDONLY)` is still pending. End-to-end validated by task 3-13's integration test.

**Do**:
- Replace the `RunE` in `cmd/state_signal_hydrate.go`:
  ```go
  RunE: func(cmd *cobra.Command, args []string) error {
      sessionName := args[0]
      dir, err := state.EnsureDir()
      if err != nil {
          return fmt.Errorf("signal-hydrate: state dir: %w", err)
      }
      client, err := newClient()  // or inject via signalHydrateDeps
      if err != nil {
          return fmt.Errorf("signal-hydrate: new tmux client: %w", err)
      }
      markers, err := fetchSkeletonMarkers(client)
      if err != nil {
          logger.Printf("signal-hydrate: show-options failed: %v", err)
          return nil // non-fatal; hook's run-shell should not error-spam
      }
      panes, err := client.ListPanesInSession(sessionName)
      if err != nil {
          logger.Printf("signal-hydrate: list-panes -t %q failed: %v", sessionName, err)
          return nil // session may not exist; non-fatal
      }
      for _, p := range panes {
          livePaneKey := state.SanitizePaneKey(sessionName, p.Window, p.Pane)
          if _, ok := markers["@portal-skeleton-"+livePaneKey]; !ok {
              continue // already hydrated or never skeleton-restored
          }
          fifoPath := state.FIFOPath(dir, livePaneKey)
          if werr := writeFIFOSignal(fifoPath); werr != nil {
              logger.Printf("signal-hydrate: write %s failed: %v", fifoPath, werr)
              // marker stays set; next attach may retry
              continue
          }
      }
      return nil
  }
  ```
- Add `fetchSkeletonMarkers(client *tmux.Client) (map[string]string, error)`:
  1. Call `client.ShowServerOptions()` — wraps `tmux show-options -sv`.
  2. Parse output lines as `@<name> <value>` (tmux's server-scope option format). Return a map of name → value, filtered to keys starting with `@portal-skeleton-`.
- Add `writeFIFOSignal(path string) error`:
  1. Retry schedule: `[]time.Duration{10ms, 20ms, 40ms, 80ms, 160ms, 320ms}` — cumulative sum is 630ms, truncated at 500ms total → use `[10, 20, 40, 80, 160]` cumulative 310ms with one more 190ms to hit exactly 500ms total. Prefer the spec's literal `10/20/40/…/≤500ms` shape. Implementation: compute `[]time.Duration{10ms, 20ms, 40ms, 80ms, 160ms, 190ms}` with cumulative ≤500ms.
  2. On each attempt: `f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)`.
     - On `errors.Is(err, syscall.ENXIO)` (no reader yet) → sleep next delay, retry.
     - On `errors.Is(err, syscall.EAGAIN)` → sleep next delay, retry.
     - On other error (including `ENOENT` — FIFO removed since marker-set) → return error immediately.
  3. On successful open: `f.Write([]byte{1})`; `f.Close()`. Content irrelevant; any byte works.
  4. On exhausted retries: return `fmt.Errorf("retries exhausted opening %s: %w", path, lastErr)`.
- Add `tmux.Client.ShowServerOptions() (string, error)` wrapping `tmux show-options -sv` if absent (or reuse from task 2-11's daemon marker enumeration).
- Tests in `cmd/state_signal_hydrate_test.go`:
  - Session with no skeleton markers: zero FIFO writes; no errors; returns 0.
  - Session with two skeleton-marked panes: exactly two FIFO writes, one per pane; each goes to the correct `hydrate-<paneKey>.fifo`.
  - Idempotent second invocation (same session, already-hydrated): `show-options -sv` returns no markers for that session's panes; zero writes.
  - Non-existent session: `list-panes` returns error; logged; return 0 (no stderr output from command).
  - Retry ladder: first attempt hits `ENXIO`, second succeeds. Test uses a fake FIFO opener with a controlled error sequence. Verify exactly 2 open attempts, 10ms sleep between them, successful write on second.
  - Retry exhaustion: all 6 attempts hit `ENXIO`. Total elapsed ≤500ms. Logged warning. Marker NOT touched by this command.
  - `ENOENT` (FIFO missing): no retry; logged; continue to next pane.
  - `EAGAIN` treated the same as `ENXIO` in retry ladder.
  - Marker never unset by this command: panic-on-unset assertion in Commander mock.
  - Non-skeleton pane (marker absent): skipped silently; no FIFO write attempt.
  - Retry delays: use a mock clock to verify `10, 20, 40, 80, 160, 190` ms between attempts without real sleeps.

**Acceptance Criteria**:
- [ ] `signal-hydrate <session>` enumerates live panes via `list-panes -t <session>`.
- [ ] Reads server-scope options via `show-options -sv` and filters for `@portal-skeleton-` prefix.
- [ ] For each live pane whose `livePaneKey` has a matching marker, opens the FIFO with `O_WRONLY | O_NONBLOCK` and writes one byte.
- [ ] Retry ladder on `ENXIO`/`EAGAIN`: 10, 20, 40, 80, 160, 190 ms delays (cumulative ≤500ms).
- [ ] Non-retryable errors (including `ENOENT`) do not retry; log and continue.
- [ ] Retry exhaustion logs a warning identifying the FIFO path and moves on — marker is NOT touched.
- [ ] Never calls `set-option -su` on any skeleton marker (helper owns marker-unset).
- [ ] Non-skeleton panes (marker absent) are skipped with zero work.
- [ ] Non-existent session argument logs and returns exit code 0.
- [ ] Idempotent across `client-attached` + `client-session-changed` double-fire (second call sees no markers → no-op).
- [ ] Command is exempt from bootstrap (via `state` in `skipTmuxCheck` from task 1-3) — no recursive bootstrap.

**Tests**:
- `"it writes a single byte to the FIFO for each skeleton-marked pane in the session"`
- `"it skips panes without the skeleton marker"`
- `"it handles a session with zero skeleton markers silently"`
- `"it retries on ENXIO with 10/20/40/80/160/190ms ladder"`
- `"it retries on EAGAIN the same as ENXIO"`
- `"it does not retry on ENOENT (FIFO missing)"`
- `"it logs retry exhaustion without touching the marker"`
- `"it never calls set-option -su on any skeleton marker"`
- `"it logs and returns 0 when the session does not exist"`
- `"it is idempotent across repeated invocations"`
- `"it does not invoke PersistentPreRunE bootstrap"`
- `"it uses O_WRONLY | O_NONBLOCK when opening the FIFO"`
- `"it completes all attempts within 500ms cumulative"`

**Edge Cases**:
- `O_WRONLY | O_NONBLOCK` — critical: POSIX specifies that `open(O_WRONLY)` on a FIFO without a reader blocks by default; `O_NONBLOCK` changes this to `ENXIO` return. Signal-hydrate is invoked via `run-shell` (synchronous per spec) — a blocking open would freeze tmux.
- Retry ladder 10/20/40/80/160/190 — cumulative 500ms exact. Beyond that, the helper itself times out at 3s (task 3-9) and the user sees an empty pane; `signal-hydrate` stops trying earlier so tmux's `run-shell` returns promptly.
- `ENXIO`: helper hasn't yet reached `open(O_RDONLY)` — retry.
- `EAGAIN`: less common on FIFOs but documented in POSIX — retry identically.
- `ENOENT`: FIFO was removed (helper already completed and unlinked it, or this pane's hydration failed). No point retrying; the marker-absence check should have caught this case — if we got here with the marker set AND the FIFO gone, something is inconsistent; log and continue.
- Marker never touched: load-bearing for correctness. If `signal-hydrate` unset the marker early (right after writing the FIFO byte), the daemon's next tick could capture mid-dump. The helper's 100ms-sleep-then-unset ordering depends on `signal-hydrate` staying hands-off.
- Double-fire: `client-attached` + `client-session-changed` fire for a single logical attach under some tmux flows. First invocation writes bytes and unblocks helpers; helpers dump + unset markers; second invocation's `show-options -sv` finds no markers (or at worst, the helper hasn't yet finished — in which case we double-write to a FIFO that still has a reader, harmless). Both paths are idempotent in effect.
- Session argument is a user-facing tmux session name (possibly containing spaces, colons, etc.) — passed through `tmux list-panes -t <name>` as an argv element. tmux handles quoting; the argv element passes through.
- Non-existent target session: tmux's `list-panes -t <name>` returns error. Log, return 0. The hook's `run-shell` should not emit error noise.

**Context**:
> Spec "Scrollback Restore Mechanics → Signal Mechanism: FIFO Per Pane → Signal (attach time)":
> The `client-attached` / `client-session-changed` hook runs `portal state signal-hydrate <session-name>`, which:
> 1. Enumerates panes in the attached session (`list-panes -t <session-name>`).
> 2. For each pane whose `@portal-skeleton-<key>` marker is set: open the pane's FIFO for writing and write a single byte.
> 3. For each pane whose marker is absent: no-op (already hydrated or never skeleton-restored).
>
> "`signal-hydrate` **does not touch the marker**. The helper owns marker-unset timing to close the capture-mid-dump race (see below)."
>
> Spec "Scrollback Restore Mechanics → FIFO open-for-write semantics": "POSIX FIFOs block `open(O_WRONLY)` until a reader opens. ... Because `signal-hydrate` is invoked via `run-shell` (synchronous by default), a stuck open would block the tmux server. Portal opens the FIFO with `O_WRONLY | O_NONBLOCK`. If the open returns `ENXIO` (no reader yet) or `EAGAIN`, `signal-hydrate` retries with a short backoff (e.g., 10ms, 20ms, 40ms, up to a ~500ms cumulative budget). If retries exhaust without a reader, `signal-hydrate` logs a warning and moves on — the skeleton marker stays set, and the next attach path will re-signal."
>
> Spec "CLI Surface → Internal Subcommands → `portal state signal-hydrate <session-name>`":
> - `list-panes -t <session-name>` → enumerate panes in the attached session.
> - For each pane with `@portal-skeleton-<paneKey>` set: open the pane's FIFO for writing, write a single byte, close.
> - For each pane without the marker: no-op.
> - Idempotent (safe to double-fire across `client-attached` + `client-session-changed` for a single logical attach).
> Does **not** unset skeleton markers. The helper owns that.
>
> Spec "Failure Modes & Recovery → Signal fires twice somehow": "Second write to the FIFO goes nowhere (the helper has already closed and unlinked the FIFO). Harmless."

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Scrollback Restore Mechanics → Signal Mechanism: FIFO Per Pane", "CLI Surface → Internal Subcommands → `portal state signal-hydrate`", "Failure Modes & Recovery".

## built-in-session-resurrection-3-12 | approved

### Task 3-12: Bootstrap state-dir sweep of orphan `hydrate-*.fifo` files

**Problem**: Task 3-2's per-pane `os.Remove + syscall.Mkfifo` sweeps a specific FIFO path immediately before pane creation — that handles the "same paneKey, prior run" case. But stale FIFOs from prior runs with *different* paneKeys (e.g., a session that was saved, restored with base-index=0, then the user set base-index=1, crashed mid-restore, then restarted) can accumulate in the state directory. Without a bootstrap-time sweep the state dir grows unboundedly. The spec calls this out: "Additional state-dir scan on bootstrap removes any stale FIFOs not matching a restored pane." Strictly speaking, the task-3-2 per-pane sweep would cover the common case on its own, but the spec's belt-and-braces sweep handles the edge cases and keeps the state dir tidy. The sweep must run *after* all skeleton restoration completes (so all to-be-kept FIFOs exist), preserve FIFOs that correspond to active panes (live markers), and only remove FIFOs (not other files matching the `hydrate-*.fifo` glob by accident).

**Solution**: Add `func SweepOrphanFIFOs(dir string, liveMarkerKeys map[string]struct{}, logger *log.Logger) error` in `internal/state/fifo_sweep.go`. Flow: (1) `glob := filepath.Glob(filepath.Join(dir, "hydrate-*.fifo"))`; (2) for each matched path: `stat, err := os.Lstat(path)` and skip if not a FIFO (preserves non-FIFO files the user may have legitimately created — though unlikely, belt-and-braces); (3) derive `paneKey` from the basename (strip `hydrate-` prefix and `.fifo` suffix); (4) if `paneKey` is in `liveMarkerKeys`, keep (it's a currently-restored pane awaiting hydration); (5) else `os.Remove` and log `"swept orphan FIFO %s"`; (6) per-file failures log and continue — one stuck FIFO must not block sweeping the rest. Caller from Phase 5's bootstrap integration builds `liveMarkerKeys` from `fetchSkeletonMarkers` (reused from task 3-11) after `Restore()` completes.

**Outcome**: After bootstrap, the state directory contains only FIFOs corresponding to live skeleton-marked panes. Orphans from prior bootstrap crashes, base-index changes, or dead-helper leaks are swept. Non-FIFO files matching the glob by coincidence are preserved. Sweep failure per-file is isolated and logged. Task 3-6's `Restore()` composition does not invoke this directly — Phase 5 wires it in as a step between Restore and `CleanStale`.

**Do**:
- Create `internal/state/fifo_sweep.go`:
  ```go
  package state

  import (
      "log"
      "os"
      "path/filepath"
      "strings"
  )

  // SweepOrphanFIFOs removes hydrate-*.fifo files in dir that do not correspond
  // to any paneKey in liveMarkerKeys. Non-FIFO files matching the glob are
  // preserved. Per-file failures log and continue.
  func SweepOrphanFIFOs(dir string, liveMarkerKeys map[string]struct{}, logger *log.Logger) error {
      matches, err := filepath.Glob(filepath.Join(dir, "hydrate-*.fifo"))
      if err != nil {
          return fmt.Errorf("sweep glob: %w", err)
      }
      for _, path := range matches {
          fi, err := os.Lstat(path)
          if err != nil {
              logger.Printf("fifo-sweep: lstat %s failed: %v", path, err)
              continue
          }
          if fi.Mode()&os.ModeNamedPipe == 0 {
              // Not a FIFO — preserve whatever this is.
              continue
          }
          base := filepath.Base(path)
          paneKey := strings.TrimSuffix(strings.TrimPrefix(base, "hydrate-"), ".fifo")
          if _, alive := liveMarkerKeys[paneKey]; alive {
              continue
          }
          if err := os.Remove(path); err != nil {
              logger.Printf("fifo-sweep: remove %s failed: %v", path, err)
              continue
          }
          logger.Printf("fifo-sweep: removed orphan %s", path)
      }
      return nil
  }
  ```
- Live-marker-keys set is built by the caller (Phase 5 bootstrap integration) from the server-scope option dump. Example caller code (lands in Phase 5):
  ```go
  markers, _ := fetchSkeletonMarkers(client)
  liveKeys := make(map[string]struct{}, len(markers))
  for k := range markers {
      liveKeys[strings.TrimPrefix(k, "@portal-skeleton-")] = struct{}{}
  }
  _ = state.SweepOrphanFIFOs(stateDir, liveKeys, logger)
  ```
- Tests in `internal/state/fifo_sweep_test.go` using `t.TempDir`:
  - State dir contains `hydrate-work__0.0.fifo` (live) and `hydrate-orphan__0.0.fifo` (orphan): only the orphan is removed; the live FIFO survives.
  - State dir contains `hydrate-orphan__0.0.fifo` + a non-FIFO file `hydrate-extra.fifo` (regular file — simulate by `os.Create` then not `syscall.Mkfifo`): only the orphan FIFO is removed; the regular file is preserved. Assert via `os.Lstat` post-sweep.
  - `paneKey` round-trip: state dir contains a FIFO whose paneKey sanitized-collides with an unrelated name. The `liveMarkerKeys` map uses the sanitized form (from task 2-1's sanitizer), matching what was set when the marker was created. Assert preservation.
  - State dir does not exist: `filepath.Glob` returns zero matches with nil error — function returns nil without error.
  - Symlink named `hydrate-fake.fifo` pointing to `/tmp/something`: `os.Lstat` reports the link (`Mode().IsRegular() == false`, but `ModeNamedPipe == 0` too — it's `ModeSymlink`). Preserved (not a FIFO). Test documents this.
  - Empty `liveMarkerKeys` map with present FIFOs: all present FIFOs are removed.
  - One FIFO fails `os.Remove` (chmod parent dir to 0500 mid-test on macOS/Linux if possible — tricky; alternative: use a subdirectory that is then made read-only so the child FIFO cannot be removed): log line emitted, other FIFOs still processed.

**Acceptance Criteria**:
- [ ] Glob `hydrate-*.fifo` under the state directory enumerates candidate paths.
- [ ] Per-path `os.Lstat` detects FIFO vs other file types; only FIFOs are candidates for removal.
- [ ] FIFOs whose paneKey is in `liveMarkerKeys` are preserved.
- [ ] FIFOs whose paneKey is not in `liveMarkerKeys` are removed via `os.Remove`.
- [ ] Per-file `os.Remove` failure is logged and the loop continues.
- [ ] Non-FIFO files matching the glob pattern are left untouched.
- [ ] State directory missing → glob returns empty → function returns `nil` without error.
- [ ] Empty `liveMarkerKeys` + present FIFOs removes all present FIFOs.
- [ ] paneKey derivation (`strings.TrimPrefix(basename, "hydrate-")` + `strings.TrimSuffix(_, ".fifo")`) matches the construction in task 2-1's `FIFOPath(dir, paneKey)`.
- [ ] Removal log line identifies the path for operator diagnosis.

**Tests**:
- `"it removes orphan FIFOs and preserves live-marked ones"`
- `"it preserves non-FIFO files matching the glob pattern"`
- `"it tolerates a missing state directory"`
- `"it removes all FIFOs when liveMarkerKeys is empty"`
- `"it round-trips sanitized paneKeys when comparing to liveMarkerKeys"`
- `"it logs and continues on a per-file removal failure"`
- `"it logs a line per removed orphan"`
- `"it treats symlinks matching the glob as non-FIFOs (preserved)"`

**Edge Cases**:
- State dir missing: `filepath.Glob` returns `nil, nil` for a non-existent directory — graceful.
- Non-FIFO file at a matching path (e.g., user manually created `hydrate-debug.fifo` as a regular file): preserved. `os.Lstat` filter enforces FIFO-only removal.
- Symlink matching the glob: preserved via the mode filter. Users (and no one else) might symlink in the state dir; respect their intent.
- paneKey sanitization round-trip: `SanitizePaneKey("a/b", 0, 0)` produced `a_b-<hash>__0.0` at marker-set time; the FIFO path is `hydrate-a_b-<hash>__0.0.fifo`; sweep's `TrimPrefix/TrimSuffix` extracts `a_b-<hash>__0.0`. If the caller built `liveMarkerKeys` correctly (stripping `@portal-skeleton-` prefix from the option name), the keys match byte-for-byte.
- Sweep failure per-file: a FIFO that can't be removed (unusual — FIFOs have no content) is logged but doesn't block sweeping the rest. Next bootstrap will retry.
- Live-pane FIFO preservation: a pane that was restored in *this* bootstrap has its marker set by task 3-5; `fetchSkeletonMarkers` picks it up; `liveMarkerKeys` contains its paneKey; sweep preserves its FIFO. Critical — if we remove a FIFO whose helper is actively blocked on it, the helper's read may see `ENOENT` on its side (though it's already blocked in `open`, so this is actually a subtle point — removing the FIFO's directory entry doesn't close an open fd; the helper continues reading from its open fd until the writer side sends bytes). Defensively we preserve live FIFOs anyway to avoid any filesystem-inode-reuse race.

**Context**:
> Spec "Restore-Side Architecture → Scope of Bootstrap Decisions vs. Implementation": "Stale-FIFO cleanup on bootstrap (state-directory scan to remove any leftover `hydrate-*.fifo` files that do not match an active pane)."
>
> Spec "Failure Modes & Recovery → Consolidated Failure-Handling Table → Orphan FIFO from a crashed helper": "Defensive `os.Remove` + `syscall.Mkfifo` on each bootstrap sweeps stale `hydrate-*.fifo` files before creating new ones. Additional state-dir scan on bootstrap removes any stale FIFOs not matching a restored pane."
>
> Spec "Save Format & Schema → FIFO Files": "Per-pane FIFOs for hydration (`hydrate-<paneKey>.fifo`) live in the state directory during the restoration window. They are created just before pane creation, unlinked by the helper on signal (or timeout), and swept defensively by `os.Remove + syscall.Mkfifo` on each bootstrap. Not part of the save schema; treated as transient coordination artifacts."
>
> Phase 5 wires the sweep into bootstrap after Restore completes and before `CleanStale` (step 7).

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Restore-Side Architecture → Scope of Bootstrap Decisions vs. Implementation", "Failure Modes & Recovery → Consolidated Failure-Handling Table", "Save Format & Schema → FIFO Files".

## built-in-session-resurrection-3-13 | approved

### Task 3-13: Integration test on isolated `tmux -L` socket — multi-session, multi-window save→restore round-trip of structure + ANSI scrollback

**Problem**: Tasks 3-1 through 3-12 land unit-tested primitives with mocked tmux. The spec's acceptance criterion for Phase 3 requires an **integration test** that proves the full save→restore→hydrate pipeline round-trips structure and ANSI-preserved scrollback on a real tmux process. Unit tests verify individual method calls; only an integration test verifies that `tmux new-session` + `split-window` + `select-layout` + `set-option -s` + FIFO + hydrate helper compose correctly on a real tmux event pipeline. The test must run on an **isolated tmux socket** (`tmux -L <unique>`) so it does not contaminate the user's live tmux state. Every socket must be killed in `t.Cleanup` on both success and failure. Test covers base-index / pane-base-index variations, multiple sessions, multiple windows per session, multiple panes per window, and ANSI SGR preservation in scrollback bytes.

**Solution**: Add `cmd/state_restore_integration_test.go` with a build-tag-gated integration test (`//go:build integration` or `t.Skip` if `TMUX_INTEGRATION` env var is unset — pick the convention consistent with existing integration tests in `cmd/root_integration_test.go`). Test flow: (1) build the portal binary via `go build -o <tempdir>/portal .` in `TestMain`; (2) each test allocates a unique socket name (`portal-test-<nanoid>`); (3) spawns `tmux -L <socket> new-session -d -s seed` to initialise a server; (4) creates two sessions with known structure (window/pane topology + ANSI-rich content typed into each pane); (5) saves state by invoking the built portal binary's internal save path (either by running `portal state daemon` briefly with mocked config paths, or by calling the save functions directly in-process via a test seam — the latter is simpler); (6) kills the tmux server entirely; (7) starts a fresh tmux server on the same socket; (8) runs `<binary> open` (or the equivalent bootstrap entry point) against that socket; (9) verifies: both sessions exist, structure matches (windows × panes), layouts applied, active panes selected, zoom state re-applied, skeleton markers set on each pane; (10) invokes `signal-hydrate` against each session; (11) captures each pane's scrollback via `tmux capture-pane -e -p` and compares to the saved bytes — asserting ANSI SGR (e.g., `\x1b[31m...\x1b[0m`) is preserved byte-for-byte; (12) verifies markers are unset after helpers complete; (13) verifies re-running `portal open` is a no-op (live-session skip path).

**Outcome**: A single integration test file gates every Phase 3 task from regressing end-to-end. CI runs it on each PR. Failure produces actionable diagnostics (which session, which window, which pane diverged). The test suite validates the composition without depending on the user's live tmux — and with aggressive `t.Cleanup` so a flaky test never leaves orphan tmux servers on CI.

**Do**:
- Create `cmd/state_restore_integration_test.go` (or `internal/restore/integration_test.go` — pick the package with the richest test helpers; `cmd/` has the existing `root_integration_test.go` pattern).
- Build tag / env-var gate consistent with existing integration tests — inspect `cmd/root_integration_test.go` and match its convention.
- Shared helpers:
  - `type tmuxSocket struct { Name string; Env []string }` — constructs with a unique socket name, prepends `TMUX_TMPDIR=<t.TempDir()>` to env so socket lives in temp.
  - `(s *tmuxSocket) Cmd(args ...string) *exec.Cmd` — returns `exec.Command("tmux", append([]string{"-L", s.Name}, args...)...)` with `s.Env` applied.
  - `(s *tmuxSocket) StartServer(t *testing.T)` — runs `tmux -L <name> new-session -d -s seed` to force server start; registers `t.Cleanup` that runs `tmux -L <name> kill-server` (tolerant of already-dead).
  - `(s *tmuxSocket) KillServer(t *testing.T)` — runs `tmux -L <name> kill-server` and waits for it to terminate.
  - `(s *tmuxSocket) ListSessions(t *testing.T) []string` — parses `tmux -L <name> list-sessions -F '#{session_name}'` output.
  - `(s *tmuxSocket) ListPanes(t *testing.T, session string) []PaneInfo` — parses `list-panes -t <session> -F '#{window_index}:#{pane_index}:#{window_layout}:#{window_zoomed_flag}:#{pane_active}:#{pane_current_path}'`.
  - `(s *tmuxSocket) CapturePane(t *testing.T, session string, window, pane int) []byte` — runs `capture-pane -e -p -S - -t <session>:<window>.<pane>` and returns raw bytes (including ANSI).
  - `(s *tmuxSocket) SetEnv(t *testing.T, session, key, value string)` — `set-environment -t <session> <key> <value>`.
  - `(s *tmuxSocket) RunShell(t *testing.T, cmdStr string)` — synchronous `run-shell` (useful for triggering `portal state signal-hydrate`).
- Test seam: the integration test invokes the orchestrator's three primitives directly — `Orchestrator.SetRestoring()` → `Orchestrator.Restore()` → `Orchestrator.ClearRestoring()` (per task 3-7's no-composite-wrapper decision) — plus `state.SweepOrphanFIFOs()`, all against a `tmux.Client` pointed at the isolated socket. Alternative: shell out to the built binary with `TMUX_STATE_DIR=<tempdir>` and appropriate env vars — more realistic but slower. Prefer the direct-call approach for this task's integration test to keep wall-clock under 5s per test; Phase 5 adds the full binary-subprocess test.
- Primary test `TestPhase3_SaveRestoreRoundTrip`:
  1. `sock := newTmuxSocket(t)`. `sock.StartServer(t)`.
  2. Set `@base-index 1`, `@pane-base-index 1` server options (to exercise the base-index drift edge case).
  3. Create session `alpha` with 2 windows: `w1` has 2 panes, `w2` has 3 panes. Active pane in `w1` is pane 2; `w2` is zoomed on pane 1.
  4. Create session `beta` with 1 window, 1 pane. Active = pane 1.
  5. In each pane, `send-keys` an ANSI-rich content string (e.g., `printf 'hello \x1b[31mRED\x1b[0m \x1b[1mBOLD\x1b[0m world\n' Enter`) and wait for the command to execute (small sleep; tests may poll `capture-pane` until the expected tail appears).
  6. Capture expected scrollback bytes per pane via `sock.CapturePane`.
  7. Set two per-session environment variables on `alpha` (`LANG=en_US.UTF-8`, `PORTAL_TEST=1`).
  8. Invoke the save path directly: `daemon.CaptureAndWrite()` or equivalent test entry point. Verify `sessions.json` exists, scrollback `.bin` files exist.
  9. `sock.KillServer(t)`. Confirm via `sock.Cmd("list-sessions").Run()` returning error.
  10. `sock.StartServer(t)` again on same socket (fresh server). Set `@base-index 0`, `@pane-base-index 0` this time (drift edge case).
  11. Invoke the restore path as the spec's three-step sequence: `orchestrator.SetRestoring()`, `orchestrator.Restore()`, `orchestrator.ClearRestoring()` — the same primitive sequence Phase 5 task 5-2's bootstrap calls, exposed for direct invocation by integration tests per task 3-7's no-composite-wrapper decision.
  12. Assert live sessions: `sock.ListSessions(t)` contains `alpha`, `beta`, `seed` (the original seed session from server start — new server's initial session). `_portal-saver` would appear if we started the daemon; for this test the daemon is NOT started (save was tested separately, restore is the focus here).
  13. Per-pane markers: for every expected pane, verify `@portal-skeleton-<livePaneKey>` is set via `sock.Cmd("show-options", "-sv").Output()` parsing.
  14. Per-window geometry: `sock.ListPanes(t, "alpha")` returns panes in saved structural order with the saved layout string; `w2` is zoomed; active panes match.
  15. Per-session environment: `sock.Cmd("show-environment", "-t", "alpha").Output()` includes `LANG=en_US.UTF-8` and `PORTAL_TEST=1`.
  16. Invoke `signal-hydrate alpha` and `signal-hydrate beta` via the test seam (or by running `portal state signal-hydrate` as a subprocess against the state dir).
  17. Wait for helpers to complete (poll for `@portal-skeleton-*` markers to disappear via `show-options -sv`; max 5s wait).
  18. Per-pane scrollback: `sock.CapturePane(t, session, window, pane)` returns bytes containing the saved ANSI content. Assert bytes include the raw `\x1b[31m` / `\x1b[0m` sequences from step 5.
  19. Re-run restore: invoke `orchestrator.SetRestoring()`, `orchestrator.Restore()`, `orchestrator.ClearRestoring()` again. Assert: zero new tmux calls beyond `list-sessions` (live-skip path). Structural state unchanged.
  20. `t.Cleanup` runs `sock.KillServer(t)` — belt-and-braces on top of `StartServer`'s registered cleanup.
- Additional focused tests (OPTIONAL — Phase 3 spec acceptance is satisfied by the primary round-trip test alone; the following expand coverage at the cost of additional integration-test surface area, CI runtime, and timing flakiness exposure):
  - `TestPhase3_HydrateTimeout` is the highest-value supplementary because the timeout path involves real tmux + real FIFO-block timing that unit tests cannot fully simulate. **Recommended: keep this one.**
  - `TestPhase3_SweepRemovesOrphanFIFOs`, `TestPhase3_CorruptSessionsJSON`, `TestPhase3_ScrollbackFileMissing` are unit-testable in isolation (tasks 3-12, 3-1, 3-10 already cover them at the unit level). **Recommended: drop these supplementary integration variants** in favor of the unit-test coverage already in those tasks.

  If the user prefers full integration coverage, all four supplementary tests can stay; if they prefer leaner integration suite, drop the three duplicates and keep only the round-trip + hydrate-timeout pair.
- CI consideration: integration tests SHOULD run on CI but MAY be skipped on local developer machines via env gate to avoid flakiness from tmux versions / OS differences. Follow `cmd/root_integration_test.go`'s convention.

**Acceptance Criteria**:
- [ ] Test uses an isolated `tmux -L <unique-name>` socket and never touches the user's live tmux.
- [ ] `t.Cleanup` runs `tmux -L <name> kill-server` on both success and failure paths.
- [ ] Test validates two distinct saved sessions with multi-window, multi-pane topology.
- [ ] Test covers base-index / pane-base-index drift (save at 1,1; restore at 0,0 — or vice versa).
- [ ] Scrollback bytes are compared with ANSI SGR sequences preserved — assert presence of `\x1b[31m` / `\x1b[0m` markers.
- [ ] `@portal-skeleton-<paneKey>` markers are set after restore and cleared after signal-hydrate + helper dump completes.
- [ ] Per-session environment round-trips via `show-environment -t <session>`.
- [ ] Zoom flag round-trips via `#{window_zoomed_flag}`.
- [ ] Active pane selection round-trips via `#{pane_active}`.
- [ ] Re-running restore on a live server is a no-op (live-skip path exercised).
- [ ] Supplementary tests cover: orphan FIFO sweep, corrupt sessions.json, hydrate timeout, scrollback file missing.
- [ ] Test gate (build tag or env var) matches existing integration-test convention in the repo.
- [ ] Test failure produces actionable diagnostics — log which session/window/pane diverged and what was expected vs. observed.

**Tests**:
- `"TestPhase3_SaveRestoreRoundTrip: two sessions round-trip structure, layout, zoom, active, CWD, env, and ANSI scrollback"`
- `"TestPhase3_SaveRestoreRoundTrip: live-session re-run is a no-op"`
- `"TestPhase3_SweepRemovesOrphanFIFOs: preserves live, removes orphans"`
- `"TestPhase3_CorruptSessionsJSON: logs the spec-mandated stderr warning and creates no sessions"`
- `"TestPhase3_HydrateTimeout: 3s timeout leaves marker set, shell starts"`
- `"TestPhase3_ScrollbackFileMissing: empty-pane path clears marker and starts shell"`
- `"TestPhase3_BaseIndexDrift: save at base-index 1, restore at base-index 0 works end-to-end"`

**Edge Cases**:
- Isolated socket: `tmux -L <name>` with `TMUX_TMPDIR=<t.TempDir()>` ensures the socket lives in a test-owned directory and cannot collide with the user's live tmux. Even if the test dies catastrophically, `t.Cleanup` fires in most cases; `testing.T.TempDir` also auto-removes its directory, taking any orphan socket with it.
- `t.Cleanup` order: registrations are LIFO. `StartServer(t)` registers `kill-server` first; later failures register additional cleanups on top. Final cleanup order is correct (kill server last).
- base-index / pane-base-index variations: save with one, restore with another — verifies the live-paneKey re-query in task 3-5 functions correctly. Tests should cover both directions (save=0 restore=1 AND save=1 restore=0).
- ANSI SGR preservation: `capture-pane -e` is load-bearing — the `-e` flag tells tmux to include escape sequences. Saved scrollback was captured with `-e` (per task 2-9). Test compares byte-for-byte.
- Marker cleared after signal-hydrate + helper dump: polling loop with a max timeout (5s — generous given 100ms settle sleep and sub-second dumps). On timeout, test fails with a diagnostic naming which pane's marker is stuck.
- Restore re-runnable: skeleton-restore is idempotent via the has-session check. Re-running should produce zero new tmux calls beyond `list-sessions`; test via a Commander wrapper that counts calls.
- CI vs local: env gate `TMUX_INTEGRATION=1` or build tag `//go:build integration`. Makefile / CI config should set this for PRs; local `go test ./...` can skip to avoid requiring tmux on dev machines.
- Scrollback bytes comparison: exact-match may be brittle to trailing whitespace; prefer contains-based assertions targeting the ANSI sequences and the content words (e.g., `"RED"`, `"BOLD"`). Document the choice and rationale in the test's comments.

**Context**:
> Spec "Scrollback Restore Mechanics → Validation Reference": "The mechanism was empirically validated on an isolated tmux socket during discussion:
> - `cat FILE; exec bash` pattern: 1000-line ANSI scrollback rendered correctly; clean `bash-5.3$` prompt at end.
> - Shell history contained only post-test commands — no helper, no `cat`, no scrollback content.
> - Blocking-FIFO variant: pane empty before signal; after `echo 'go' > fifo`, scrollback rendered and shell prompt appeared.
> - Default-socket sessions were verified identical before and after the test — validation does not contaminate the user's live tmux state. The isolated socket pattern (`tmux -L <unique-name>`) is the recommended approach for future mechanism verification."
>
> Spec "Phase 3 Acceptance": "Integration test on an isolated `tmux -L` socket verifies a multi-session, multi-window save round-trips structure + ANSI scrollback."
>
> Spec "Save Format & Schema → Index Semantics and base-index / pane-base-index": "If `base-index` / `pane-base-index` changed between save and restore, the *numeric* indices shift but the structural relationships (window order, pane order within a window, which pane was active) are preserved."
>
> Existing pattern: `cmd/root_integration_test.go` already uses the binary-subprocess pattern for integration tests. This task follows the same convention for gate and binary-build discipline.

**Spec Reference**: `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md` — sections "Scrollback Restore Mechanics → Validation Reference", "Save Format & Schema → Index Semantics and base-index / pane-base-index", "Restore-Side Architecture", and the Phase 3 acceptance bullet point in the planning document.
