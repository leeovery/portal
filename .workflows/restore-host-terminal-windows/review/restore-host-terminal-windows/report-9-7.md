TASK: Route the spawn-ack write-failure DEBUG through the enumerated `detail` attr (restore-host-terminal-windows-9-7 / tick-04f9fa)

ACCEPTANCE CRITERIA:
- The spawn-ack write-failure DEBUG emits only enumerated `spawn` attr keys (`session`, `batch`, `detail`); no `error` key on this `spawn`-component line.
- The line remains DEBUG, best-effort, and non-fatal (the write failure still falls through to `Connect`).
- The message string is unchanged.

STATUS: Complete

SPEC CONTEXT:
Specification §Observability (lines 448-450) enumerates the closed `spawn` component attr set: `batch`, `terminal`, `bundle_id`, `resolution`, `session`, `ack`, `opened`/`total`, and the opaque OS-specific `detail`. `error` is NOT in the spawn enumeration. The spec designates `detail` as the driver/opaque-payload attr, and CLAUDE.md's closed-taxonomy rule ("never invent at call-site") applies. This DEBUG line's former `error` key was the sole out-of-catalog attr on a spawn-component line; the fix routes the opaque write-failure payload through the spec-designated `detail`, matching detect.go / logemit.go.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/attach.go:64-68
- Notes: The emission now carries `"session", name`, `"batch", ackBatch`, `"detail", err.Error()` — all three keys are members of the enumerated `spawn` set. The `error` key is gone. Message string `"spawn-ack marker write failed"` unchanged. Emission remains `spawnLogger.Debug(...)` (DEBUG level; `spawnLogger = log.For("spawn")` per cmd/spawn.go:19). It stays inside the `if err := ackWriter.Write(...); err != nil` best-effort block that does NOT return — control falls through to `return connector.Connect(name)` (line 72). Behaviour is otherwise unchanged. No other production or test site references this line's old `error` key (grep across all `*.go` confirms only attach.go emits and attach_test.go asserts it).

TESTS:
- Status: Adequate
- Coverage: New dedicated subtest `cmd/attach_test.go:247-296` ("it routes the write-failure DEBUG through the enumerated detail attr") induces a write failure via `mockAckWriter{err: fmt.Errorf("set-option failed")}`, captures records through `logtest.Sink` + `log.SetTestHandler`, locates the DEBUG record by level+message, and asserts (a) `session` == "s1", (b) `batch` == "b1", (c) `detail` == "set-option failed", (d) `rec.HasAttr("error")` is false, and (e) `connector.connectedTo == "s1"` (best-effort fall-through preserved). The existing best-effort/ordering regression subtests (lines 195-245) exercise the write-before-connect ordering and non-fatal exec paths and pass unchanged.
- Notes: Test directly verifies each acceptance criterion — the presence of the three enumerated attrs, the explicit absence of `error`, DEBUG level, and the fall-through to Connect. It would fail if the feature broke (e.g. reverting to `error`, dropping `detail`, or returning on write failure). Not over-tested: no redundant assertions; the negative `error`-absence assertion is the key regression guard and is warranted.

CODE QUALITY:
- Project conventions: Followed. Uses the package-level `spawnLogger` (`log.For("spawn")`) per the golang-observability / closed-taxonomy convention; attr keys now conform to the spec's enumerated spawn set. `err.Error()` for the opaque string payload matches the detect.go / logemit.go `detail` routing pattern.
- SOLID principles: Good — single-attr-key change, no structural impact.
- Complexity: Low — one-line attr swap.
- Modern idioms: Yes.
- Readability: Good — the surrounding comment (lines 59-62) still accurately explains the best-effort, fall-through-to-Connect rationale.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
