AGENT: duplication
FINDINGS: none
SUMMARY: I made a full fresh pass over the feature, including the phase-10 additions (the token match across the restore-to-wait hand-over, commit-now reading the skeleton markers itself through CaptureAndRefile, and the hard link for a moved mid-restore pane whose file another record shares). Nothing clears the floor. Each rule the feature added still has one owner:
- the token-shape gate and the token-derived name (PendingScrollbackPath / PendingScrollbackFile), shared by the re-file, the link and the preview
- the error rules for the placement (placeStoredScrollback)
- the previous-record lookup and the content carry (takePrevRecord / carryPrevContent), shared by both merges
- the scrollback skip (paneSkipsScrollback over CaptureCycle)
- the pending marker's name and presence rule
- the pending view read by the picker and by doctor
- the install resume mode (installResumeModeOf), read by hydrate and by doctor
- the hand-off to a hook or a shell, the chain argv, the act keys, the canvas fill and the capped wrap

The near-copies below fall under the floor:
- refilePendingPane and linkMovedPane repeat only the already-filed check and the record update.
- The four index walks in internal/state repeat only loop scaffolding around SanitizePaneKey, and tests with non-contiguous indices pin each one.
- The leave-screen-then-clear order in the waiter, the recovery tail and the shell backstop is pinned by an order test at each site.
- The draw and wait flag decodes are covered by round-trip tests.
- The rest, which earlier cycles already held below the floor, still are: the process-role subcommand literals, the capture harness's own pending read, the hand-built shipped pair on the draw's degraded prefs path, dedupKeyOf's mirror of SeedHashMap's key, and the two raw-mode wrappers.

None of these could drift both silently and with real consequence.
