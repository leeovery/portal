# Consolidation Findings: killing-all-sessions-wipes-restore-state (Phase 3)

## Findings

None cleared the bar.

## Comment Corrections

- internal/state/scrollback.go:347-350 — the `captureAndRefile` doc restates, branch by branch, the rule `classifyFailedCapture` (scrollback.go:375-386) already carries in its own code and comment, so the two descriptions drift apart on the first edit to either
  OLD: // set and the error the capture gave, classified by the confirmation sent after
// it unless one of its reads was already refused: refused, or not answered by
// ownServer, wraps that error in ErrTmuxStoppedAnswering, while an answer from
// ownServer, or an ownServer unknown and so sent nothing, leaves it unchanged. A
  NEW: // set and the error the capture gave, as classifyFailedCapture classifies it. A

## Spec Defects

### S1: An answered lazy pane's transcript is judged at a path that no longer holds it
- **Claim**: §2.4 (specification.md:96): "An empty capture may replace a saved non-empty transcript only once it is confirmed by the rule in §2.2 … An unconfirmed empty capture is not written. The saved transcript stands, and the refused write is logged (§4)." Read with §1.1: every session that is not killed stays restorable "with `sessions.json`, its scrollback files and its resume hooks intact".
- **Observed**: The writer that now owns this rule judges "saved transcript" at the pane's positional file alone. `ScrollbackWriter.Write` (internal/state/commit_cycle.go:64-69) takes only the pane key, and `confirmEmptyCapture` checks `savedTranscriptMayHoldBytes(ScrollbackFile(dir, paneKey))` (internal/state/scrollback.go:291).
  - Where the transcript lives: once a lazy pane has waited, its transcript is its token-named file. The housekeeping pass of the first commit naming that path removed the positional name (scrollback.go:95-98, `gcOrphanScrollback` at internal/state/commit.go:124). The dump skips the pane while it waits (scrollback.go:330-337), so nothing writes the positional name again.
  - What the answer changes: once the user answers, the marker is clear. `mergeFrozenPanes` carries a previous record only for panes in the pending set (internal/state/capture.go:327-345), so the pane gets a fresh record naming its positional path (capture.go:585).
  - The empty-capture loss: suppose that pane's `capture-pane` comes back empty in the first dumping cycle after the answer. That is tmux's shutdown answer to an in-flight read, landing after the cycle's own confirmation passed. The writer finds no positional file, sends no confirmation and writes the empty bytes (commit_cycle.go:65-68). The commit then names the empty positional file, and its housekeeping deletes the token-named transcript.
  - Two more ways the same transition loses the transcript with no empty capture at all:
    - that pane's `capture-pane` is refused: `dumpPane` writes nothing (cmd/state_daemon.go:332-341), but the commit still names the absent positional file and deletes the token-named one;
    - the first commit after the answer is a `commit-now`, which dumps nothing, and tmux exits before the daemon's next dump. That is §3.2's three-step loss, reached through a user answer shortly before shutdown instead of a marker cleared at it.
  - The window: a reboot or `kill-server` that lands before the first dumping cycle after an answer has finished. The daemon's idle cadence is 30s (`MaxGap`, cmd/state_daemon.go:463), and a notify-driven `save.requested` shortens it.
- **Read**: Code wrong for the empty-capture half. §2.4's saved transcript is whatever file the pane's previous record names, and the landed writer looks only at the positional one. The refused-capture and no-dump halves are genuinely open. §2.3 covers only cycles that end uncommitted. §2.4 covers only empty captures. §5.2's residue does not list a committed cycle whose record moved off its token-named file before its bytes were rewritten.

### S2: §2.4 points the scrollback write at a call site the phase moved
- **Claim**: §2.4 (specification.md:92): "The dump writes a pane's scrollback read whenever it differs from the saved file (`WriteScrollbackIfChanged`, called from the dump in `cmd/state_daemon.go`)."
- **Observed**: The dump now writes through the `ScrollbackWriter` the commit cycle hands it (`writer.Write`, cmd/state_daemon.go:342). `WriteScrollbackIfChanged` is called only from `ScrollbackWriter.Write` (internal/state/commit_cycle.go:68). The commit guard refuses the name in any non-test file outside `internal/state` (internal/state/commit_guard_test.go:175-178). `rg -n WriteScrollbackIfChanged cmd/state_daemon.go` → no matches.
- **Read**: Spec stale. This is a call-site pointer in the defect description, and it moved when the empty-capture confirmation went inside the commit cycle.
