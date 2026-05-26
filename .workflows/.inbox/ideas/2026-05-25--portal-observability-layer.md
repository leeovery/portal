# Portal observability layer

Build out portal's logging into a deliberate observability layer that supports
post-mortem reconstruction across reboots, daemon lifecycles, and bootstrap
sequences — not just incidental log lines added when someone happened to
need them.

The seed for this idea was a reboot where some Claude `--resume` hooks fired
and others didn't, and `portal.log` couldn't tell us which path each helper
actually took. The same shape of debugging gap shows up across several
unrelated subsystems: silent error paths, missing tick-level summaries,
discarded diagnostic context at boundaries, and inconsistent log prefixes
that defeat grep-based audit trails. The fix isn't a one-off patch — it's a
coherent set of patterns applied consistently across the codebase.

## The patterns we want everywhere

1. **Subsystem-prefixed log lines** — every log entry begins with a
   subsystem key (`hydrate:`, `capture:`, `daemon:`, `bootstrap:`,
   `saver:`, `kill-barrier:`, etc.) so `grep '<key>:' portal.log` yields a
   complete chronological audit trail for that subsystem. The hydrate idea
   is the prototype: `grep 'hydrate:'` should reconstruct exactly which
   pane took which exit path on every helper invocation.

2. **Single INFO line per terminal point / decision point** — when a
   subsystem makes a meaningful choice (exit path taken, lookup hit/miss,
   command resolved, ordering branch chosen), emit one INFO line capturing
   the inputs and the outcome. Not a flood of state-change lines — one
   structured summary per decision. This is what makes the audit trail
   useful instead of overwhelming.

3. **DEBUG breadcrumbs on swallowed-error paths** — every time we
   intentionally proceed past an error (errors.Is/As shape match,
   ESRCH-after-kill, ENOENT-treated-as-empty, "treat any error as
   transient"), emit a DEBUG line so a post-mortem with
   `PORTAL_LOG_LEVEL=debug` can reconstruct what happened. The point is
   not to log the error in production — it's to keep the breadcrumb
   available for the moment we need it.

4. **Preserve diagnostic context at boundaries** — when a subprocess
   fails or an external command returns unexpected output, capture stderr
   alongside stdout and propagate both into the wrapped error. Discarding
   stderr is the most common form of "we lost the debug context exactly
   where we needed it most". Same principle for syscalls (`errno` text)
   and tmux command failures.

5. **Cycle-level summaries** — daemon ticks, capture loops, bootstrap
   sequences, orphan sweeps — every cycle emits a single INFO summary at
   completion ("tick: 3 sessions, 12 panes, 2 natural-churn skips, 18ms"
   or similar) so an operator can reconstruct what happened over a
   window without needing per-event lines. Per-event WARNs still fire on
   anomalies; the summary is the steady-state grep target.

6. **Log-level discipline** — a clear contract for what goes where:
   - DEBUG: breadcrumbs on swallowed-error paths, per-event state changes
     under cycle summaries, observed transient values.
   - INFO: decision-point summaries, cycle summaries, lifecycle events
     (daemon start/exit, bootstrap step transitions, self-eject markers).
   - WARN: unexpected-but-recoverable conditions (per-session capture
     failures classified as anomalous, retries triggered, transient probe
     failures inside hysteresis).
   - ERROR: unrecoverable — production code should rarely emit these
     because the daemon swallows-and-continues by design. Non-contention
     lock-failure is one current legitimate ERROR site (recently
     re-classified to WARN in cycle-2 review remediation; see
     slow-open-empty-previews-and-zombie-sessions T11-1 for the
     rationale).

7. **Log-level propagation verified end-to-end** — `PORTAL_LOG_LEVEL`
   must actually take effect through the test → tmux server →
   respawn-pane'd daemon chain. Today this is implicit and fragile;
   adding a positive log-marker assertion (the daemon emits a one-line
   "log level: %s" at start under the daemon component, tests assert on
   it) makes the chain auditable. Without this, raising DEBUG coverage
   silently fails in environments where the env var doesn't propagate.

## Concrete near-term work

The original seed for this idea — `cmd/state_hydrate.go` — remains the
right first chunk. Implementing the four exit-path INFO lines plus the
`execShellOrHookAndExit` lookup-decision logging gives us a complete
post-reboot audit trail for hook firing and proves out patterns (1) and
(2) on a single self-contained subsystem.

The hydrate gap (verbatim from the original idea):

- silent ENOENT exit at `cmd/state_hydrate.go` ~120 (helper opened FIFO
  and got "no such file or directory" — never reaches the hook)
- timeout path at ~115 (helper waited 3 s, gave up, fires hook)
- file-missing path at ~147 (scrollback couldn't be read, fires hook)
- success path at ~188 (signal arrived, scrollback dumped, fires hook)

Each terminal point emits a single INFO line including `hook-key` and the
resolved hook command (or `<none>` if no hook registered).
`execShellOrHookAndExit` itself logs lookup decisions — hit vs miss vs
error — so we can distinguish "hooks.json drifted from the saved
hook-key" from "helper never reached the lookup."

After this, `grep 'hydrate:' portal.log` gives a complete per-pane audit
trail of the resurrection.

## Additional concrete gap-closures surfaced by review

Each of these is small in isolation and unworthy of its own work-unit
ceremony — but together they fall under the same observability initiative
and should ship as one coherent improvement.

- **`defaultIdentifyPS` discards stderr** (`internal/state/daemon_identity.go`,
  cycle-1 review #20 / T1-1). Use `.CombinedOutput()` or capture
  `cmd.Stderr` into a buffer and append it to the wrapped error on
  failure. Pattern (4) — preserve diagnostic context.

- **Capture-tick lacks all-natural-churn summary** (`internal/state/capture.go`,
  cycle-1 review #24 / T2-3). When every per-session error in a tick
  classified as natural-churn (session ended cleanly during the
  capture), the tick currently emits N per-session WARNs and no
  summary. Add a single INFO "capture: tick complete, N natural-churn
  skips, 0 anomalous" to give postmortems a grep target. Pattern (5) —
  cycle-level summary.

- **Capture log format harmonisation** (`internal/state/capture.go`,
  cycle-1 review #23 / T2-3). Harmonise the per-session WARN format to
  `"capture: <natural-churn|anomalous> session %q: %v"` so a single
  classification grep works. Pattern (1) — subsystem prefix.

- **`escalateKillToSIGKILL` swallows non-ESRCH errors**
  (`internal/tmux/portal_saver.go`, cycle-1 review #28 / T4-1).
  `SendSIGKILL` returning a non-ESRCH error is currently logged at no
  level and proceeds. Add a DEBUG breadcrumb naming the PID and the
  error. Pattern (3) — DEBUG breadcrumb on swallowed-error paths.

- **Hard-to-explain defensive branches go uncommented** (e.g.
  `defaultIdentifyPS` zero-exit + empty-stdout, treated as transient;
  `cmd/state_daemon.go` capture-tick log-and-continue). Future code
  archaeology benefits from a one-line "Why this branch exists" comment
  tied to the spec contract. Not strictly observability but adjacent —
  every silent fallthrough deserves either a log line or a comment, and
  preferably both.

## Architectural limit (carried forward from the hydrate seed)

Portal exec's hooks via `syscall.Exec`, replacing the helper process. So
portal will never see the hook command's own exit status (e.g. whether
`claude --resume <UUID>` actually launched Claude or exited immediately
with an invalid-session error). Capturing that would require wrapping
the exec'd command in a shell envelope that records exit status before
chaining to `$SHELL` — a separate, more invasive change with its own
correctness considerations. The exit-path logs above are the
high-signal-per-LOC win regardless of whether the wrapper idea is later
pursued.

## Why this matters

Most of these gaps were discovered the same way: a real incident
happened, the existing log lines weren't enough to reconstruct what
went wrong, and someone had to reverse-engineer the state from
circumstantial evidence (scrollback mtimes, hooks.json diffs, pgrep
output, tmux list-windows). Each individual gap looks tiny in isolation;
the cost shows up only when an incident catches the codebase missing
the breadcrumbs that would have made debugging straightforward.
Establishing the patterns once, and applying them consistently across
the subsystems that already have known gaps, costs less than the next
single incident's reconstruction time.

The investigation context that surfaced the hydrate-helper seed is
preserved in `MEMORY.md` (`project_reboot_hooks_followup`); this idea
is now the broader observability initiative that fell out of it,
enriched by parallel gaps surfaced during the cycle-1 review of
slow-open-empty-previews-and-zombie-sessions.
