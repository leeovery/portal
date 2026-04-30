# Review Report: built-in-session-resurrection-12-3

**TASK**: Expand task 5-8 — marker-suppression integration test with non-vacuous probe

**ACCEPTANCE CRITERIA**:
- Add a probe `set-hook -ga` registered before the orchestrator runs that records structural events to a tempfile during the marker window.
- Assert at least one probe event fires (non-vacuity guard).
- Assert `sessions.json.saved_at` is not advanced during the marker window.
- File gated with `//go:build integration` and `testing.Short()` skip.
- Cleanup of global hook in `t.Cleanup`.
- Likely lives in `cmd/bootstrap/phase5_marker_suppression_integration_test.go`.

**STATUS**: Complete

**SPEC CONTEXT**:
The spec's *Bootstrap Flow → Ordering Rationale* and *Save-Side Architecture → Triggers & Serialization → Properties → Restoration guard* require that during the `@portal-restoring` window (steps 3–6), structural hook events fired by the restore itself (`session-created`, `window-linked`, etc.) must NOT cause a daemon-tick capture that would overwrite the pre-reboot `sessions.json`. The original Phase 5 task 5-8 test was descoped to merely assert the marker was set during steps 4–5 — vacuously true with stub Saver/Restore because no hooks ever fired. Phase 12 task 12-3 closes this gap by wiring a real `RestoreAdapter` (so structural events DO fire) plus a probe hook (so we can prove they fired) plus the `saved_at` invariant.

**IMPLEMENTATION**:
- Status: Implemented
- Location: `/Users/leeovery/Code/portal/cmd/bootstrap/phase5_marker_suppression_integration_test.go:1-233`
- Notes:
  - Build tag `//go:build integration` on line 1; `testing.Short()` skip at line 72-74.
  - Probe installation at line 129-132 uses `set-hook -ga session-created` with a `run-shell` body appending epoch-ns to the tempfile — semantically correct.
  - `t.Cleanup` at line 133-140 runs `set-hook -gu session-created` to remove the probe; comment correctly notes that `kill-server` in `tmuxtest.Socket` cleanup is a belt-and-braces backstop.
  - Real `bootstrapadapter.RestoringMarker` wraps the marker lifecycle; real `bootstrapadapter.RestoreAdapter` wraps a real `restore.Orchestrator` so skeleton restore actually fires `session-created` for `probe-target`. `Saver` remains `NoOpSaver` — appropriate because this test scopes to Restore-side write discipline.
  - Probe tempfile placed in a separate `t.TempDir()` (line 87) outside `stateDir` — correct, prevents accidental confusion.
  - Pre-run `saved_at` is fixed at a deterministic UTC instant (line 94) so the post-run equality assertion compares against a known value.
  - Sanity check at lines 173-176 asserts `probe-target` is actually live post-Run — distinguishes "Restore broken" from "suppression broken" if the test ever fails.
  - Final assertion that `@portal-restoring` is cleared by step 6 (lines 214-218) preserves the lifecycle coverage the original test had.

**TESTS**:
- Status: Adequate
- Coverage:
  - Non-vacuity guard (a) — explicit fail on zero probe events at lines 191-193.
  - Suppression invariant (b) — `saved_at` equality at lines 199-209, with explicit handling of the `skip=true` case from `state.ReadIndex`.
  - Marker lifecycle — `@portal-restoring` unset post-Run at lines 214-218.
  - Restore actually fired — `list-sessions` sanity at lines 173-176.
- Notes:
  - The deliberately narrow scope (Restore-side only, `NoOpSaver`) is documented inline (lines 63-70). This is appropriate.
  - `countNonEmptyLines` (lines 225-233) is the right idiom for the probe-fire count.
  - Uses `os.IsNotExist` check (line 185) for the missing-probe-file path rather than fatalling.
  - No redundant assertions; each serves a distinct contract.

**CODE QUALITY**:
- Project conventions: Followed (no `t.Parallel()`; `t.Setenv`/`t.Cleanup`; `tmuxtest.New`/`tmuxtest.SkipIfNoTmux`).
- SOLID: Good. Composes adapters at seam interfaces.
- Complexity: Low.
- Modern idioms: Yes. `t.Setenv`, `os.IsNotExist`, `time.Date` for deterministic fixtures.
- Readability: Excellent. File-level godoc explains why this test exists, what it replaces, why it lives in a separate file.
- Issues: None of substance.

**BLOCKING ISSUES**:
- None

**NON-BLOCKING NOTES**:
- [idea] Probe shell command at line 129 builds via string concatenation: `"run-shell \"echo $(date +%s%N) >> " + probeFile + "\""`. If `probeFile` ever contained shell metacharacters, this would be an injection seam. `fmt.Sprintf` with `%q` would document the assumption.
- [idea] Both `TestPhase5_RestoreCreatesMissingSession` and this test construct similar `restore.Orchestrator` + `RestoreAdapter` + `bootstrap.Orchestrator` wiring with ~25 lines of overlapping boilerplate. Phase 13 task 13-1 already plans shared helpers.
- [quickfix] Probe-file read on line 182 returns a fatal on non-`IsNotExist` errors without including the probe-file path in the diagnostic.
- [idea] Sanity check at line 173-176 uses `strings.Contains` on `list-sessions` output. A future fixture rename to `not-probe-target` could false-positive. Line-iteration would tighten it.
