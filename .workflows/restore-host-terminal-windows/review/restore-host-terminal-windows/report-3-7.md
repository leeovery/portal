TASK: restore-host-terminal-windows-3-7 — permission-required burst-stop (tick-83b670)

ACCEPTANCE CRITERIA:
1. mapGhosttyResult maps an output containing -1743 or -1712 to OutcomePermissionRequired with a non-empty opaque Guidance (naming Ghostty + Automation-settings hint) and an opaque Detail; a non-permission non-zero exit still maps to OutcomeSpawnFailed.
2. A permission-required on window k in a burst stops the adapter being called for k+1..N-1 (FakeAdapter.Calls length == k).
3. Permission guidance surfaced once for the batch (returned error == driver's Guidance string, not the generic "failed to open window(s)" one-liner).
4. Windows opened before k left in place (no teardown); trigger self-attach skipped (exit 1, no self-exec).
5. Batch markers Cleaned on the permission path.
6. General code never inspects an AppleEvent number — recognition lives only in the driver; orchestrator switches on Outcome/Guidance alone.

STATUS: Complete

SPEC CONTEXT: Spec §"Permissions & Error Quarantine (TCC)" (lines 395-420) fixes the closed result taxonomy (success/spawn-failed/permission-required) and mandates that all -1712/-1743/TCC/deep-link specifics stay inside the driver, translated to a generic typed result. §420 "Within a burst" specifies: a permission-required result is accounted like a failed window (skip self-attach, leave opened windows in place) AND stops the burst — because spawns are sequential and the macOS Automation grant is per-(source, target), every later window would hit the identical wall; guidance surfaces once for the batch, not the generic one-liner. §Reporting (line 64) maps permission-required → exit 1 with the guidance on stderr. §Testing (line 486) requires the error-mapping to be unit-tested via a fabricated osascript outcome. Config recipes never produce permission-required (line 374), so the burst-stop is native-adapter-only — correctly scoped to the Ghostty driver here.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - internal/spawn/ghostty.go:109-129 — mapGhosttyResult: clean exit → Success; else -1743/-1712 substring → PermissionRequired(out, ghosttyPermissionGuidance()); else SpawnFailed. Guidance string names Ghostty + System Settings → Privacy & Security → Automation + the x-apple.systempreferences Privacy_Automation deep-link, composed opaquely in the driver.
  - internal/spawn/burst.go:177-179 — the sole early-stop: after appending each WindowResult and firing progress, break on result.Outcome == OutcomePermissionRequired; k+1..N-1 are never composed or handed to the adapter. Switches on generic Outcome only.
  - internal/spawn/classify.go:43-50 — FirstPermission switches on Outcome alone (never a driver detail string).
  - cmd/spawn.go:169 (Ack.Clean on every post-burst path, before the branch) and cmd/spawn.go:191-199 — the permission branch checked FIRST inside the failed>0 block: emits per-window DEBUG detail + the permission INFO (opaque detail only), leaves earlier windows in place, skips Connect, returns errors.New(perm.Result.Guidance).
- Notes: Ordering is correct — clean-exit→Success precedes the permission check precedes the spawn-failed catch-all (per Key Do), and a permission signal only ever rides a non-clean exit, so no misclassification. The permission window is not OK() so it is never awaited (ack=AckFailed → lands in PartitionResults' failed set), but FirstPermission is checked before the generic failed branch so it never double-reports. All six criteria are met, verified against the exact spec sections. No drift.

TESTS:
- Status: Adequate
- Coverage:
  - Driver mapping (internal/spawn/ghostty_openwindow_test.go:88-155): TestMapGhosttyResult table-drives -1743 and -1712 → OutcomePermissionRequired, asserts non-empty Guidance containing "Ghostty" and "Automation", opaque Detail carrying the output; regression sub-tests pin non-permission non-zero exit → SpawnFailed (Guidance empty), execution error → SpawnFailed, clean exit → Success. Covers criterion 1 fully.
  - Burst-stop (internal/spawn/burst_test.go:313-349): window 2 of 5 permission → adapter.calls == 2 (3,4,5 never spawned), results len == 2, results[1].Outcome == OutcomePermissionRequired, earlier argv verified in list order, err nil. Covers criterion 2 at the burst seam. Sibling tests (248-311) prove spawn-failed/timeout do NOT early-stop — guarding that permission is the SOLE stop.
  - CLI (cmd/spawn_test.go:1142-1208): TestSpawnPermissionRequired asserts err == guidance verbatim, no "failed to open" leak, adapter.Calls == 2, conn.calls == 0 (self-attach skipped), ack.Cleaned == 1, not a UsageError, not a silent-exit sentinel (→ stderr exit 1), and no "-1743" leak in the message (driver-quarantine). Covers criteria 3/4/5/6.
  - CLI emission set (cmd/spawn_test.go:1217-1261): 2 per-window DEBUGs + exactly 1 permission INFO + 0 generic summary. Golden-body parity (1276-1284) pins logSpawnPermission's rendered attrs.
- Notes: Balanced — each criterion has exactly one focused assertion path; no redundant happy-path duplication and no testing of implementation internals. Burst-level test correctly asserts only burst-level behaviour (Connect/Clean/error are exercised at the cmd level where they live), avoiding over-mocking. The FakeAdapter (internal/spawntest/adapter.go) records only the argvs it is actually handed, so Calls == 2 genuinely proves the burst — not the adapter — stopped. Would fail if the break were removed (Calls would be 5) or if FirstPermission were checked after the generic branch (err would be the generic one-liner). No under- or over-testing found.

CODE QUALITY:
- Project conventions: Followed. Driver-quarantine boundary (spec's central invariant) is honoured exactly — the only site touching -1743/-1712 is mapGhosttyResult; burst.go and cmd/spawn.go switch on Outcome/Guidance. Matches the Adapter/Result taxonomy, the classify.go count-semantics chokepoint pattern, and the shared logemit.go emitter split. DI seams (osascriptRunner, FakeAdapter, injected clock/ack) consistent with the codebase.
- SOLID principles: Good. mapGhosttyResult is pure and single-responsibility; ghosttyPermissionGuidance isolates the opaque copy; FirstPermission is a small pure predicate reused by both CLI and picker paths (no drift).
- Complexity: Low. mapGhosttyResult is a 3-branch linear map; the burst break is a single guarded statement; the cmd permission branch is one guarded return.
- Modern idioms: Yes — errors.New for the plain exit-1 error, table-driven sub-tests, slices.Clone defensive copies.
- Readability: Good. Comments are load-bearing and accurate (the burst.go early-stop rationale, the cmd.go "checked FIRST means never double-reports" note, the driver-quarantine remarks).
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None. (Considered the strings.Contains substring match for the AppleEvent codes, but it is exactly the approach the task's Key Do prescribes and osascript output format makes a false positive implausible; tightening to "(-1743)" would drift from the spec, so no change is proposed.)
