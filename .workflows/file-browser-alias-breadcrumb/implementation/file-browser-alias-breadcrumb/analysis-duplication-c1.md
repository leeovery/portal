AGENT: duplication
CYCLE: 1
STATUS: clean
FINDINGS: none

SUMMARY: No significant duplication detected. This work unit is a pure removal of the
file-browser feature — it adds no code and writes no new tests, so duplication-introduced
is impossible by construction. The two deleted packages (internal/ui/, internal/browser/)
are gone from disk with their tests. Surviving non-test edits (cmd/open.go,
internal/tui/model.go) are clean deletions with no near-duplicate survivor left behind.
Survivor test files carry no orphaned browser helpers (zero residual
mockDirLister/stubDirLister/browser./ui./DirLister references). The mandated rename
TestCommandPendingBrowseAndNKey → TestCommandPendingNKey is coherent (n-key-only body, no
stale browser setup). Docs scrubbed per manifest. Remaining "browse" grep hits
(cmd/open_initial_mode_test.go:28, internal/tui/switch_view_key_test.go:29,
internal/tui/model_test.go:2162) all pre-date this work and refer to the unrelated
session-list switch-view (s key) / projects empty-state banner — not findings.
go build ./... green.
