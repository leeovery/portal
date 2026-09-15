## Attempt 1

ISSUES:
- `internal/tui/model.go:1514` — `return m, tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore(), m.maybeDispatchDetectionCmd())` is the exact shape this task exists to eliminate, newly introduced inside the helper it converted. `surfaceBufferedWarnings` (`internal/tui/bootstrap_warnings.go:55`, pointer receiver) clears `bufferedWarnings` and calls `setFlash`, which bumps `flashGen` and re-syncs both page layouts; `maybeDispatchDetectionCmd` (`internal/tui/spawn_detect.go:47`, pointer receiver, comment: "the latch mutation must persist onto the model Update returns") sets `detectDispatched`. Both now mutate `dismissLoadingGate`'s own value receiver, and the plain operand `m` that carries those mutations out is read in the same return statement as the calls — the order between a return statement's non-call operand and its calls is unspecified by the Go spec, which is verbatim the hazard the task's Problem statement cites. Before this change no such reliance existed at this site: the pointer receiver landed the mutations on the caller's `m`, which was returned on its own line. Verified empirically (go1.27.1, optimized and `-gcflags=all='-N -l'`) that gc evaluates the calls first today, so this is latent rather than live and every test passes — which is precisely why it would ship unnoticed. If the order ever flips, the cold + TUI route silently swallows every soft bootstrap warning (the band is set on the discarded copy while `flashTickCmd` carries the bumped generation the returned model does not have) and walks terminal detection a second time per picker session.
  FIX: assign the batch to a local before returning, preserving the sequence exactly:
  ```go
  m.transitionFromLoading()
  cmd := tea.Batch(m.surfaceBufferedWarnings(), m.refetchSessionsAfterRestore(), m.maybeDispatchDetectionCmd())
  return m, cmd
  ```
  Order among the three arguments is unaffected — call operands within one expression are specified left-to-right — so the "decision precedes surfaceBufferedWarnings" sequence the doc comment above the func names still holds. No comment or test is needed; this is the same compiler-carried shape the rest of the task adopts.
  CONFIDENCE: high

NOTES:
- The `:=` shadowing at `internal/tui/model.go:1629` and `:1672` is safe. In both arms `m, cmd := m.dismissLoadingGate()` is immediately followed by `return m, cmd` as the only route out of the block, so the shadowed copy is what leaves; nothing after either block reads the outer `m`. The `cmd` shadow does not disturb `Update`'s `defer func() { cmd = tea.Batch(closed, cmd) }()` either — a `return x, y` assigns the enclosing named results before defers run, and that defer is only registered on the `tea.WindowSizeMsg` path, which cannot reach these arms.
- The task body prescribed `m, cmd = m.dismissLoadingGate()` for those two sites; the executor used `:=`. Semantically identical as written, but `=` is the marginally more robust spelling, since it cannot be left behind by a later edit that moves the return out of the block.
