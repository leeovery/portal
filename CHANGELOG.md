# Changelog

All notable user-facing changes to Portal are recorded here. This file
follows the spirit of [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added

- **Session scrollback preview.** Press `Space` on the highlighted session in
  the TUI to open a Quick Look-style preview of that session's saved
  scrollback. Within the preview: `]` / `[` cycle windows (wraps), `Tab`
  cycles panes within the focused window, viewport scroll keys
  (`↑`/`↓`/`j`/`k`/`PgUp`/`PgDn`/`Home`/`End`) scroll the loaded buffer, and
  `Esc` returns to the sessions list. Each pane shows the last ~1000 lines of
  saved scrollback. Preview is read-only — opening and dismissing leaves the
  session byte-identical (no hydration, no resume-hook firing, no tmux state
  mutation). Brand-new panes with no captures yet render `(no saved
  content)`.

### Upgrade Notes

- After upgrading, restart your tmux server (`tmux kill-server`) once to
  clear any leftover `0` session created by the previous version.
