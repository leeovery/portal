# Structured discrimination for the convergence/teardown hook WARNs (+ optional DEBUG eviction detail)

Two observations on the hook-convergence logging, both enhancements (no current
gap):

1. The two convergence/teardown WARNs — `"show-hooks failed"` and
   `"failed to evict portal hook"` — are distinguished only by free-form message
   text. If a future log-grep / log-analysis consumer needs to discriminate them
   programmatically, a structured attr would be cleaner than message matching.
   NOTE: this is constrained today by the intentionally **closed** log attr
   taxonomy — adding an attr requires amending the spec's closed vocabulary, not
   a call-site invention. Treat as a taxonomy-amendment discussion, not a quick
   edit.

2. Per-event eviction detail is not emitted at DEBUG on the successful-eviction
   path (only the aggregate `reaped` INFO fires). The spec marks per-event DEBUG
   detail as optional ("may be emitted at DEBUG"), so this is purely an
   observability enhancement.

Source: review of state-notify-cascade-on-binary-upgrade/state-notify-cascade-on-binary-upgrade (recommendation #6)
