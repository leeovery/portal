TASK: 8-8 — Replace fmt.Sprintf with strconv.Itoa in identifyPS

STATUS: Complete

SPEC CONTEXT: Cycle-2 architecture finding #4 — `internal/state/daemon_identity.go` used `fmt.Sprintf("%d", pid)` in hot `defaultIdentifyPS` path. Mechanical perf/idiom swap.

IMPLEMENTATION:
- Status: Implemented
- Location: `internal/state/daemon_identity.go:65` — `exec.Command("ps", "-o", "comm=,args=", "-p", strconv.Itoa(pid)).Output()`
- `strconv` imported (line 7)
- `fmt` retained (line 4) — legitimately used by three `fmt.Errorf` calls. Plan's "drop fmt if no other usage" correctly evaluated
- `grep fmt\.Sprintf` returns zero matches in file

TESTS:
- Status: Adequate
- Existing `internal/state/daemon_identity_test.go` exercises `IdentifyDaemon` via `identifyPS` seam — production `defaultIdentifyPS` change observationally invisible (Itoa(n) and Sprintf("%d", n) byte-identical)
- Not over/under-tested

CODE QUALITY:
- Standard Go; one-line swap; `strconv.Itoa` is canonical

BLOCKING ISSUES:
- None

NON-BLOCKING NOTES:
- None
