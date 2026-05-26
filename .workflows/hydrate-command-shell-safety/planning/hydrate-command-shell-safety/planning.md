# Plan: Hydrate Command Shell Safety

## Phase 1: Apply Change

Restore shell-quoting in `buildHydrateCommand` and broaden `sanitizeSessionName` to an allowlist so resume works for any tmux session name.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| hydrate-command-shell-safety-1-1 | Restore shellQuoteSingle and apply to buildHydrateCommand interpolations | Embedded single quotes; whitespace; shell-meta bytes ($, backtick, ;); existing test pins need updating to quoted form |
| hydrate-command-shell-safety-1-2 | Broaden sanitizeSessionName to an allowlist `[A-Za-z0-9._-]` | Leading `.` still explicitly replaced to `_`; existing collision suffix handles new collisions; `isUnsafeByte` removed; existing five panekey sub-tests must remain green |
