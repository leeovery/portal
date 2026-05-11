TASK: killed-sessions-resurrect-on-restart-2-1 — Flip TestHydrate_TimeoutDoesNotUnsetSkeletonMarker to assert marker-unset on timeout, then make handleHydrateTimeout call unsetSkeletonMarkerOrLog

ACCEPTANCE CRITERIA:
- handleHydrateTimeout calls unsetSkeletonMarkerOrLog (state.UnsetSkeletonMarkerForFIFO under the hood) before exec; failure is a soft warning, not a block on shell exec.
- Unit test asserts marker-unset primitive is invoked.
- The 100ms settle-sleep is preserved (in runHydrate, not the handler).
- Edge cases: (a) UnsetSkeletonMarkerForFIFO failure logs soft warning and does not block subsequent exec, (b) paneKey derived from FIFO basename via existing seam, (c) set-option -su argv observed exactly once per timeout.

STATUS: Complete

SPEC CONTEXT: Spec § Fix 2 → Specific Changes → 1 (lines 145–146): timeout handler invokes `unsetSkeletonMarkerOrLog` → state.UnsetSkeletonMarkerForFIFO; failure soft-warn, no block. § Specific Changes → 4: 100ms settle-sleep preserved. § Spec Supersession explicitly supersedes original "Helper does NOT unset marker on FIFO timeout" invariant.

IMPLEMENTATION:
- Status: Implemented
- Location:
  - /Users/leeovery/Code/portal/cmd/state_hydrate.go:260–277 — handleHydrateTimeout body ends with `unsetSkeletonMarkerOrLog(cfg)` before returning nil.
  - /Users/leeovery/Code/portal/cmd/state_hydrate.go:322–326 — shared unsetSkeletonMarkerOrLog delegates to state.UnsetSkeletonMarkerForFIFO and logs soft WARN on failure.
  - /Users/leeovery/Code/portal/cmd/state_hydrate.go:103–116 — runHydrate timeout branch: invokes cfg.HandleTimeout, then time.Sleep(hydrateSettleSleep), then execShellOrHookAndExit.
- Notes: Handler now symmetric with handleHydrateFileMissing.

TESTS:
- Status: Adequate
- Coverage:
  - /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:1083–1118 — `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` flipped from original DoesNotUnset. Asserts exact argv `["set-option", "-su", "@portal-skeleton-tu__0.0"]` exactly once.
  - FIFO basename `hydrate-tu__0.0.fifo` → paneKey `tu__0.0` via state.PaneKeyFromFIFOPath — edge case (b).
  - /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:1212–1253 — `TestHydrate_TimeoutHandler_OrderingAndTimingInvariants`.
  - /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:1050–1067 — `TestHydrate_Timeout_PreservesSettleSleepBeforeExec`.
  - /Users/leeovery/Code/portal/cmd/state_hydrate_test.go:1155–1172 — `TestHydrate_TimeoutExecsShell`.

CODE QUALITY:
- Project conventions: Followed. No t.Parallel(). recordingCommander is canonical cmd-layer DI shape. Nil *state.Logger is no-op safe.
- SOLID: Good. unsetSkeletonMarkerOrLog shared by three call sites.
- Complexity: Low.
- Modern idioms: Yes.
- Readability: Good. Doc comment at lines 248–259 cites spec.
- Issues: None.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- [idea] No explicit unit test forces underlying set-option -su to return an error and asserts WARN log + exec proceeds. Edge case (a) guaranteed architecturally (void-returning helper) but not pinned by a timeout-specific regression test.
- [idea] `TestHydrate_TimeoutUnsetsSkeletonMarkerWithSetOptionSU` argv-match loop reimplements inline deep-equality; sibling test uses reflect.DeepEqual. Could extract small `countArgvMatches` helper.
