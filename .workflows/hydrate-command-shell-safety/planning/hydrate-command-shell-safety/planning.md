# Plan: Hydrate Command Shell Safety

## Phase 1: Apply Change

Restore shell-quoting in `buildHydrateCommand` and broaden `sanitizeSessionName` to an allowlist so resume works for any tmux session name.

#### Tasks
status: approved

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| hydrate-command-shell-safety-1-1 | Restore shellQuoteSingle and apply to buildHydrateCommand interpolations | Embedded single quotes; whitespace; shell-meta bytes ($, backtick, ;); existing test pins need updating to quoted form |
| hydrate-command-shell-safety-1-2 | Broaden sanitizeSessionName to an allowlist `[A-Za-z0-9._-]` | Leading `.` still explicitly replaced to `_`; existing collision suffix handles new collisions; `isUnsafeByte` removed; existing five panekey sub-tests must remain green |

### Phase 2: Analysis (Cycle 1)

Address findings from Analysis (Cycle 1).

#### Tasks

| Internal ID | Name | Edge Cases |
|-------------|------|------------|
| hydrate-command-shell-safety-2-1 | Extract sanitized-stem assertion helper in panekey_test.go | Preserve lowercase-hex check across all five collision-bearing sub-tests; helper signature accepts `(t, got, wantStem, w, p)`; no production code in `internal/state/` modified |
