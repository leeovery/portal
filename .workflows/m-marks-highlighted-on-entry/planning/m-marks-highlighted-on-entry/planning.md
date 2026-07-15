# Plan: M Marks Highlighted On Entry

## Phase 1: Apply Change

Mark the currently-highlighted session on multi-select entry, and sync the tests and docs to the new behaviour.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| m-marks-highlighted-on-entry-1-1 | Mark highlighted session on multi-select entry | Cursor on a `HeaderItem` row → enters with zero (the `selectedSessionItem()` `ok=false` path); zero-selected still reachable via double-`m` and `Esc`; a multi-tag By-Tag row marks the underlying session once (identity keyed on `Session.Name`) |
| m-marks-highlighted-on-entry-1-2 | Sync documentation to entry-marks-highlighted behaviour | The "you can also sit in the mode with nothing selected" statement stays true (zero-selected still reachable) — adjust only the entry-marks-nothing implication, don't over-rewrite |
