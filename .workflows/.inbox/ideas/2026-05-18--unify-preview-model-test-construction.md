# Unify open-coded previewModel construction across the preview test corpus

The wider `internal/tui/pagepreview_*_test.go` corpus still has many open-coded `stubEnumerator{groups: …}` + `recordingReader{bytes: …}` + `NewPreviewModel(…)` triples — e.g. in `pagepreview_brandnew_test.go`, `pagepreview_error_test.go`, `pagepreview_enter_test.go`, `pagepreview_tab_test.go`, the `preview_attach_*_test.go` files, and others.

The `preview-visual-distinction` work introduced `newFramePreviewModel` / `newFramePreviewModelAt` in `pagepreview_helpers_test.go` and migrated the three new frame-related call sites (cycle-1 task 2-2). The wider corpus was explicitly out of scope for that cycle.

A future sweep could unify these on a small family of helpers — possibly a variant that takes a `groups []tmux.WindowGroup` slice rather than a single window/pane pair, so multi-window/multi-pane fixtures are accommodated.

Scope as its own task rather than retroactively expanding the cycle-1 2-2 task.

Source: review of preview-visual-distinction/preview-visual-distinction
