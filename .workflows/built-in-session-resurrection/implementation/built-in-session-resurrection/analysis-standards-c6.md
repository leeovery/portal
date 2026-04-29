---
agent: standards
cycle: 6
findings_count: 7
status: issues_found
---
# Standards Analysis (Cycle 6)

## Summary

Round-trip tests bypass the spec'd `client-attached`/`client-session-changed` hook pathway by direct FIFO writes, and two Phase 5 task 5-10 acceptance bullets are missing — convention/logger discipline is otherwise clean.

---

## Spec Conformance Gaps

### FINDING S1: Reboot round-trip test bypasses spec'd `client-attached` / `client-session-changed` hook pathway and the `signal-hydrate` binary

- **SEVERITY:** medium
- **FILES:** `cmd/bootstrap/reboot_roundtrip_test.go:308-540`, `internal/restore/integration_full_test.go:239-580`
- **DESCRIPTION:** Phase 5 task 5-9 acceptance bullets explicitly require: "`client-attached` is exercised by spawning `tmux attach-session` on a fresh PTY" and "`client-session-changed` is exercised by `tmux switch-client`." Both round-trip tests instead bypass the entire signal pathway by writing the FIFO byte directly via test-local `openAndSignalFIFO`. The planning's documented fragile-PTY fallback ("invoke `portal state signal-hydrate <session>` directly") is also not used — the test duplicates the writeFIFOSignal logic. Net result: the production tmux hook → run-shell → `portal state signal-hydrate` argv → `runSignalHydrate` body → FIFO write pipeline is never exercised end-to-end. A regression in argv parsing, hook registration content, hook-driven binary resolution, or production retry budget would not be caught. The `client-session-changed` switch-client variant is missing entirely.
- **RECOMMENDATION:** Either spawn the already-built portal binary with `state signal-hydrate <session>` (binary is on PATH in these tests) to exercise the production CLI surface, or add a `switch-client` subtest that toggles between two restored sessions to exercise `client-session-changed`. The current test comment ("test-side replacement for `portal state signal-hydrate`") implicitly acknowledges the gap.

### FINDING S2: Phase 5 task 5-10 acceptance test "saved_at not advanced during a steady-state reattach window" is missing

- **SEVERITY:** low
- **FILES:** `cmd/reattach_integration_test.go:314-375`
- **DESCRIPTION:** Planning task 5-10 enumerates seven acceptance test bullets including "saved_at is not advanced during a steady-state reattach window" (phase-5-tasks.md:947) — the planning text explicitly notes this is "duplicated intentionally because this test case exercises a different trigger (normal command, not a test-only probe)." `TestReattachIntegration_SteadyStateReattachZeroStructuralRewrites` asserts pane-id preservation via skeleton-marker absence but never compares `sessions.json.saved_at` pre/post Run.
- **RECOMMENDATION:** Capture sessions.json saved_at before bootstrap and assert it is unchanged after Run, mirroring `phase5_marker_suppression_integration_test.go:207-210`.

### FINDING S3: Task 5-10 test "portal open PATH resolves a session name present only in sessions.json" is missing

- **SEVERITY:** low
- **FILES:** `cmd/reattach_integration_test.go:636-679`
- **DESCRIPTION:** Planning task 5-10's test list (phase-5-tasks.md:942) names "portal open PATH resolves a session name present only in sessions.json." `TestReattachIntegration_OpenLaunchesTUIAfterRestoredSkeleton` exercises bare `portal open` (no positional arg, TUI launch) but never invokes `portal open <path>` against a saved-only session. The path-arg branch — alias/zoxide/direct-path resolution → cmd/open.go attach hand-off — is uncovered.
- **RECOMMENDATION:** Pre-seed an alias entry resolving to a saved-only session name and invoke `portal open <path>`, asserting connector dispatch reaches the saved name.

### FINDING S4: Phase5 marker-suppression non-vacuity guard is partial — daemon save pipeline is not exercised

- **SEVERITY:** low
- **FILES:** `cmd/bootstrap/phase5_marker_suppression_integration_test.go:155-167`
- **DESCRIPTION:** The test wires `bootstrap.NoOpSaver{}` and never starts a real `portal state daemon`, so the spec's actual "Restoration guard" contract (daemon ticks observe `@portal-restoring=1` at entry and skip the capture, spec § Save-Side Architecture → Single-Writer Serialization → Properties → Restoration guard) is not under test. The probe proves session-created fires; the saved_at-unchanged assertion only proves Restore itself does not write sessions.json mid-run. The original assertion-of-absence test is acknowledged as vacuous; the replacement narrows but does not exercise the daemon's tick-time marker check.
- **RECOMMENDATION:** Document the scope limitation in the test header (currently presented as the suppression contract, but only covers Restore-side write discipline). A true daemon-tick suppression test would spawn `portal state daemon` against the same socket or invoke the daemon tick body inline with the marker set — out of Phase 12 scope if intentional, but the test's claim should match what it proves.

## Convention Violations

### FINDING S5: Reboot round-trip hook registration uses `tmux.PaneTarget` where the production helper's `--hook-key` flag carries a `state.SanitizePaneKey`-formatted string

- **SEVERITY:** low
- **FILES:** `cmd/bootstrap/reboot_roundtrip_test.go:155-169`
- **DESCRIPTION:** The test registers the on-resume hook via `tmux.PaneTarget("alpha", cfg.saveBase+0, cfg.savePaneBase+0)`. Per spec § "Helper hook lookup under index drift," helpers consult hooks.json by the saved structural identifier `<raw-session>:<saved-window>.<saved-pane>` — which `SessionRestorer.collectArmInfos` passes to `--hook-key` from sessions.json. `PaneTarget` is a tmux-target formatter, not a saved-key formatter; the test relies on the two formats coincidentally producing the same string for these inputs. A future variant where saveBase!=0 (or where PaneTarget's separators diverge from SanitizePaneKey's) would silently drift between test fixture and production hook key, causing the hook to fail lookup despite the test "passing setup."
- **RECOMMENDATION:** Replace `tmux.PaneTarget` with `state.SanitizePaneKey("alpha", cfg.saveBase+0, cfg.savePaneBase+0)` so test fixture and production hook-key formatter share the same canonical source.

### FINDING S6: `state_signal_hydrate.go` retry comment cites a non-existent spec section title

- **SEVERITY:** low
- **FILES:** `cmd/state_signal_hydrate.go:18-21`
- **DESCRIPTION:** Comment cites "Spec → 'FIFO open-for-write semantics'" — no spec section by that exact title exists. Closest is "Signal Mechanism: FIFO Per Pane" (spec L763) and "Helper Behavior on Startup."
- **RECOMMENDATION:** Update the comment to cite the actual spec section title for navigability.

## Documentation Drift

### FINDING S7: Spec § Fatal Bootstrap Errors does not explicitly enumerate Clear @portal-restoring as fatal — only Set

- **SEVERITY:** low
- **FILES:** `.workflows/built-in-session-resurrection/specification/built-in-session-resurrection/specification.md:1394`, `cmd/bootstrap/bootstrap.go:227-229`, `.workflows/built-in-session-resurrection/planning/built-in-session-resurrection/phase-5-tasks.md:108`
- **DESCRIPTION:** Phase 12 prompt notes "step 6 `Restoring.Clear` failure is now documented as fatal (matches spec § Fatal Bootstrap Errors and CLAUDE.md)." Spec § Fatal Bootstrap Errors lists `@portal-restoring set-option fails` as fatal but does not explicitly enumerate Clear (the unset). Implementation (bootstrap.go:227-229), CLAUDE.md, and reconciled planning all classify Clear as fatal — consistent — but the spec text relies on readers extending "set-option" symmetrically to "unset." Future spec edits could miss the symmetric case.
- **RECOMMENDATION:** Optionally add an explicit "Clear @portal-restoring fails (unset)" bullet to spec § Fatal Bootstrap Errors so spec/planning/implementation alignment is enforced at the authoritative source. Out of Phase 12 scope.
