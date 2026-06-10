TASK: 3-2 — Update the non-Go docs (README.md, CLAUDE.md)

ACCEPTANCE CRITERIA:
- README.md describes THREE TUI views; "file browser" gone, count word "three".
- CLAUDE.md tui-row page state machine reads "Loading → Sessions → Projects → Preview"; "(peer of pageFileBrowser)" removed; valid "pagePreview → pageSessions" transition mention LEFT INTACT.
- CLAUDE.md browser package-table row deleted.
- CLAUDE.md ui package-table row deleted.
- Case-insensitive grep of both files for file-browser tokens returns no surviving genuine reference.
- No edits to go.mod/go.sum/.goreleaser*/Makefile/shell-completion/embed/build-tag files.

STATUS: Complete

SPEC CONTEXT: Prose docs are invisible to the build/test gate; must be reconciled by hand.

IMPLEMENTATION:
- Status: Implemented
- README.md:253 reads "The TUI has three views: session list, project picker, and scrollback preview." (file browser gone, "three").
- CLAUDE.md tui row: state machine "Loading → Sessions → Projects → Preview" (no FileBrowser); "(peer of pageFileBrowser)" reworded to "The pagePreview arm renders a read-only scrollback preview when Space is pressed on the Sessions page"; valid "pagePreview → pageSessions" dismiss-handler mention survives intact (no over-deletion).
- CLAUDE.md browser and ui package-table rows both deleted (grep ^| `(ui|browser)` | no match).
- Case-insensitive grep of both files for file-browser tokens: No matches.
- Protected files (go.mod, go.sum, .goreleaser.yaml) untouched; no Makefile exists.

TESTS:
- Status: N/A (removal task — verification is the post-edit hand-grep, which is clean).

CODE QUALITY:
- N/A (prose only). Edit sites located by content match.

BLOCKING ISSUES:
- None.

NON-BLOCKING NOTES:
- None.
