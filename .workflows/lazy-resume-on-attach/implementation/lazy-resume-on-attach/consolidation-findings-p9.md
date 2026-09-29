# Consolidation Findings: lazy-resume-on-attach (Phase 9)

## Findings

### F1: The preview restates the saver's rule for where a waiting pane's re-filed transcript lives
- **Class**: near-miss
- **Failure**: The picker preview's reader copies both halves of the re-file rule that `internal/state` owns. The first half is which panes get a token-named file: the `nanoid.IsTokenShaped` gate `refilePendingPane` applies. The second is where that file sits: `PendingScrollbackFile` joined onto the state dir through `FromSlash`, which is the unexported `joinStored`. The state package can change either half without the preview following: widen or narrow the gate, or change how the stored path joins onto the directory. Then every affected waiting pane shows `(no saved content)` in the preview for its whole wait, which can run for days. A pane tmux has moved shows the capture of the pane now at its old address. No test fails. The state tests never exercise the preview, and the preview's own tests build the token-file path with the same copied composition, so they keep staging and reading the old location. A user notices only by pressing Space on a session that holds a waiting pane. This is the defect this phase's preview task existed to fix.
- **Evidence**:
  - `internal/tui/preview_adapter.go:28-29`: the copy (gate plus `filepath.Join(a.stateDir, filepath.FromSlash(state.PendingScrollbackFile(...)))`), with its comment restating the rule at `:23-26`.
  - `internal/state/scrollback.go:121-125`: the original gate and name in `refilePendingPane`.
  - `internal/state/scrollback.go:143-147`: where `renameStoredScrollback` joins both paths.
  - `internal/state/scrollback.go:175-177`: `joinStored`.
  - `internal/state/paths.go:92-96`: `PendingScrollbackFile`.
  - Why the drift would pass: `internal/tui/pagepreview_waiting_pane_test.go:25` and `:145` compose the path the same copied way.
- **Proposed shape**:
  - Declare the rule once in `internal/state`: add `PendingScrollbackPath(dir, token string) (path string, ok bool)` beside `PendingScrollbackFile` in `paths.go`. `ok` is false when the token is not token-shaped. `path` is `joinStored(dir, PendingScrollbackFile(token))`.
  - `refilePendingPane` takes its gate from `ok`. It keeps `PendingScrollbackFile` for the relative form the record stores.
  - The adapter replaces lines 28-29 with one call and drops its `nanoid` and `path/filepath` imports. Its comment then no longer restates the state package's rule.
  - This is a pure refactor: the same files are read and written as today.
- **Bank**:
  - Executor, deposited twice: export a state helper giving the absolute path of a waiting pane's token-named scrollback file, so the preview stops rebuilding the `joinStored` composition.
  - Reviewer: let `internal/state` own the whole rule, token-shape check plus stored-path join; the token-shape half is where silent drift is realistic.

## Spec Defects

### S1: §7.2 and §10 still accept a preview artifact that this phase closed
- **Claim**:
  - §7.2 (`specification.md:312`): "One consequence is accepted rather than closed here: the picker's scrollback preview shows nothing for a waiting pane that has been rearranged. That preview resolves a saved transcript from the pane's live position rather than from the record … Closing it means resolving the preview through the pane's token too, which belongs with the wider migration (§10)."
  - §10 (`specification.md:423`): "This feature does the narrow half … and leaves the preview artifact that section names standing. The wider migration closes that artifact …"
- **Observed**:
  - The preview now resolves a waiting pane through its token. The live token and pending columns ride the preview's single enumeration (`internal/tmux/tmux.go:590-594`, `:659`). `previewModel.currentScrollback` (`internal/tui/pagepreview.go:260-267`) hands the token to the reader only while the pane waits. The reader then reads `scrollback/pane-<token>.bin` first and falls back to the positional file only on `(nil, nil)` (`internal/tui/preview_adapter.go:27-36`).
  - The `TestPreviewWaitingPane_*` suite (`internal/tui/pagepreview_waiting_pane_test.go`) pins this, for moved and unmoved panes alike.
  - The claim's scope was also wrong before the change. The re-file moves every tokened waiting pane's bytes, moved or not (`refilePendingPane`, `internal/state/scrollback.go:121-136`), so the artifact reached every such pane, not only a rearranged one. For a moved pane the preview could show another pane's capture, not nothing.
  - Review task 5 (`review-tasks-c1.md`) states this corrigendum is owed once the change lands.
- **Read**: Spec stale. Both paragraphs need a corrigendum saying the preview resolves a waiting pane through its token, and that no preview artifact is accepted for a waiting pane. §10's remaining scope (stamping every pane, token-named scrollback for unfrozen panes, retiring the positional key) is unaffected.
