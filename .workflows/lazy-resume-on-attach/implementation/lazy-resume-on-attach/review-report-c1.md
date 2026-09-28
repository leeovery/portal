# Review Report: Lazy Resume On Attach (Cycle 1)

## Stats

- Total findings: 35
- Deduplicated findings: 27
- Proposed tasks: 4

## Summary

The review's 35 findings deduplicated into 27 actions. The 22 contained corrections were already applied and committed this cycle (`f5896dcdd`), so none of them is proposed here. Four of the five planning actions are proposed as tasks, each with its direction settled:
- **Pending re-file:** can overwrite a waiting pane's saved transcript with another pane's history.
- **Colour probe on macOS:** paints every panel dark and echoes the terminal's reply onto it.
- **Waiter:** misses resizes and swallows keys after three Alt chords.
- **Capped command row:** can show a space the command does not have.

The fifth action, the Space preview showing nothing for every waiting pane, contradicts the scope the specification accepts, so it is recorded below as a spec defect rather than staged. No Decisions are staged.

## Spec Defects

### S1: Blank preview reaches every waiting pane, not only a rearranged one
- **Claim**: §7.2, `specification.md:312`: "One consequence is accepted rather than closed here: the picker's scrollback preview shows nothing for a waiting pane that has been rearranged." §10 (`:423`) repeats that scope: durable pane identity "leaves the preview artifact that section names standing".
- **Observed**: The same section's re-file rule, added by the 2026-09-21 corrigendum, says: "At the tick a pane is first frozen, its scrollback is re-filed under a name derived from its durable token." The code does exactly that. `refilePendingPane` (`internal/state/scrollback.go`) renames any stored path that is not already the token path, whether or not the pane moved. The preview still looks for the transcript under the pane's position:
  - It reads `state.ScrollbackFile(stateDir, paneKey)` (`internal/tui/preview_adapter.go:23`), with the key composed at `internal/tui/pagepreview.go:257`.
  - So every waiting pane that carries a token shows `(no saved content)` (`pagepreview.go:31`) for the whole wait.
  - The lazy integration suite's subject is never moved, yet its record names `PendingScrollbackFile(token)`.
  - Before this change-set, the same unmoved pane previewed its transcript.
  - The enumeration the preview uses, `tmux.ListWindowsAndPanesInSession` (`internal/tmux/tmux.go:586-590`), emits only window index, window name and pane index. `WindowGroup` carries `PaneIndices []int`.
  - Sources: 2-6-2, 7-protecting-the-waiting-pane-s-saved-transcript-8-seeing-what-is-waiting-1.
- **Read**: Genuinely open. The claim's scope is stale against the spec's own re-file rule, whichever way this is settled. Which side gives way is a product call:
  - **Widen the accepted consequence.** Token-resolved preview stays with durable pane identity. The feature ships a blank Space preview for every waiting pane under the default lazy mode, which after a reboot is roughly the forty-one panes the spec itself counts.
  - **Close the gap in this feature.** For a pending pane, the preview reads the token-named file.

  My lean is that the code is wrong. The acceptance was argued from the artifact being narrow and short-lived, which no longer holds on the default path. The pieces needed to close it already exist: `state.PendingScrollbackFile`, and the token and pending columns the capture format already reads.

  If it is routed back as code, the review's conditions apply:
  - Carry the token and pending columns on the preview's single existing `list-panes` enumeration.
  - For a pane not yet re-filed, fall back to the positional file through `TailScrollback`'s `(nil, nil)` not-found shape.
  - No `os.Stat` and no per-focus tmux read in `internal/tui` (`TestConstruction_ReadsNoThemesDirectory`, `TestPreviewHermetic_FullLifecycleProducesOnlyOpenEnumerationAndPerFocusReads`).
  - Expect the change to reshape `ListWindowsAndPanesInSession`'s format and return value, and the `ScrollbackReader.Tail` seam that every fake implements.

## Discarded Findings
- None. The review discarded no finding. The 22 do-now actions (A1–A22) were applied in this cycle rather than proposed.
