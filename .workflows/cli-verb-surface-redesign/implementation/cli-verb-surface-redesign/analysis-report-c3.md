---
topic: cli-verb-surface-redesign
cycle: 3
total_findings: 6
deduplicated_findings: 6
proposed_tasks: 1
---
# Analysis Report: CLI Verb Surface Redesign (Cycle 3)

## Summary
Cycle 3 surfaced 6 findings (3 duplication, 3 architecture; standards returned clean). No cross-agent duplicates — all six are distinct. Exactly one is a genuine, low-risk, non-churn improvement: single-sourcing the command-on-attach usage-error string that is authored verbatim at both the single-target and multi-target guards, the lone shared open/burst message not yet centralised the way every sibling message is. The remaining five are latent fragilities, forced/below-threshold tidies, or spec-governed choices already considered — all discarded with rationale below.

## Discarded Findings
- uninstall cmd-layer substring match vs typed tmux sentinel (architecture, medium) — The clean fix is not available at low risk. `KillSession` (internal/tmux/tmux.go:386) does NOT route its error through `wrapNoSuchSession`, so it surfaces no wrappable `tmux.ErrNoSuchSession` for `errors.Is` to consume. Worse, the boundary discriminator matches only the case-sensitive `"no such session"` phrasing, while kill-session/has-session emit `"can't find session"` — so `ErrNoSuchSession` does not even cover this command family. Replacing the substring cleanly would require (a) routing a destructive, many-caller chokepoint (`KillSession`) through new boundary wrapping AND (b) broadening `ErrNoSuchSession`'s matching semantics for every existing consumer — a global-blast-radius boundary change, not a low-risk swap. The substring is pre-existing logic relocated verbatim from the deleted state_cleanup.go (not introduced by this feature), is HasSession-gated with a tiny race window, and is documented in-source. Per the actionability rule (no wrappable sentinel exists → cannot cleanly improve), discard.
- Twin completion prefix-filters — completeSessionNames / completeAliasKeys (duplication, low) — Only two instances (below Rule of Three), divergent intent (session names vs alias keys), and each keeps its own source seam + directive at the call site. Extraction is forced and low-value; discard.
- Repeated doctor server-down guard shape — checkDaemonAlive / checkSaverUp / checkHooksRegistered (duplication, low) — The detail string is already single-sourced via the doctorRuntimeNotRunning const; only a 3-line guard-and-return shape repeats. Keeping each check fully self-contained is deliberate per-check clarity; the finding itself flags it as optional. Discard.
- Raw-argv routing parser's unguarded no-value-flag assumption (architecture, low) — Already covered by the Task 7-3 flag-map↔cobra drift guard (TestOpenTargetPinsCoverValueTakingFlags) and documented in-source ("for review"). A deeper anchor-the-scan fix was a deliberate design tradeoff; the dual-parser is spec-mandated for order recovery. Latent, not an active bug. Discard.
- Three-way attach/mint modelling with lossy back-conversion — surfaceToResult Domain placeholder (architecture, low) — The bare-string domain vocabulary matches the spec's log-taxonomy string values; the single-surviving-surface path routes through openResolved, which ignores Domain and emits no further resolve line, so the placeholder is harmless. This spec-governed modelling was already considered and discarded in cycle 2. Latent trap only if a future consumer reads the degenerate Domain — not an active defect. Discard.
