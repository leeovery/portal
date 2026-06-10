---
status: complete
created: 2026-06-10
cycle: 2
phase: Gap Analysis
topic: file-browser-alias-breadcrumb
---

# Review Tracking: File Browser Alias Breadcrumb - Gap Analysis

## Findings

### 1. Removal manifest omits the preview surface-audit allow-list, which names both deleted packages

**Source**: Specification analysis
**Category**: Gap/Ambiguity
**Affects**: Removal Manifest → "Other `*_test.go` (incidental coupling — preview tests)"; Removal Manifest → "Re-sweep at implementation start (required)"; Acceptance Criteria & Testing → Acceptance gate ("Zero remaining references")

**Details**:
`internal/tui/pagepreview_surface_audit_test.go` contains a `preExistingPackages` allow-list (the `TestSurfaceAudit_NoNewPackageForPreview` test) that includes two entries naming the packages this spec deletes:
- L295: `"browser": {},`
- L321: `"ui": {},`

After the removal deletes `internal/ui/` and `internal/browser/`, these two map keys become stale references to directories that no longer exist. This is precisely the class of dangling reference the spec's acceptance gate calls out as a problem ("Zero remaining references… A grep hit in a non-compiled context… that the build/test gate would miss must also be reconciled").

Three concrete problems, each genuinely new (not addressed by cycle 1):

1. **Not in the manifest.** The spec already enumerates an "Other `*_test.go` (incidental coupling — preview tests)" subsection listing three preview test files (`pagepreview_entry_test.go`, `pagepreview_refetch_test.go`, `pagepreview_bracket_test.go`) but omits this fourth preview test file, even though it carries two live references to the removed packages. The manifest claims its site *set* is "complete as of that date" — this is a known omission from that set.

2. **The build/test gate does not catch it.** The audit test iterates over directories that exist on disk and checks each against the allow-list. An allow-list entry with no corresponding directory is simply unused — the test stays green after removal. So the spec's authoritative gate (green `go build` + green `go test`) passes while the stale entries survive.

3. **The prescribed re-sweep does not catch the `"ui"` entry.** The spec's required re-sweep greps for `internal/ui`, `internal/browser`, `pageFileBrowser`, `DirLister`, `WithDirLister`, and `b`/"browse". The allow-list uses the bare map keys `"browser"` and `"ui"`. The `b`/"browse" target happens to substring-match `"browser"`, but **no re-sweep target matches the bare `"ui"` key** (`internal/ui` requires the path prefix). So even an implementer who runs the re-sweep exactly as written would leave `pagepreview_surface_audit_test.go:321` pointing at a deleted package.

Net effect: an implementer following the manifest section-by-section, then running the prescribed re-sweep, then trusting the green build/test gate, would ship with two stale allow-list entries naming non-existent packages — contradicting the spec's own "Zero remaining references" goal. The fix is to add this file to the manifest (remove the two allow-list keys) and to broaden the re-sweep grep targets to include the bare `ui`/`browser` tokens (or simply note that bare-token allow-list maps must be reconciled).

**Proposed Addition**:
1. New manifest bullet under "Other `*_test.go` (incidental coupling — preview tests)": `internal/tui/pagepreview_surface_audit_test.go` — remove the stale `"browser":` (L295) and `"ui":` (L321) keys from `TestSurfaceAudit_NoNewPackageForPreview`'s `preExistingPackages` allow-list, with a note that the build/test gate misses this (unused allow-list keys leave the test green) and the bare `"ui"` key escapes the path-prefixed re-sweep grep.
2. Broaden the "Re-sweep at implementation start (required)" grep targets to also include the bare quoted tokens `"ui"` and `"browser"` for allow-list/map-key references.

**Resolution**: Approved
**Notes**: Verified against the live tree — `internal/tui/pagepreview_surface_audit_test.go:295` (`"browser"`) and `:321` (`"ui"`) are real stale allow-list entries that survive removal undetected. Logged both edits. Approved via auto.
