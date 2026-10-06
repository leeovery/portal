## Attempt 1

ISSUES:
- /Users/leeovery/Code/portal/internal/state/commit.go:107-123 — `nameless` keeps everything except the name and scrollback paths: the environment, each window's name, active and zoomed flags, and each pane's folder, active flag, running program and pane token. So a rename logs a drop whenever any of those fields changed between the last save on disk and the commit that follows the rename. That happens in normal use:
  - The daemon only captures when `save.requested` is set or every 30s (`MaxGap`) (/Users/leeovery/Code/portal/cmd/state_daemon.go:186-191, 463).
  - `session-renamed` triggers a capture (/Users/leeovery/Code/portal/internal/tmux/hooks_register.go:25), but nothing fires when the user changes folder, starts a program, or tmux renames a window to the program it is running (on by default). So the saved record can be up to 30s behind on exactly those fields, while the rename's own commit lands within about a second.
  - I checked this on a disposable `-L` socket. A session at `/private/tmp` running `zsh` ran `cd / && sleep 30` and was then renamed. Afterwards the window name and running program both read `gsleep` and the folder read `/`, while the window layout `b25d,80x24,0,0,0` was unchanged.
  - Concrete case: `portal open ~/proj` creates `proj-ab12cd` (the `session-created` capture records `zsh`). The user starts an editor and renames the session with prefix-$ within 30s. The log gets `session dropped session=proj-ab12cd`, which is the false trail this task exists to remove.
  FIX: Compare only the windows' layout strings, in window order, and drop the environment, window name/active/zoomed and every pane field from the comparison.
  - Why layouts work: each layout embeds tmux's pane ids. tmux never reuses a pane id within one server run, and a rename doesn't change them.
  - The layouts on disk stay current: a layout change, a new window and a closed window fire `window-layout-changed`, `window-linked` and `window-unlinked`. Those already trigger a capture within one tick.
  - Fixtures: `capturedSessionRecord` (/Users/leeovery/Code/portal/internal/state/commit_drop_log_test.go:158-169) gives every session the same layout `b25d,80x24,0,0,1`. Under the narrower identity, unrelated sessions would pair up, so each fixture session needs its own pane id in its layout (e.g. a parameter).
  - New tests: a unit test where the renamed record's pane folder, running program and window name also changed, expecting no drop line. In `/Users/leeovery/Code/portal/internal/state/commit_rename_realtmux_test.go`, change bravo's folder and start a program in its pane before `rename-session`, waiting until `pane_current_command` reads the new program back.
  ALTERNATIVE: Capture tmux's `#{session_id}` as a new optional `Session` field (missing in older files reads as empty, like `portal_pane_id`) and match on it, with empty never matching. That is tmux's own rename-stable identity, and it holds even when a layout change lands in the same tick as the rename. The cost is a new capture column, a schema field and edits across the capture fixtures. I recommend the layouts-only fix: it stays inside `commit.go` and needs no schema change.
  CONFIDENCE: medium

COMMENT_CORRECTIONS:
- /Users/leeovery/Code/portal/internal/state/commit.go:105-106 — "never" is wrong for tmux session groups (`new-session -t`). Their sessions share windows, so they share layouts and pane ids.
  OLD: // A window layout carries tmux's server-unique pane ids, so two live sessions
// never compare equal.
  NEW: // A window layout carries tmux's server-unique pane ids, so two live sessions
// compare equal only when they share their windows.

NOTES:
- The executor reported one unexplained `internal/state` failure in a concurrent run. I could not reproduce it: a full `go test ./internal/state` run and 10 runs of the four new tests all passed.
- `TestCommitTakesOneNewSessionAsTheRenameOfOneSavedSessionOnly` accepts either bravo or charlie as the dropped one. That is fine, since which of two identical records gets paired is not a contract.
- The `.tick/tasks.jsonl` change is workflow bookkeeping and was not reviewed.
