TASK: Audit os-syscall and io/FIFO read boundaries for %w path/errno preservation and EOF/timeout=expected classification (portal-observability-layer-4-3)

ACCEPTANCE CRITERIA:
- Every audited os-read error preserves its *os.PathError via %w; errors.Is(fs.ErrNotExist)/fs.ErrPermission traverse the wrap.
- ENOENT-on-open at expected sites (TailScrollback, SeedHashMap dir, readDaemonFile, CreateFIFO remove) returns expected non-error outcome.
- FIFO open timeout returns ErrHydrateTimeout (expected) → timeout handler, not boundary failure.
- EOF terminator / zero-newline TailScrollback is (nil,nil); genuine mid-chunk io.ReadFull short read wraps with path.
- Mid-stream io.Copy/Read error carries underlying error to handler's errors.Is switch.
- No errors.New(...) discarding an *os.PathError in audited scope.

STATUS: Complete

SPEC CONTEXT:
Spec § Diagnostic context preservation (731-806). Boundary class 3 (os syscalls): preserve path+errno via %w, never errors.New that discards *os.PathError. Class 4 (io/bufio/FIFO): EOF/timeout expected; other I/O errors wrap fmt.Errorf("read %s: %w"). Verify-and-close-gaps task.

IMPLEMENTATION:
- Status: Implemented (audit confirms all enumerated sites already compliant; no production fix required)
- Per-site: scrollback_tail.go:55-127 (ENOENT/zero-byte/zero-newline→(nil,nil); other open + Seek/ReadFull wrap "tail scrollback %s: %w"); scrollback.go:45-75 (ReadDir ENOENT→empty map; other WARN; per-file WARN+skip; WriteScrollbackIfChanged wraps); fifo.go:32-41 (Remove ENOENT swallowed; other remove + Mkfifo wrap; Chmod intentionally ignored+documented); fifo_sweep.go:65-102 (Glob wraps; per-file WARN+continue); daemon_state.go:128-137 (readDaemonFile ENOENT→absentSentinel; other "read %s: %w"; ReadPIDFile parse wraps); restore/session.go (no os/io read boundary — filepath.Join + state.CreateFIFO + tmux calls); state_hydrate.go (timeout→ErrHydrateTimeout→HandleTimeout; non-timeout open wraps "open fifo %s: %w"; os.Open err verbatim into hydrateFileMissingContext.Cause; io.Copy err verbatim; handleHydrateFileMissing classifies via errors.Is fs.ErrNotExist/fs.ErrPermission/default).
- Notes: Only errors.New is ErrHydrateTimeout sentinel — not a context-losing wrap. No prohibited discarding wrap.

TESTS:
- Status: Adequate
- Coverage: each named regression test located — scrollback_tail_test.go ((nil,nil) ENOENT/zero-byte/zero-newline; mid-scan wrap + errors.Is(fs.ErrPermission) + path; fd-closed-on-error); daemon_state_internal_test.go (TestReadDaemonFile three arms + success); fifo_test.go (ENOENT tolerance, mkfifo/remove wrap, PreservesPathError); state_hydrate_test.go (timeout→HandleTimeout; ClassifiesCauseFromRawChain; MidStreamCopyError EISDIR; PassesRawCauseVerbatim errors.As *os.PathError; PassesPermissionCauseVerbatim); scrollback_test.go (SeedHashMap ENOENT→empty map + per-file + nil-logger).
- Notes: Real OS-error fixtures (chmod 0o000, dir-as-file, close-before-seek), not string mocks — would fail if %w swapped for %v. Root/Windows guards appropriate. Structural traversal assertions (errors.As/Is), not string form.

CODE QUALITY:
- Project conventions: Followed (%w wrapping; sentinels; no t.Parallel; isolated tmpdirs).
- SOLID: Good — readDaemonFile shared classifier; handleHydrateFileMissing single funnel.
- Complexity: Low.
- Modern idioms: Yes (errors.Is/As, io/fs sentinels, table subtests, test seams).
- Readability: Good — (nil,nil)/sentinel/swallow + verbatim-Cause contracts documented.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] The fs.ErrPermission-traversal regression tests are duplicated near-verbatim across three files (chmod 0o000 / root-skip / Windows-skip setup); a shared assertPreservesPathError(t, fn) helper would reduce drift as more boundary sites are added.
- [idea] MidStreamCopyError test asserts only Cause non-nil (EISDIR differs across platforms); a future hardening could assert errors.As *os.PathError to lock path context for the directory-Read error.
