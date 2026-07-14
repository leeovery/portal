TASK: restore-host-terminal-windows-2-7 — N≥2 unsupported/NULL atomic no-op — exit 1, nothing spawned

ACCEPTANCE CRITERIA:
- `portal spawn s1 s2` with resolution == unsupported records ZERO FakeAdapter.OpenWindow calls (check precedes any adapter call — atomic).
- Same invocation calls Connect ZERO times (no self-attach) and returns a plain error → exit 1, one-line stderr message naming the detected terminal (friendly name + bundle id for a resolved-but-undriven identity, or the "no host-local terminal" line for NULL).
- `portal spawn s1` (N=1) on the same unsupported/NULL terminal opens nothing but DOES self-attach s1 via the connector.
- A recognised-but-undriven identity (com.apple.Terminal) and a NULL identity both trigger the N≥2 atomic no-op; message names the identity for the former, honest no-host-local line for the latter.
- Returned error is not a *cmd.UsageError (exit 1, not 2) and is not silenced (prints to stderr).

STATUS: Complete

SPEC CONTEXT: Spec (Terminal Identity & Detection → Unsupported-terminal behaviour) mandates that `Enter` with N=1 proceeds regardless of detection (plain self-attach, no adapter), while `Enter` with N≥2 on an unsupported/NULL terminal is an atomic no-op — nothing opens because the N−1 external windows need an unavailable adapter. Reporting & exit codes: unsupported/NULL with N≥2 → exit 1 with the same one-line message the picker would show, on stderr, nothing self-execs. The asymmetry (N=1 works, N≥2 blocked) is intentional; only external-window spawning needs the adapter. Design copy: `⚠ unsupported terminal — Apple Terminal · com.apple.Terminal`; NULL folds to the honest "no host-local terminal" line.

IMPLEMENTATION:
- Status: Implemented
- Location: cmd/spawn.go:156-159 (the gate in runSpawn), cmd/spawn.go:220-227 (unsupportedSpawnMessage), cmd/spawn.go:235-239 (logSpawnUnsupported); shared renderers internal/spawn/message.go:58-72 (UnsupportedNoopMessage) and internal/spawn/logemit.go:86-97 (LogUnsupported); main.go:83-110 (classify → plain error prints to stderr, exit 1).
- Notes: Gate placement is exactly as planned — after `adapter, resolution := deps.Resolve(id)` (line 151) and BEFORE the first adapter touch `deps.NewBurster(adapter).Run(...)` (line 161), so the no-op is atomic. Resolve only classifies identity → adapter; it opens no windows. The gate returns `errors.New(unsupportedSpawnMessage(id))` — a plain, non-UsageError, non-silent error — so main.classify prints to stderr and exits 1; the connector is never called (no self-attach) and no adapter method runs.
- Deliberate, superior deviation from the plan's literal wording (not drift): the plan said compose the message inline via fmt.Sprintf and gate on `len(external) >= 1 && resolution == …`. The implementation (a) delegates the message to the shared spawn.UnsupportedNoopMessage renderer (byte-identical to what the Phase-5 picker will render — DRY across the CLI/picker boundary, documented in message.go's doc comment), and (b) omits the `len(external) >= 1` conjunct because the N=1 (empty-external) case already returned earlier at cmd/spawn.go:145, making the gate structurally unreachable for N=1. Both changes are semantically equivalent and cleaner; the produced messages match the plan exactly ("spawn: no host-local terminal — nothing opened" for NULL; "spawn: unsupported terminal — <Name> · <BundleID> — nothing opened" for a resolved-but-undriven identity). Verified the em-dash (U+2014) and middle dot (U+00B7) match byte-for-byte between message.go and the test assertions.

TESTS:
- Status: Adequate
- Coverage: cmd/spawn_test.go covers every acceptance criterion. "it refuses an N>=2 batch on an unsupported terminal atomically with no adapter call" (587) asserts len(adapter.Calls)==0. "it does not self-attach on an N>=2 unsupported batch and exits 1" (610) asserts zero Connect calls, not-UsageError, not-silent. "it names the detected terminal (friendly name + bundle id) in the one-line message" (637) asserts the exact message string AND exactly one INFO outcome line carrying only resolution=unsupported/terminal/bundle_id with no per-window/summary attrs. "it prints the honest no-host-local-terminal line for a NULL identity N>=2 batch" (683) asserts the NULL message plus zero adapter/connect calls. "it still self-attaches for N=1 on an unsupported terminal" (712) asserts zero adapter calls and Connect==[s1]. A second N=1-on-unsupported case using a NULL identity is covered under TestSpawnPipeline (353), so both identity kinds (NULL and resolved-but-undriven) are exercised for the N=1 self-attach path.
- Notes: Tests are behaviour-focused and would genuinely fail if the gate were removed — with the gate gone, buildSpawnDeps defaults NewBurster to a real burster over the injected FakeAdapter, so OpenWindow would be recorded and adapter.Calls != 0 would trip. The outcome-line assertions guard against over-emission (exactly one INFO, no opened/total/ack/batch attrs). No redundancy or excessive mocking — each test isolates a distinct dimension. Not over-tested.

CODE QUALITY:
- Project conventions: Followed. Component logger bound once (spawnLogger = log.For("spawn")); the closed message string + attr set live solely in internal/spawn (message.go / logemit.go) with the CLI adding only the "spawn: " prefix — matching the codebase's single-renderer/no-drift discipline. DI seams (Detector/Resolve/Connector/Logger) are all injectable; the unit test drives the full pipeline with no real tmux/osascript.
- SOLID principles: Good. Message rendering and log emission are delegated to single-responsibility helpers shared with the picker (OCP for the Phase-5 caller).
- Complexity: Low. The gate is a single guarded early return.
- Modern idioms: Yes.
- Readability: Good. The gate carries a clear comment explaining the atomic-no-op rationale and why it precedes any adapter call; the pre-flight-runs-first ordering is documented at cmd/spawn.go:129-135.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [quickfix] cmd/spawn.go:156 — Consider restoring the plan's explicit `len(external) >= 1 &&` conjunct to the gate condition (`if len(external) >= 1 && resolution == spawn.ResolutionUnsupported {`). It is currently redundant (the N=1 early return at line 145 guarantees len(external) >= 1 here, so behaviour is unchanged), but adding it self-documents the gate's N≥2 precondition and defends against a future refactor that reorders or removes the N=1 short-circuit silently turning this into an N=1-blocking path. Low priority; current code is correct and covered by the N=1 self-attach tests.
