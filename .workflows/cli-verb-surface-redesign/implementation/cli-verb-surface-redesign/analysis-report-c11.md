---
topic: cli-verb-surface-redesign
cycle: 11
total_findings: 5
deduplicated_findings: 5
proposed_tasks: 1
---
# Analysis Report: cli-verb-surface-redesign (Cycle 11)

## Summary
Cycle 11 confirms a converging, heavily-consolidated surface: standards is CLEAN for the third consecutive cycle, and all five raw findings (2 duplication, 3 architecture) are low-severity seam/tidy observations. Four are discarded — two as previously-considered-and-still-correctly-discarded, two as deliberate, documented, standards-verified design choices below the actionable bar. The single surviving finding is the one that clusters into a genuine pattern: the host-terminal resolution seam `func(spawn.Identity) (spawn.Adapter, spawn.Resolution)` is spelled inline at 8 sites while the spawn package already establishes a named-seam convention (`ExecutableResolver`).

## Discarded Findings
- Twin completion prefix-filter completers (duplication, low) — Verified still exactly 2 instances (cmd/completion.go:40-48, :81-89); the only other `strings.HasPrefix` hits in cmd/ are unrelated (marker-prefix match, flag detection). Below the Rule-of-Three, and already discarded in cycles 8, 9, and 10 as a legitimate 2-instance pattern. Posture unchanged — no third instance exists.
- Mass-deletion hazard condition re-authored in doctor's read-only stale-hooks check (duplication, low) — Deliberate, documented cmd-layer policy split: doctor.go:423-429 explicitly states the hazard guard "is NOT part of that shared predicate — it is a cmd-layer repair-safety policy applied here (and in runHookStaleCleanup)", and both sites cross-reference each other ("Mirror runHookStaleCleanup"). The duplicated part is a trivial 2-clause boolean (`len(live)==0 && len(persisted)>0`) with legitimately divergent bodies (checkResult vs Warn-and-defer). Standards verified this as conformant. Not a drift risk worth relocating the invariant out of its intentional cmd-layer home.
- Open-burst detector bypasses the anti-drift seam bundle (architecture, low) — Previously discarded in cycle 9: the split is a deliberate lazy-memoisation choice so a fully-injected burst never builds the whole bundle (avoiding an eager terminals.json load when only the detector needs defaulting). The finding's own text concedes this exact documented rationale. Both paths call `spawn.NewDetector(client)` identically today — no behavioural drift. Posture unchanged.
- Picker-launch prediction re-derived in bootstrap routing (architecture, low) — `isTUIPath` is documented, explicitly cross-referenced to open.go's RunE ("See cmd/open.go's RunE for the gating logic that mirrors this check"), pin-vector-guarded via the shared `openDomainPinFlags`/`anyOpenDomainPin`, agrees with RunE for every case today, and is covered by concurrent_*_test.go. The finding self-downgrades to "a latent self-containedness seam rather than an active bug" and its recommendation ("consider..." / "keep cross-reference comments in lockstep") is already satisfied. Below the actionable bar on a converging topic.
