---
phase: 4
phase_name: Diagnostic context preservation at boundaries
total: 6
---

## portal-observability-layer-4-1 | approved

### Task 4-1: Embed exit-status + trimmed stderr in the three production `exec.Cmd` boundary sites

**Problem**: Three production `exec.Cmd` boundaries discard the child's stderr on failure, so when these calls fail the diagnostic context is lost exactly where it is needed: `defaultIdentifyPS` (`internal/state/daemon_identity.go`) calls `ps … .Output()` and returns the bare `err` (the named cycle-1 defect that started this subtopic), `PgrepPortalDaemons` (`internal/state/pgrep.go`) calls `pgrep … .Output()` and wraps the error without stderr, and `resolver.RealCommandRunner.Run` (`internal/resolver/gitroot.go`) calls `cmd.Output()` and returns the raw `err`. Boundary class 1 of the spec mandates every `exec.Cmd` site embed exit status (or signal) + trimmed stderr in the wrapped error.

**Solution**: At each of the three sites, capture the child's stderr (either via a `bytes.Buffer` assigned to `cmd.Stderr` before `cmd.Output()`, or via `cmd.CombinedOutput()` where stdout/stderr separation is not needed) and wrap the error so it embeds the binary path, argv (`cmd.Args[1:]`), the exit status/signal, and the trimmed stderr text. Because exactly three sites share this identical pattern, this task MAY introduce the shared `internal/log.CombinedOutputWithContext` helper named in the spec (the 3-site threshold is met) — the task decides helper-vs-inline below.

**Outcome**: When `ps`, `pgrep`, or `git rev-parse` fails at these sites, the returned error chain contains the binary path, the argv, the exit status, and the trimmed stderr; the pre-existing classification contracts (`IdentifyDead` on canonical pid-not-found, `(nil, nil)` on pgrep status-1-no-matches, `(dir, nil)` on gitroot not-a-repo) are unchanged.

**Do**:
- **Decide helper-vs-inline first.** The spec permits the shared helper `func CombinedOutputWithContext(cmd *exec.Cmd) ([]byte, error)` in `internal/log` "after 3+ identical boundary-wrapping patterns appear". These three sites meet the threshold. Two of them (`defaultIdentifyPS`, `PgrepPortalDaemons`) need stdout AND a separate exit/stderr-bearing error, and one (`resolver.Run`) returns stdout-only. Choose ONE:
  - **Helper path (preferred):** add `internal/log.CombinedOutputWithContext(cmd *exec.Cmd) ([]byte, error)` that assigns a `bytes.Buffer` to `cmd.Stderr`, runs `cmd.Output()` (so stdout is returned separately from the captured stderr), and on error returns `(stdoutBytes, fmt.Errorf("%s %v: %w (stderr: %s)", cmd.Path, cmd.Args[1:], err, strings.TrimSpace(stderr.String())))`. Return the captured stdout on the error path too (callers like `defaultIdentifyPS` rely on stdout-on-non-zero-exit to discriminate pid-not-found). **Import-cycle guard:** `internal/log` MUST NOT import `internal/state` (Task 1-8's invariant); the helper takes a `*exec.Cmd` and depends only on stdlib, so no cycle. `internal/state` and `internal/resolver` already may import `internal/log` (they call `log.For`).
  - **Inline path:** write the same `bytes.Buffer` + wrap pattern verbatim at each of the three sites. Acceptable but duplicative; prefer the helper.
- **`defaultIdentifyPS` (`internal/state/daemon_identity.go:64-67`):** replace `exec.Command("ps", …).Output()` with the helper (or inline buffer). Return `(string(out), wrappedErr)`. **Preserve the documented contract** in the `identifyPS` godoc: on non-zero exit, return whatever stdout was captured (may be empty) AND a non-nil error; `IdentifyDaemon` discriminates "PID not found" (non-zero exit + empty stdout) from a transient failure (non-zero exit + non-empty stdout) using this pair. The wrapping must not alter stdout — only the returned `err` gains stderr/argv/exit context. The canonical "pid not found" shape (`ps -p <gone>` exits non-zero with empty stdout) must still produce empty stdout + non-nil err so `IdentifyDaemon` returns `IdentifyDead`.
- **`PgrepPortalDaemons` (`internal/state/pgrep.go:49-59`):** the status-1-no-matches branch is load-bearing and MUST stay first. Keep the existing `var exitErr *exec.ExitError; if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(TrimSpace(stdout)) == 0 { return nil, nil }` check exactly as-is (it returns `(nil, nil)` on pgrep's documented "nothing found" signal — wrapping it as an error would force a WARN on every clean bootstrap). Only the FINAL fallthrough wrap (`return nil, fmt.Errorf("pgrep -fx %q: %w", PortalDaemonArgvPattern, err)`) gains stderr context — it currently embeds the argv but not stderr. If using the helper, ensure the helper returns stdout so the status-1 check can still inspect it; if the helper is awkward against the early-return shape, inline the buffer here and only the fallthrough wrap is enriched.
- **`resolver.RealCommandRunner.Run` (`internal/resolver/gitroot.go:20-27`):** replace `cmd.Output()` + bare `return "", err` with the helper (or inline buffer) so the returned error embeds `git`'s argv + stderr. **Do NOT change `ResolveGitRoot`'s swallow behaviour:** `ResolveGitRoot` (same file, lines 38-41) catches the `runner.Run` error and returns `(dir, nil)` — the expected "not a git repo / git absent" outcome stays swallowed to the original `dir` (this is the `expected` classification; no log line is added in this task). Only `RealCommandRunner.Run`'s returned error gains context, for the benefit of any future caller that surfaces it.
- Do NOT add any `logger.Warn`/`logger.Debug` calls in this task — this is error *wrapping*, not log emission. The level discipline + later-phase instrumentation determine how these wrapped errors reach `portal.log`. (When a future log site logs one of these errors, it passes the wrapped `err` directly to the `"error"` attr per the Phase 1 convention — never `err.Error()`.)
- **PATH-lookup edge:** a `*exec.Error` (binary not on PATH, e.g. `ps`/`pgrep`/`git` missing) has no exit code and no stderr. The wrap must render cleanly in that case — `cmd.Path`/`cmd.Args` are still populated, stderr is empty, and the `%w` preserves the `*exec.Error`. Verify `errors.As(err, &*exec.Error)` still succeeds through the wrap.

**Acceptance Criteria**:
- [ ] On a non-zero `ps`/`pgrep`/`git` exit, the returned error string contains the binary path, the argv, the underlying exit-status error, and the trimmed stderr text.
- [ ] `defaultIdentifyPS` still returns captured stdout alongside the error on non-zero exit; `IdentifyDaemon` still classifies the canonical "pid not found" shape (non-zero exit + empty stdout) as `IdentifyDead` and a non-zero-exit-with-stdout shape as transient.
- [ ] `PgrepPortalDaemons` still returns `(nil, nil)` on pgrep status-1 with empty stdout (no-matches); only the non-status-1 / OS-layer failure path returns the stderr-enriched wrapped error.
- [ ] `ResolveGitRoot` still returns `(dir, nil)` when `git rev-parse` fails (not-a-repo / git absent) — the swallow is unchanged; only `RealCommandRunner.Run`'s error is enriched.
- [ ] A PATH-lookup failure (`*exec.Error`) wraps cleanly with empty stderr and remains recoverable via `errors.As(err, &execErr)`.
- [ ] No `_, _ = cmd.Run()` or `cmd.Output()`-without-stderr-capture remains at these three sites; `errors.As` against `*exec.ExitError` still traverses the wrapped error.

**Tests**:
- `"it embeds argv and trimmed stderr when ps exits non-zero with stderr"` (via the `identifyPS` seam returning a synthetic non-zero exit + stderr, or a real `ps` against an invalid arg)
- `"it preserves empty-stdout-on-pid-not-found so IdentifyDaemon returns IdentifyDead"`
- `"it returns transient error with stderr when ps exits non-zero with stdout"`
- `"PgrepPortalDaemons returns (nil,nil) on status-1 no-matches"`
- `"PgrepPortalDaemons wraps with stderr on an OS-layer/non-status-1 failure"`
- `"RealCommandRunner.Run embeds git argv and stderr on non-zero exit"`
- `"ResolveGitRoot still returns dir unchanged when git rev-parse fails"`
- `"a PATH-lookup *exec.Error wraps cleanly with empty stderr and stays errors.As-recoverable"`

**Edge Cases**:
- PATH-lookup `*exec.Error` has no exit code and no stderr — wrap must render cleanly (empty stderr) and stay `errors.As`-recoverable.
- Signal-killed child (e.g. SIGKILL) vs non-zero exit: both render via `%w` of the underlying `*exec.ExitError`; the wrap text must not assume a numeric exit code.
- Empty stderr renders cleanly (no dangling `(stderr: )` noise beyond the documented format).
- pgrep status-1-no-matches stays `(nil, nil)` — NOT wrapped as an error.
- gitroot expected not-a-repo failure still swallowed to the original `dir` (no log line).
- Helper-vs-inline: the optional `log.CombinedOutputWithContext` helper is permitted at the 3-site threshold; if introduced it must not import `internal/state` (cycle guard).

**Context**:
> Boundary class 1 worked example (spec § Diagnostic context preservation → Mechanical rule):
> ```go
> var stderr bytes.Buffer
> cmd.Stderr = &stderr
> out, err := cmd.Output()
> if err != nil {
>     return nil, fmt.Errorf("%s %v: %w (stderr: %s)", cmd.Path, cmd.Args[1:], err, strings.TrimSpace(stderr.String()))
> }
> ```
> "PROHIBITED: `_, _ = cmd.Run()`, `cmd.Output()` without `cmd.Stderr` assignment, or any error path that returns a wrapped error WITHOUT stderr text included."
>
> Enumerated gap-closure site: "`defaultIdentifyPS` (`internal/state/daemon_identity.go`) | stderr discarded on failure | Boundary class 1 — embed trimmed stderr in the wrapped error (the worked example already shown above)." (spec § Diagnostic context preservation → Enumerated gap-closure sites)
>
> "After 3+ identical boundary-wrapping patterns appear in production code, a shared helper in `internal/log` MAY be added: `func CombinedOutputWithContext(cmd *exec.Cmd) ([]byte, error)`. Until 3+ sites need it, write the wrapping at each call site directly." (spec § Diagnostic context preservation → Boundary helper)
>
> `IdentifyDaemon` contract (`internal/state/daemon_identity.go`): on non-zero exit + empty stdout → `IdentifyDead`; on non-zero exit + non-empty stdout → transient error. `PgrepPortalDaemons` three-shape contract (`internal/state/pgrep.go`): `([]int, nil)` / `(nil, nil)` on status-1-empty / `(nil, err)` otherwise.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Diagnostic context preservation at boundaries (Mechanical rule — Boundary class 1; Boundary helper; Enumerated gap-closure sites — `defaultIdentifyPS`)

---

## portal-observability-layer-4-2 | approved

### Task 4-2: Embed exit code + tmux argv + trimmed stderr in `RealCommander.Run`/`RunRaw` and verify sentinel detection

**Problem**: The tmux commander (`internal/tmux/tmux.go` `runCommand` → `WrapCommandError`) already captures the child's stderr into `(*exec.ExitError).Stderr` and exposes it via the `*CommandError` type, but the wrapped error does NOT structurally carry the tmux **argv** or the **exit code** — so when a tmux command fails, the log site cannot tell *which* tmux invocation failed from the `*CommandError` alone (callers add argv context at their own wrap layer, but the commander-level error omits it). Boundary class 2 of the spec mandates the commander embed the exit code, the tmux argv, and the trimmed stderr on non-zero exit, and that `ErrNoSuchSession`/`ErrEmptyPaneList` sentinels be detectable from the stderr text.

**Solution**: Thread the tmux argv (and the underlying exit-code-bearing error) into the `*CommandError` shape so the commander-level error embeds argv + exit code + stderr, WITHOUT assigning `cmd.Stderr` (which would defeat `(*exec.ExitError).Stderr` auto-population — the load-bearing invariant). Verify and regression-lock that the existing `wrapNoSuchSession` sentinel detection (and the `ErrEmptyPaneList` path in `saverPanePID`) still recover both the sentinel (via `errors.Is`) and the `*CommandError` (via `errors.As`) on the same chain.

**Outcome**: A failing `RealCommander.Run`/`RunRaw` returns a `*CommandError` whose rendered error contains the tmux argv, the exit code, and the trimmed stderr; `cmd.Stderr` is still left nil so `ExitError.Stderr` auto-populates; `errors.As(err, &cmdErr)` and `errors.Is(err, ErrNoSuchSession)`/`errors.Is(err, ErrEmptyPaneList)` both still succeed.

**Do**:
- **Preserve the `cmd.Stderr`-nil invariant.** `runCommand` (`internal/tmux/tmux.go:79-90`) deliberately leaves `cmd.Stderr` nil — see the in-source comment and `WrapCommandError`'s godoc precondition: exec auto-populates `(*exec.ExitError).Stderr` ONLY when `cmd.Stderr == nil`; assigning a buffer silently zeroes it and defeats the wrap. Do NOT assign `cmd.Stderr`. The stderr already flows correctly; this task adds argv + exit code, not stderr capture.
- **Pass the argv into the wrap.** `runCommand` already holds `args` (the tmux argv) and `binary` (`"tmux"`). Thread these into `WrapCommandError` so the resulting `*CommandError` carries them. Two viable shapes — pick the lower-churn one:
  - **Extend `*CommandError`** (`internal/tmux/command_error.go`) with an `Args []string` field (and optionally the resolved binary), populated by a new `WrapCommandErrorWithArgs(err error, binary string, args []string) error` (or by adding params to `WrapCommandError`). `CommandError.Error()` renders the argv before the stderr. The struct stays constructable as a plain literal (the documented contract — test mocks build it directly), so add the field as exported and keep the zero value benign (empty `Args` renders without argv noise).
  - **Wrap at the `runCommand` site** with an outer `fmt.Errorf("tmux %v: %w", args, WrapCommandError(err))` so the argv rides on the outer message while the `*CommandError` (with stderr) stays recoverable via `errors.As`. Simpler, but the argv is then only in the message string, not a structured field. Prefer extending `*CommandError` so the argv is programmatically accessible (consistent with how `Stderr` is already a field).
- **Exit code.** The underlying `*exec.ExitError` already carries the exit code (`ExitError.ExitCode()`); since `WrapCommandError` wraps `err` via `%w` / the `Err` field, the exit code is already reachable. Ensure `CommandError.Error()` surfaces it in the rendered string (e.g. include `exit N` when the embedded error unwraps to `*exec.ExitError`), so a log line shows the exit code without the reader needing `errors.As`. For a PATH-lookup `*exec.Error` (no exit code), render argv-only — no `exit N` fragment.
- **Verify (do not re-implement) the `wrapNoSuchSession` chain** (`internal/tmux/errors.go:84-93`): it inspects `errors.As(err, &cmdErr)` and, when `cmdErr.Stderr` contains `"no such session"`, returns `fmt.Errorf("%w: %w", ErrNoSuchSession, err)` (multi-`%w`). After the argv change, confirm both `errors.Is(err, ErrNoSuchSession)` AND `errors.As(err, &cmdErr)` still succeed on the returned chain (the multi-`%w` keeps both reachable). `ShowEnvironment` (`tmux.go:673-684`) is the production caller that funnels through `wrapNoSuchSession` — its test surface is the regression anchor.
- **Verify the `ErrEmptyPaneList` path.** `ErrEmptyPaneList` (`internal/tmux/errors.go:48`) is returned by `saverPanePID` when tmux succeeds but reports zero panes — it is NOT detected from stderr (it is a no-stderr success-with-empty-output case), so it does not flow through the commander-level stderr wrap. Confirm the task's commander change does not perturb the `saverPanePID` → `ErrEmptyPaneList` path (it lives above `runCommand` and keys on empty stdout, not on a `*CommandError`). Add/keep a regression test asserting `errors.Is(err, ErrEmptyPaneList)` still holds.
- **`RunRaw` verbatim path.** `RunRaw` shares `runCommand` (with `trim=false`); the verbatim stdout-on-success path is unaffected — only the error path gains argv context. Verify `RunRaw`'s success output is byte-identical (no trim) after the change.
- **`socketCommander` parity.** `internal/tmuxtest.socketCommander` and the `transienttest` commander also call through `WrapCommandError` (per its godoc: "single source of truth for the production wrap shape") so test discriminators behave identically. If `WrapCommandError`'s signature changes, update those call sites so the test commander still produces the same `*CommandError` shape (now with argv) as production.

**Acceptance Criteria**:
- [ ] A non-zero tmux exit returns a `*CommandError` whose rendered `Error()` contains the tmux argv, the exit code, and the trimmed stderr.
- [ ] `cmd.Stderr` is still left nil in `runCommand`; `(*exec.ExitError).Stderr` still auto-populates and feeds `CommandError.Stderr`.
- [ ] `errors.As(err, &cmdErr)` recovers the `*CommandError` (with `Stderr` and the new argv) on the returned chain.
- [ ] `errors.Is(err, ErrNoSuchSession)` still succeeds for a "no such session" stderr through `wrapNoSuchSession`'s multi-`%w` chain, AND `errors.As(err, &cmdErr)` still succeeds on the same value.
- [ ] `errors.Is(err, ErrEmptyPaneList)` still succeeds for the empty-pane-list case (path unperturbed by the commander change).
- [ ] A PATH-lookup `*exec.Error` (tmux missing) produces a `*CommandError` carrying the argv with empty stderr and no `exit N` fragment.
- [ ] `RunRaw` success output is verbatim (untrimmed) and unchanged.
- [ ] `*CommandError` remains constructable as a plain struct literal (test mocks compile unchanged or with only the new field added).

**Tests**:
- `"Run embeds tmux argv, exit code, and stderr on non-zero exit"`
- `"runCommand leaves cmd.Stderr nil so ExitError.Stderr auto-populates"`
- `"errors.As recovers *CommandError with argv and stderr after the wrap"`
- `"errors.Is(ErrNoSuchSession) and errors.As(*CommandError) both succeed on the same chain"`
- `"errors.Is(ErrEmptyPaneList) still holds for the empty-pane-list case"`
- `"a PATH-lookup *exec.Error renders argv with empty stderr and no exit fragment"`
- `"RunRaw returns verbatim untrimmed output on success"`
- `"argv with spaces and quotes renders intact in the error"`

**Edge Cases**:
- `cmd.Stderr`-nil precondition preserved — assigning a buffer would zero `ExitError.Stderr` and defeat the wrap (do NOT do it).
- PATH-lookup `*exec.Error` carries argv but empty stderr and no exit code.
- Multi-`%w` sentinel chain (`ErrNoSuchSession`) still recoverable via both `errors.Is` (sentinel) and `errors.As` (`*CommandError`).
- `RunRaw` verbatim path unaffected (no trim on success).
- argv containing spaces / quotes / shell metacharacters rendered intact (it is data, not re-parsed).
- `*CommandError` plain-struct-literal constructability preserved for test mocks (no required factory).

**Context**:
> Boundary class 2 (spec § Diagnostic context preservation → Mechanical rule): "The commander MUST capture both stdout and stderr on every invocation. On non-zero exit: the returned error MUST embed the exit code, the tmux argv, and the trimmed stderr text. Tmux-specific sentinel errors (`ErrNoSuchSession`, `ErrEmptyPaneList` per `internal/tmuxerr`) MUST be detected via the stderr text and wrapped with the sentinel using `fmt.Errorf("%w: %s", sentinel, stderr)`. PROHIBITED: returning a generic error from a tmux invocation without the stderr context."
>
> `WrapCommandError` precondition (`internal/tmux/command_error.go` godoc): "the `*exec.Cmd` whose `Output()` produced err must have left `cmd.Stderr == nil`. exec.Cmd auto-populates `(*exec.ExitError).Stderr` only under that condition — assigning `cmd.Stderr` … silently zeroes `exitErr.Stderr` and defeats the wrap." This helper is "the single source of truth for the production wrap shape. Both `internal/tmux.runCommand` and `internal/tmuxtest.socketCommander` call through it."
>
> `runCommand` invariant comment (`internal/tmux/tmux.go`): "cmd.Stderr is deliberately left nil — see WrapCommandError's godoc." `wrapNoSuchSession` (`internal/tmux/errors.go`): "The Go 1.20+ multi-%w form is required so both the sentinel and the underlying chain remain reachable on the same error value."

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Diagnostic context preservation at boundaries (Mechanical rule — Boundary class 2)

---

## portal-observability-layer-4-3 | approved

### Task 4-3: Audit `os`-syscall and `io`/FIFO read boundaries for `%w` path/errno preservation and EOF/timeout=`expected` classification

**Problem**: Boundary classes 3 (`os` syscalls) and 4 (`io`/FIFO/scrollback reads) of the spec require that `os`-package errors preserve their underlying `*os.PathError` (path + errno) via `%w`, never replaced by a context-losing `errors.New(...)`, and that EOF / FIFO-open-timeout outcomes are treated as *expected* (valid `(nil, nil)`-style returns, not boundary failures) while genuine mid-stream read errors wrap with path context. The codebase is already **largely compliant** here — this is a verify-and-close-gaps pass that audits each `os`/`io` read site, fixes any that drop the `*os.PathError`, and adds regression-locking tests so the compliant behaviour cannot silently regress.

**Solution**: Audit each enumerated `os`/`io` read boundary, confirm it wraps with `%w` (preserving the underlying `*os.PathError`/sentinel) or correctly classifies an expected EOF/ENOENT/timeout as a non-error `(nil, nil)` / sentinel return, fix any gap found, and add focused regression tests asserting `errors.Is` traversal to `fs.ErrNotExist` / `fs.ErrPermission` and the expected-outcome returns. No behaviour changes where the code is already correct — the deliverable is the audit + the regression-lock tests.

**Outcome**: Every audited `os`/`io` read site provably preserves the `*os.PathError` chain (`errors.Is` reaches `fs.ErrNotExist`/`fs.ErrPermission`) or correctly returns the expected non-error outcome; no `errors.New(...)` that discards an `*os.PathError` remains in the audited scope; regression tests lock each contract.

**Do**:
- **Audit (verify-and-close-gaps) these sites:**
  - `internal/state/scrollback_tail.go` `TailScrollback`: ENOENT-on-open → `(nil, nil)` (expected, line 57-59); zero-byte file → `(nil, nil)` (line 69-71); zero-newline file → `(nil, nil)` (line 119-122). Any other open error and every `Seek`/`io.ReadFull` error wrap with `fmt.Errorf("tail scrollback %s: %w", path, err)` (lines 61, 67, 92, 96). Confirm `errors.Is(err, fs.ErrPermission)` and `errors.Is(err, os.ErrClosed)` traverse the wraps. **Already compliant — add regression tests only.**
  - `internal/state/scrollback.go` `SeedHashMap`: `os.ReadDir` ENOENT → empty map, no warn (expected, line 49); other dir errors and per-file `os.ReadFile` errors are logged at WARN and skipped (lines 52, 67). After Phase 1 these are `*slog.Logger` calls passing the wrapped `err` to the `"error"` attr — confirm the `error_class="unexpected"` classification matches the level table (a per-file read failure drops that pane's seed entry = a dropped unit → WARN with `error_class="unexpected"`; the directory-read failure degrades the whole seed → WARN `unexpected`). `WriteScrollbackIfChanged` wraps with `fmt.Errorf("write scrollback %s: %w", paneKey, err)` (line 107) — compliant.
  - `internal/state/fifo.go` `CreateFIFO`: `os.Remove` ENOENT → swallowed (expected, line 33); other remove errors and `syscall.Mkfifo` errors wrap with path (lines 34, 37); the defensive `os.Chmod` error is intentionally ignored (line 39 — this is a *defensive branch* documented in-source; see Task 4-6 for the comment-closure scope). Compliant.
  - `internal/state/fifo_sweep.go` `SweepOrphanFIFOs`: `filepath.Glob` error wraps with `fmt.Errorf("glob fifos in %s: %w", dir, err)` (line 30); per-file `os.Lstat`/`os.Remove` errors are WARN-and-continue (lines 36, 48). After Phase 1, these are slog WARNs with the wrapped `err` in the `"error"` attr. Compliant.
  - `internal/state/daemon_state.go` `readDaemonFile`: ENOENT → `absentSentinel` (`ErrPIDFileAbsent`/`ErrVersionFileAbsent`, expected, line 130-132); other I/O errors wrap with `fmt.Errorf("read %s: %w", filepath.Base(path), err)` (line 133). `ReadPIDFile` parse error wraps `parse daemon.pid: %w` (line 49). Compliant — confirm `errors.Is(err, ErrPIDFileAbsent)` and `errors.Is(err, fs.ErrPermission)` both work as appropriate.
  - `internal/restore/session.go`: tmux-call wraps are boundary class 2 (covered by 4-2); the file path joins (`collectArmInfos`) involve no `os` read. No `os`/`io` read boundary to fix here — confirm and note.
  - `cmd/state_hydrate.go` `runHydrate`: FIFO open timeout → `ErrHydrateTimeout` (expected, routed to `HandleTimeout` — line 102); FIFO open non-timeout error wraps `open fifo %s: %w` (line 120); the 1-byte signal `f.Read` error is intentionally ignored (`_, _ = f.Read(buf)`, line 126 — documented "even a 0-byte read can mean writer closed"; defensive branch, Task 4-6 scope); scrollback `os.Open` ENOENT/permission/I-O classified by `handleHydrateFileMissing` via `errors.Is(ctx.Cause, fs.ErrNotExist)`/`fs.ErrPermission` (lines 295-300) — the `Cause` must be the raw `*os.PathError` for those `errors.Is` checks to work; confirm `runHydrate` passes `err` verbatim into `hydrateFileMissingContext{Cause: err}` (line 141) WITHOUT pre-wrapping (a `fmt.Errorf` wrap would still let `errors.Is` traverse, but the handler's switch relies on the chain — verify). The non-handler fallthrough wraps `open scrollback %s: %w` (line 150). `io.Copy` mid-stream error → also `HandleFileMissing` (line 158-168) — the mid-stream `Read` error is the boundary-class-4 "read error mid-stream" case; confirm `Cause` carries the underlying error.
- **Fix any gap found.** If any audited site uses `errors.New(...)` / `fmt.Errorf` WITHOUT `%w` where it drops an `*os.PathError`, or mis-classifies an expected EOF/ENOENT/timeout as a failure, fix it to `fmt.Errorf("…: %w", …, err)` (class 3) or the expected-outcome return (class 4). The expectation from the read is that the codebase is already compliant; the deliverable is the verification + regression tests, plus any small fix the audit surfaces.
- **EOF semantics (class 4):** confirm `TailScrollback`'s reverse-scan treats a zero-newline / partial-final-record file as the expected `(nil, nil)` "no terminated content" outcome — NOT an error. `io.ReadFull` returning `io.ErrUnexpectedEOF` mid-chunk IS a genuine boundary failure (a chunk we expected to be present went short) and correctly wraps with path. Lock both with tests.
- **Do NOT add new log emission** in this task — log sites that consume these errors already exist (post-Phase-1) and pass the wrapped `err` to the `"error"` attr directly. This task only guarantees the *wrapped error preserves context*; it does not add or move log calls.

**Acceptance Criteria**:
- [ ] Every audited `os`-read error preserves its `*os.PathError` via `%w`; `errors.Is(err, fs.ErrNotExist)` / `errors.Is(err, fs.ErrPermission)` traverse the wrap where applicable.
- [ ] ENOENT-on-open at the expected sites (`TailScrollback`, `SeedHashMap` dir, `readDaemonFile`, `CreateFIFO` remove) returns the expected non-error outcome (`(nil, nil)` / empty map / absent-sentinel / swallow) — NOT a wrapped failure.
- [ ] FIFO open timeout returns `ErrHydrateTimeout` (expected) and routes to the timeout handler, not a boundary failure.
- [ ] EOF terminator / zero-newline file in `TailScrollback` is the expected `(nil, nil)` outcome; a genuine mid-chunk `io.ReadFull` short read wraps with path.
- [ ] A mid-stream `io.Copy`/`Read` error in the hydrate path carries the underlying error to the handler's `errors.Is` switch (classification by `fs.ErrNotExist`/`fs.ErrPermission`/generic still works).
- [ ] No `errors.New(...)` that discards an `*os.PathError` remains in the audited scope.

**Tests**:
- `"TailScrollback returns (nil,nil) on ENOENT, zero-byte, and zero-newline files"`
- `"TailScrollback wraps a Seek/ReadFull mid-scan error with path and stays errors.Is(fs.ErrPermission)"`
- `"readDaemonFile returns the absent sentinel on ENOENT and wraps other I/O errors with path"`
- `"CreateFIFO swallows ENOENT on remove and wraps other remove/mkfifo errors with path"`
- `"runHydrate routes a FIFO open timeout to HandleTimeout, not a boundary error"`
- `"handleHydrateFileMissing classifies ENOENT vs permission vs generic from the raw Cause chain"`
- `"a mid-stream io.Copy failure carries the underlying error into the file-missing handler"`
- `"no audited os-read site drops the *os.PathError"` (errors.Is traversal assertions)

**Edge Cases**:
- ENOENT-on-open = expected `(nil, nil)` / sentinel, not wrapped.
- Mid-stream `Read`/`io.ReadFull` error wraps with path (genuine boundary failure).
- EOF terminator / zero-newline file is expected, not a failure.
- FIFO open timeout = expected (`ErrHydrateTimeout`).
- `errors.Is` unwraps through `%w` to `fs.ErrPermission` / `fs.ErrNotExist`.
- No `errors.New(...)` that drops the `*os.PathError`.
- The hydrate `_, _ = f.Read(buf)` and `CreateFIFO`'s defensive `os.Chmod` ignore are intentional defensive branches — their *comments* are Task 4-6's scope; this task confirms they are correct, not that they wrap.

**Context**:
> Boundary class 3 (spec § Diagnostic context preservation → Mechanical rule): "Go's `os` package wraps syscall errors with path + errno text by default … Do NOT replace these with a wrapper that loses the path/errno context. When adding context, use `fmt.Errorf("...: %w", ..., err)` so the underlying error is preserved verbatim and accessible via `errors.Unwrap`. PROHIBITED: `return errors.New("file operation failed")`-style wrapping that discards the original `*os.PathError`."
>
> Boundary class 4: "EOF and timeout conditions are valid expected outcomes, not boundary failures — they take the `expected` classification in the level discipline. Other I/O errors (read error mid-stream, write error mid-write) wrap with `fmt.Errorf("read %s: %w", path, err)` to preserve path context."
>
> `TailScrollback` contract (`internal/state/scrollback_tail.go`): "All 'no content available' outcomes converge on `(nil, nil)` with NO error: ENOENT on open, a zero-byte file, and a file whose reverse scan finds zero '\n' bytes. … Any other open error … and any Seek/Read error … propagate as `(nil, err)` wrapped with the unified prefix 'tail scrollback <path>: ...' and %w, so errors.Is works against fs.ErrPermission, os.ErrClosed, etc."

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Diagnostic context preservation at boundaries (Mechanical rule — Boundary class 3 and Boundary class 4)

---

## portal-observability-layer-4-4 | approved

### Task 4-4: Add the SIGKILL-escalation DEBUG breadcrumb in `escalateKillToSIGKILL`

**Problem**: `escalateKillToSIGKILL` (`internal/tmux/portal_saver.go`) performs the Component A kill-barrier escalation — when a prior daemon survives the `kill-session` poll, it identity-checks the PID and, only on `IdentifyIsPortalDaemon`, sends a direct SIGKILL. The skip branch (not a portal daemon / transient error / dead) emits a WARN, but the *escalation* branch (the one that actually fires SIGKILL) has **no breadcrumb** — so a forensic reader sees the `saver: kill-barrier escalated` INFO lifecycle event (added in Phase 5) but no record beneath it of the moment the SIGKILL decision was committed. This is the named gap-closure defect: "no breadcrumb on the SIGKILL escalation path."

**Solution**: Add ONE DEBUG breadcrumb at the escalation decision in `escalateKillToSIGKILL`, beneath the `saver: kill-barrier escalated` INFO lifecycle event, carrying the `target_pid` (the PID about to be SIGKILL'd). The breadcrumb is DEBUG (a decision-point detail under the INFO lifecycle line, per the level discipline), uses the post-Phase-1 `*slog.Logger`, and must respect the load-bearing "no work between identity-check and SIGKILL" invariant.

**Outcome**: When the escalation branch fires, a DEBUG line is emitted naming the `target_pid` before the SIGKILL syscall; the skip branch is unchanged; the identity-check → SIGKILL adjacency invariant is preserved.

**Do**:
- **Locate the escalation branch.** In `escalateKillToSIGKILL` (`internal/tmux/portal_saver.go:424-446`): the function calls `saver.IdentifyDaemon(priorPID)`; on `err != nil || result != state.IdentifyIsPortalDaemon` it emits the existing skip-WARN and returns nil; otherwise (the escalation branch) it calls `saver.Barrier.SendSIGKILL(priorPID)`. The breadcrumb belongs on the escalation branch, immediately before `SendSIGKILL`.
- **Emit the DEBUG breadcrumb.** After Phase 1, this file's saver/bootstrap WARN sink is a `*slog.Logger` (the `BarrierLogger` interface is retyped to `*slog.Logger` in Task 1-8; the package binds a `log.For` logger). Emit:
  ```go
  logger.Debug("kill-barrier escalating to SIGKILL", "target_pid", priorPID)
  ```
  using `target_pid` from the closed Lifecycle attr group. The component is `saver` (this is saver-lifecycle territory; the `saver: kill-barrier escalated` INFO event it sits beneath is component `saver` per the Phase 5 catalog). If the package-level logger var in this file is bound to a different component after Phase 1 (the existing WARNs render under `bootstrap` via `state.ComponentBootstrap`), bind/use a `log.For("saver")` logger for this breadcrumb so it renders under `saver:` consistent with the Phase 5 `kill-barrier escalated` lifecycle event it annotates. `[needs-info]` is NOT raised — the spec explicitly homes `kill-barrier escalated` under `saver` (Saver lifecycle events table) and this breadcrumb sits "beneath" that event, so `saver` is the correct component.
- **Preserve the identity-check → SIGKILL adjacency invariant.** The spec and the in-source comment (`portal_saver.go:419-423`) state: "no work (other than the syscall itself) runs between the check and the signal … The two seam calls are deliberately adjacent in source." A `logger.Debug(...)` call IS a statement between the check and the signal. Resolve by placing the breadcrumb as the IMMEDIATELY-preceding statement to `SendSIGKILL` (i.e. the order is: identity-check passes → `logger.Debug(...)` → `SendSIGKILL(priorPID)`), and update the in-source comment to acknowledge that the single DEBUG breadcrumb is the *only* permitted intervening statement and is non-mutating (it does not touch the PID, the process table, or any state that could let the PID recycle). The breadcrumb is a kernel-side write to an `O_APPEND` fd (unbuffered, microseconds) — it does not yield a scheduling window that materially widens the recycle race versus the pre-existing function-call overhead. Document this explicitly so a future reviewer does not "fix" the invariant by deleting the breadcrumb.
- **Skip branch unchanged.** Do NOT add a breadcrumb to the skip branch — it already emits a WARN (`prior daemon (pid=%d) not identity-checked …`). Only the escalation (SIGKILL-firing) branch gains the DEBUG breadcrumb.
- **Nil/discard sink safety.** Post-Phase-1 the logger is a real `*slog.Logger` (never nil — the nil-receiver contract was removed in Task 1-8; "silent" is an `io.Discard` slog logger). The `Debug` call is safe regardless of level (it is filtered out at INFO production default). Confirm the breadcrumb does not fire on the skip branch under any input.

**Acceptance Criteria**:
- [ ] The DEBUG breadcrumb fires ONLY on the `IdentifyIsPortalDaemon` escalation branch (the SIGKILL-firing path), never on the skip-WARN branch.
- [ ] The breadcrumb carries the `target_pid` attr equal to the PID being SIGKILL'd.
- [ ] The breadcrumb is the immediately-preceding statement to `SendSIGKILL` (the only intervening statement between the identity-check passing and the signal), and the in-source adjacency-invariant comment is updated to reflect this.
- [ ] The breadcrumb renders under the `saver:` component (consistent with the `saver: kill-barrier escalated` lifecycle event it annotates).
- [ ] The breadcrumb is DEBUG (filtered out at the INFO production default) and is safe against a discard logger sink.
- [ ] The skip-branch WARN is unchanged; the escalation poll/return behaviour is unchanged.

**Tests**:
- `"it emits a DEBUG kill-barrier-escalating breadcrumb with target_pid on the SIGKILL escalation branch"` (via `log.SetTestHandler` capture + a seam driving `IdentifyDaemon`→`IdentifyIsPortalDaemon` and a survive-then-die `IsAlive`)
- `"it does NOT emit the escalation breadcrumb on the skip branch (IdentifyNotPortalDaemon / transient error / IdentifyDead)"`
- `"the escalation breadcrumb is the immediately-preceding statement to SendSIGKILL"` (assert ordering: no SIGKILL observed before the breadcrumb in the capture)
- `"the breadcrumb fires under component=saver"`

**Edge Cases**:
- Fires only on the `IdentifyIsPortalDaemon` escalation branch — not the skip-WARN branch.
- `target_pid` attr present and equal to the SIGKILL'd PID.
- Fires before the SIGKILL syscall as the only intervening statement (no other work between identity-check and signal) — the adjacency invariant holds.
- Discard/no-op logger sink is safe (post-Phase-1 the logger is a real `*slog.Logger`, never nil).

**Context**:
> Enumerated gap-closure site (spec § Diagnostic context preservation → Enumerated gap-closure sites): "`escalateKillToSIGKILL` (`internal/tmux/portal_saver.go`) | no breadcrumb on the SIGKILL escalation path | DEBUG breadcrumb at the escalation decision, beneath the `saver: kill-barrier escalated` INFO lifecycle event."
>
> Adjacency invariant (`internal/tmux/portal_saver.go` `escalateKillToSIGKILL` godoc): "The identity-check → SIGKILL pairing is the spec's load-bearing residual-recycle-window invariant: no work (other than the syscall itself) runs between the check and the signal. The two seam calls are deliberately adjacent in source."
>
> Level discipline: "Probe failure inside a hysteresis window … `Debug` per failure"; decision-point detail beneath an INFO lifecycle event is DEBUG. `target_pid` is in the closed Lifecycle attr group (spec § Subsystem prefix taxonomy → Lifecycle). `saver: kill-barrier escalated` is component `saver` (spec § Saver and daemon lifecycle event taxonomy → Saver lifecycle events).

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Diagnostic context preservation at boundaries (Enumerated gap-closure sites — `escalateKillToSIGKILL`); § Saver and daemon lifecycle event taxonomy (kill-barrier escalated component)

---

## portal-observability-layer-4-5 | approved

### Task 4-5: Close the `ShowGlobalHooks` failure-log asymmetry with the missing WARN branch

**Problem**: `internal/tmux/hooks_register.go` has an asymmetry in how it logs `ShowGlobalHooks` failures: `migrateSessionClosedHook` (lines 309-313) emits a WARN before returning the wrapped `show-hooks failed: %w` error, but its two siblings that make the identical `ShowGlobalHooks` call — `RegisterHookIfAbsent` (lines 116-119) and `migrateHydrationHooks` (lines 237-240) — return the same wrapped error **silently**, with no WARN. This is the named gap-closure defect: "failure-log asymmetry — one branch logs, the sibling failure path does not." The result is that some `show-hooks` failures during bootstrap step 2 surface a WARN and some do not, depending on which sibling hit the failure.

**Solution**: Add the missing WARN to the silent `ShowGlobalHooks`-failure branch(es) so all `show-hooks`-failure paths log uniformly, per the level-discipline table. The WARN passes the wrapped `err` directly to the `"error"` attr and carries `error_class="unexpected"` (the swallowed/returned error drops a unit of work — the hook registration/migration for that event did not happen).

**Outcome**: Every `ShowGlobalHooks`-failure path in `hooks_register.go` emits exactly one WARN with the wrapped error before returning/aborting; the return/abort behaviour is unchanged; no path double-logs the same failure.

**Do**:
- **Identify the silent branches.** `migrateSessionClosedHook` already WARNs (`log.Warn(state.ComponentBootstrap, "session-closed migration skipped: show-hooks failed: %v", err)`, line 311 — post-Phase-1 this is a slog WARN). The silent siblings are:
  - `RegisterHookIfAbsent` (`internal/tmux/hooks_register.go:116-119`): `raw, err := c.ShowGlobalHooks(); if err != nil { return fmt.Errorf("show-hooks failed: %w", err) }` — no WARN.
  - `migrateHydrationHooks` (`internal/tmux/hooks_register.go:237-240`): `raw, err := c.ShowGlobalHooks(); if err != nil { return 0, fmt.Errorf("show-hooks failed: %w", err) }` — no WARN.
- **Add the missing WARN.** In each silent branch, before the `return`, emit one WARN under the `bootstrap` component (these lines render under `bootstrap` today via `state.ComponentBootstrap`; after Phase 1 the package binds the equivalent `log.For` logger). Use a terse message + attrs:
  ```go
  logger.Warn("show-hooks failed", "error", err, "error_class", "unexpected")
  ```
  Pass the wrapped `err` directly to the `"error"` attr (NEVER `err.Error()` — Phase 1 convention) so the handler renders the full chain including the tmux stderr (preserved by Task 4-2). `error_class="unexpected"` per the level table (the failure drops the hook-registration unit of work for this event).
  - **`RegisterHookIfAbsent` logger access:** this helper currently takes no logger (its signature is `(c *Client, event, expectedSubstring, fullCommand string)`). Bind a package-level `var logger = log.For("bootstrap")` in `hooks_register.go` (post-Phase-1 these files already log via a bound `log.For` logger for the migration helpers) and use it directly in `RegisterHookIfAbsent` — no signature change needed, since the slog logger is package-bound, not threaded as a parameter. (This also lets the existing `migrateHydrationHooks`/`migrateSessionClosedHook` `log` parameter and the package-level logger coexist; prefer the package-level `log.For("bootstrap")` for the new WARN so `RegisterHookIfAbsent` needs no new parameter.)
  - **`migrateHydrationHooks` logger access:** it already receives a logger (`log MigrationLogger` → post-Phase-1 `*slog.Logger`). Use that injected logger for the WARN so the test seam continues to capture it.
- **Match the existing WARN's component and shape.** `migrateSessionClosedHook`'s existing WARN renders under `bootstrap` — keep all three uniform under `bootstrap`. The message phrase should be consistent ("show-hooks failed"); the existing `migrateSessionClosedHook` line may be normalised to the same terse-message+attrs shape during Phase 1's big-bang conversion — if it already reads `logger.Warn("show-hooks failed", "error", err, "error_class", "unexpected")` post-Phase-1, the three sites are then identical and this task only adds the two missing ones.
- **No double-log.** `RegisterPortalHooks` (lines 376-405) folds each sibling's returned error into an `errors.Join` aggregate. The WARN fires inside the sibling (once per failure), and `RegisterPortalHooks` does NOT separately log the aggregate — confirm it does not, so each failure is logged exactly once at the point of failure, not also at the aggregate site. If `RegisterPortalHooks` (or bootstrap step 2) currently logs the aggregate, ensure the new per-sibling WARN does not duplicate it (prefer the per-sibling WARN as the single source; the aggregate is returned for control flow, not re-logged).
- **Return/abort behaviour unchanged.** The WARN is added BEFORE the existing `return` — the returned error, the abort-before-append semantics, and the `errors.Join` folding are all unchanged. Only the log emission is added.

**Acceptance Criteria**:
- [ ] `RegisterHookIfAbsent` emits one WARN ("show-hooks failed") with the wrapped `err` in the `"error"` attr and `error_class="unexpected"` before returning the `show-hooks failed: %w` error.
- [ ] `migrateHydrationHooks` emits the same WARN before returning `(0, show-hooks failed: %w)`.
- [ ] `migrateSessionClosedHook`'s pre-existing WARN remains (no regression) and is consistent in component/shape with the two added WARNs.
- [ ] The `"error"` attr passes the wrapped error directly (not `.Error()`), so the rendered line includes the tmux stderr from the underlying `*CommandError`.
- [ ] Each `show-hooks` failure is logged exactly once (no double-log via the `RegisterPortalHooks` `errors.Join` aggregate).
- [ ] All three WARNs render under the `bootstrap` component.
- [ ] The return values, abort-before-append semantics, and `errors.Join` folding are byte-identical to before.

**Tests**:
- `"RegisterHookIfAbsent emits one WARN with the wrapped error and error_class=unexpected on a show-hooks failure"` (via a `Commander` failing `show-hooks` + `log.SetTestHandler` capture)
- `"migrateHydrationHooks emits the same WARN on a show-hooks failure and still returns (0, err)"`
- `"migrateSessionClosedHook's existing WARN is unchanged"`
- `"the error attr carries the wrapped *CommandError chain including stderr, not the .Error() string"`
- `"a single show-hooks failure is logged exactly once (no aggregate double-log)"`
- `"return/abort behaviour is unchanged on a show-hooks failure"`

**Edge Cases**:
- Identify the silent consuming branches (`RegisterHookIfAbsent` / `migrateHydrationHooks`) vs `migrateSessionClosedHook` which already WARNs.
- `"error"` attr passes the wrapped `err` directly (full chain incl. stderr from Task 4-2's `*CommandError`).
- WARN fires once per failure (no double-log into the `errors.Join` aggregate).
- `error_class="unexpected"` (the failure drops the hook-registration unit of work for that event).
- Return / abort behaviour unchanged — only the log emission is added.
- `RegisterHookIfAbsent` needs no signature change — use the package-level `log.For("bootstrap")` logger.

**Context**:
> Enumerated gap-closure site (spec § Diagnostic context preservation → Enumerated gap-closure sites): "`ShowGlobalHooks` | failure-log asymmetry — one branch logs, the sibling failure path does not | add the missing WARN on the unlogged failure branch per the level-discipline table."
>
> Level discipline (spec § Log-level discipline → Mechanical level-selection table): "`log-and-continue` on an unexpected error where swallowing it drops a unit of work or leaves the function's postcondition unmet … → `Warn` with `error_class="unexpected"`."
>
> slog attr usage (spec § Diagnostic context preservation): "The `"error"` attr value MUST be the wrapped error directly (`err`, not `err.Error()`); slog handles serialization. The custom handler renders the full chain of wrapped messages including the stderr text."
>
> Existing asymmetry in code: `migrateSessionClosedHook` (`internal/tmux/hooks_register.go`) emits `log.Warn(... "session-closed migration skipped: show-hooks failed: %v", err)` before returning; `RegisterHookIfAbsent` and `migrateHydrationHooks` return `fmt.Errorf("show-hooks failed: %w", err)` silently.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Diagnostic context preservation at boundaries (Enumerated gap-closure sites — `ShowGlobalHooks`); § Log-level discipline (Mechanical level-selection table)

---

## portal-observability-layer-4-6 | approved

### Task 4-6: Comment the uncommented defensive branches in the phase's boundary code

**Problem**: The fourth enumerated gap-closure defect is open-ended: "Defensive branches (various) | branch exists for a non-obvious reason, uncommented | add a 'why this branch exists' code comment (not a log line)." Several boundary-code branches swallow or ignore an error for a deliberate, non-obvious reason (e.g. the hydrate helper's `_, _ = f.Read(buf)` treating a 0-byte read as valid signal arrival, `CreateFIFO`'s intentionally-ignored defensive `os.Chmod`, the pgrep exit-0-with-empty-output defensive guard). A reader cannot tell a deliberate defensive swallow from a forgotten error check without a comment, which is exactly the "lost context at the boundary" failure mode this subtopic guards against. This defect is closed with **code comments only** — no log line, no behaviour change.

**Solution**: Add a one-line "why this branch exists" code comment to each uncommented defensive branch in the boundary code touched by this phase (the `os`/`io`/`exec.Cmd`/tmux-commander sites of tasks 4-1 through 4-5), explaining the deliberate reason the error is swallowed/ignored/defensively guarded. Skip branches that already carry such a comment. No log lines, no control-flow changes.

**Outcome**: Every deliberate defensive swallow/ignore in the phase's boundary code carries a comment stating why; a future reader can distinguish a deliberate defensive branch from a forgotten error check; behaviour is unchanged.

**Do**:
- **Scope: "various", limited to boundary code touched this phase.** The spec leaves the defect open-ended ("various"); bound the scope to the `os`/`io`/`exec.Cmd`/tmux-commander boundary sites this phase audits or modifies (tasks 4-1–4-5). Do NOT sweep the entire codebase — that is out of scope and would balloon the task.
- **Audit each boundary branch for an uncommented deliberate swallow/ignore.** Candidate sites observed (verify against the live files; some already carry comments and are skipped):
  - `cmd/state_hydrate.go:126` — `_, _ = f.Read(buf)` (1-byte FIFO signal read): already has an inline comment ("Errors are ignored: even a 0-byte read can mean 'writer closed' which is still arrival") — **already commented, skip** unless the comment is thin; if thin, tighten it.
  - `internal/state/fifo.go:39` — `_ = os.Chmod(path, 0o600) // defensive against umask`: already has a brief comment AND a full godoc paragraph — **already commented, skip.**
  - `internal/state/pgrep.go:61-66` — the exit-0-with-empty-output branch: already commented ("defensive guard … handled here for robustness") — **already commented, skip.**
  - `internal/tmux/tmux.go` — `ListSessions` returns `([]Session{}, nil)` on a `list-sessions` error (line 186-188): this swallows a tmux error to an empty list (a deliberate "no server running → no sessions" defensive branch). If uncommented, add: why returning empty-not-error is correct here (no tmux server is a valid zero-sessions state, not a failure to surface). Verify the live state post-Phase-1.
  - `internal/tmux/portal_saver.go` — the tolerant `_ = c.KillSession(...)` swallows in `killSaverAndWaitForDaemon` (lines 369, 375, 380): the godoc explains the tolerance ("the session may have auto-destroyed between probe and kill — that is equivalent to 'already absent'"), but the inline `_ =` swallows may lack an adjacent comment. If the rationale is only in the function godoc and not obvious at the swallow site, add a one-line inline pointer.
  - `cmd/state_hydrate.go` — `_ = os.Remove(cfg.FIFO)` (lines 128, 268) and `_ = f.Close()` (line 127): the timeout handler's `os.Remove` already has a comment (lines 266-268); the step-2 `os.Remove`/`Close` at lines 127-128 may lack one — add a one-liner if uncommented ("FIFO consumed; best-effort unlink, next bootstrap sweeps stale FIFOs").
- **Comment shape.** One terse line stating the deliberate reason, e.g. `// ENOENT here is the fresh-path common case — swallow.` or `// Tolerant: session may have auto-destroyed between probe and kill (== already absent).` The comment explains *why the error is deliberately not surfaced*, distinguishing it from a forgotten check.
- **Skip already-commented branches.** Where a branch already carries an adequate "why" comment (most of `fifo.go`, `pgrep.go`, the hydrate timeout handler, the portal_saver godocs), do nothing — adding redundant comments is noise.
- **No log lines, no behaviour change.** This task adds comments ONLY. It must not add any `logger.*` call, must not change any error handling, control flow, or return value. A `git diff` of this task contains only comment-line additions (and possibly comment tightening), no executable-line changes.
- **Coordinate with Task 4-3.** Task 4-3's audit identifies the defensive `os`/`io` ignore branches (the hydrate `f.Read`, `CreateFIFO`'s `os.Chmod`); this task adds/verifies their comments. If 4-3 confirms a branch is correct-and-commented, 4-6 skips it; if 4-3 finds it correct-but-uncommented, 4-6 comments it.

**Acceptance Criteria**:
- [ ] Every uncommented deliberate defensive swallow/ignore in the phase's boundary code (tasks 4-1–4-5 sites) carries a one-line "why this branch exists" comment.
- [ ] Branches that already carry an adequate "why" comment are left unchanged (no redundant comments added).
- [ ] No log line is added; no control-flow, error-handling, or return-value change is made (the diff is comment-only).
- [ ] The scope is limited to boundary code touched this phase — no codebase-wide comment sweep.
- [ ] `go build ./...` and `go test ./...` remain green (a comment-only change cannot break either, which is itself the regression guard).

**Tests**:
- This task is comment-only and has no runtime behaviour to test. The verification is: (a) `go build ./...` and `go test ./...` stay green (comment-only diffs cannot change either), and (b) a reviewer confirms each enumerated boundary swallow/ignore carries a rationale comment. No new test cases are added; if any existing test would change output, the change is NOT comment-only and must be reverted.

**Edge Cases**:
- Comment-only (no log line, no behaviour change) — the diff contains only comment additions/tightening.
- Already-commented branches are skipped (no redundant comments).
- Scope limited to boundary code touched this phase ("various" per spec) — not a codebase-wide sweep.
- Coordinates with Task 4-3's audit: 4-3 identifies the defensive `os`/`io` branches; 4-6 ensures each carries a rationale comment.

**Context**:
> Enumerated gap-closure site (spec § Diagnostic context preservation → Enumerated gap-closure sites): "Defensive branches (various) | branch exists for a non-obvious reason, uncommented | add a 'why this branch exists' **code comment** (not a log line)."
>
> Subtopic intent (spec § Diagnostic context preservation → Decision): "Discarding stderr is the most common form of 'we lost the debug context exactly where we needed it most'." An uncommented deliberate swallow is the same loss-of-context failure mode at the source level: a reader cannot distinguish a deliberate defensive swallow from a forgotten check.
>
> Note the enumerated sites are "named explicitly because a purely-mechanical level/boundary pass can skip a site where nothing about the code shape forces a new log call" — the comment closure is exactly such a non-mechanical, deliberate fix.

**Spec Reference**: `.workflows/portal-observability-layer/specification/portal-observability-layer/specification.md` § Diagnostic context preservation at boundaries (Enumerated gap-closure sites — Defensive branches (various))
