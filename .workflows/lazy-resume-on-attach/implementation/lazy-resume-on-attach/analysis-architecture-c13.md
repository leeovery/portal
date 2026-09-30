AGENT: architecture
FINDINGS: none
COMMENT_CORRECTIONS:
- internal/state/capture.go:342-343 — since the missed-session carry, the capture distinguishes panes the enumeration lists from panes in sessions it reached, and this set holds only the second; a live pane in a missed session carries a token it does not contain
  OLD: // liveTokenSet holds the token of every record in fresh: before any merge
// replaces a pane with a previous record, that is every live pane's token.
  NEW: // liveTokenSet holds the token of every record in fresh: before any merge
// replaces a pane with a previous record, that is the token of every live pane
// in a session the capture reached.
- internal/state/capture.go:39-42 — the address-match exclusion is built from liveTokenSet, so it covers only tokens carried by panes in reached sessions; a record whose token a live pane in a missed session carries can still be taken by address
  OLD: // address has become. A skipped pane carrying no token keeps the whole previous
// record at its own address, a pending one those three fields of it, unless a
// live pane carries that record's token. A stale marker never resurrects a
// killed pane.
  NEW: // address has become. A skipped pane carrying no token keeps the whole previous
// record at its own address, a pending one those three fields of it, unless a
// live pane in a session the capture reached carries that record's token. A
// stale marker never resurrects a killed pane.
SUMMARY: This was a full fresh pass over the resume chain (hydrate, draw, wait, recover, the shell backstop and the tty helpers), the stores' two shapes and the discard route, the capture's merge, carry, link and re-file, the locked commit cycle and the daemon's dump, the picker's dots, settle and preview, doctor, and the capture harness. Each piece composes from a single declaration: one argv composer with round-trip parse tests, one presence rule and option name for the pending marker, one skip rule, one pending-scrollback path, and one set of act keys. No candidate cleared the floor. Four candidates were dropped. PendingResumeView.Rows has no consumer. CaptureStructure is exported but only tests call it. A missed-session carry that collides with a reached name refuses the whole commit, which the spec mandates and which only a contrived rename sequence can make persistent. The preview's (nil, nil) fallback was dispositioned in cycle 12. The only remedies owed are two comment corrections to the capture's live-token claims, which predate the carry's distinction between listed panes and reached sessions.
