TASK: 1-1 — Implement state.IdentifyDaemon primitive

ACCEPTANCE CRITERIA:
- `state.IdentifyDaemon` callable with signature `func(pid int) (IdentifyResult, error)`
- Returns IdentifyNotPortalDaemon for current `go test` process
- Returns IdentifyDead for known-dead PID
- Stub matches `"portal portal state daemon"` → IdentifyIsPortalDaemon
- Anchored regex rejects `state daemon-foo`
- Recycled PID (`sleep sleep 30`) → IdentifyNotPortalDaemon
- `ps` error with non-empty stdout → transient `(0, err)`
- `pid <= 0` returns IdentifyDead without invoking `ps`

STATUS: Complete

SPEC CONTEXT: Spec § "Shared Primitive — Daemon Identity Check" — three-result contract + transient error. Implementation uses `ps -o comm=,args= -p <pid>` matching `comm == "portal"` AND argv against `^portal state daemon( |$)`. Primitive does NOT encode caller-specific transient policy; consumers A/B/C own that.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/daemon_identity.go` (152 lines)
- Notes:
  - `IdentifyResult` iota constants in spec order (lines 15-32)
  - `IdentifyDaemon` signature matches (line 100); pid<=0 short-circuit (lines 101-103)
  - `identifyPS` test seam (line 62), `defaultIdentifyPS` shells `ps -o comm=,args= -p <pid>` (line 65)
  - `daemonArgvPattern` compiled once (line 47) from exported `PortalDaemonArgvPattern` constant (line 43) — shared source-of-truth consumed by `internal/bootstrapadapter/orphan_sweep.go`, `cmd/bootstrap/orphan_sweep.go`, `internal/tmux/portal_saver.go`, `internal/state/daemon_lock.go`, `internal/state/pgrep.go`
  - Branch semantics: non-zero exit + empty trimmed stdout → IdentifyDead; non-zero exit + non-empty stdout → wrapped err via %w; zero exit + empty/malformed → transient err; zero exit + parseable → comm/argv match decides
  - Docstring enumerates A/B/C transient-error semantics verbatim from spec

TESTS:
- Status: Adequate
- Location: `internal/state/daemon_identity_test.go` (237 lines)
- Coverage: All 11 plan-mandated tests present. Bonus: trailing flags after `daemon`, real-process self-check via `os.Getpid()`, `errors.Is`-preservation via `%w`
- No `t.Parallel()` (CLAUDE.md-compliant); dead-PID test uses reserved high PID instead of fork-wait (equivalent, less flaky)
- Not over-tested; each test isolates one branch

CODE QUALITY:
- Project conventions: Followed (leaf package placement, seam with cleanup-restore)
- SOLID: Good; seam-based open/closed
- Complexity: Low; flat branch tree
- Modern idioms: `errors.Is`-compatible `%w` wrapping, `regexp.MustCompile` at init, `strconv.Itoa`
- Readability: Good; docstrings reference consumers by component letter

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- [idea] "Zero exit with empty stdout" treated as transient (lines 120-123) — defensible but not in spec's enumerated branches; a one-line rationale tying it to the `( |$)` regex contract would close the loop
- [idea] `defaultIdentifyPS` uses `.Output()` discarding stderr; capturing stderr into the wrapped error would aid operator debugging
- [quickfix] Line 117 error reads "ps failed with output %q" — "stdout" would be more precise
