package session

import (
	"github.com/leeovery/portal/internal/resolver"
	"github.com/leeovery/portal/internal/tmux"
)

// PaneStamper is the combined seam the lazy stamp-on-render fallback consumes:
// it reads the active pane's current_path (to derive an un-stamped session's
// directory) AND stamps the resolved @portal-dir back onto the session so
// subsequent renders take the fast path. It embeds PaneCurrentPathReader so the
// "active pane only" derivation contract is reused unchanged, and adds the
// single SetSessionOption write. *tmux.Client satisfies both halves.
type PaneStamper interface {
	PaneCurrentPathReader
	SetSessionOption(session, name, value string) error
}

// Compile-time proof that the production *tmux.Client satisfies the combined
// reader+stamper seam.
var _ PaneStamper = (*tmux.Client)(nil)

// ResolveAndStampDir is the grouped render's per-session derive-use-then-stamp
// helper for the @portal-dir lazy fallback. currentDir is the session's
// @portal-dir value as surfaced by ListSessions (tmux.Session.Dir).
//
// Fast path — currentDir != "": the stamp already exists, so it is returned as
// (currentDir, true) without reading panes, deriving a git-root, or re-stamping.
//
// Lazy path — currentDir == "": the directory is derived live from the active
// pane via ResolveSessionDir, then best-effort cached:
//
//   - unresolvable (ok==false from the resolver: killed mid-resolve, blank pane,
//     or no enclosing path): return ("", false) and stamp nothing. The session
//     falls to the Unknown/Untagged bucket (Phase 2) and is re-attempted next
//     render.
//   - resolvable (ok==true): the derived canonical directory is BOTH (a) the
//     value returned for THIS render and (b) best-effort stamped onto the
//     session via SetSessionOption(session, PortalDirOption, dir). The derived
//     value is returned irrespective of the write outcome — a write failure is
//     swallowed (leaving Dir empty so the next render re-derives + re-stamps)
//     and never drops the session from the view.
//
// A non-nil error from the resolver's pane read (a non-churn transport fault)
// is treated identically to "unresolvable this pass": the session is not
// stamped and routes to Unknown/Untagged, to be re-attempted next render. The
// render must never abort, so no error is propagated.
func ResolveAndStampDir(session, currentDir string, deps PaneStamper, runner resolver.CommandRunner) (string, bool) {
	// Fast path: the stamp is already present — use it verbatim, no derivation.
	if currentDir != "" {
		return currentDir, true
	}

	// Lazy path: derive the directory from the active pane. Any unresolvable or
	// transport-fault result is non-fatal — nothing to stamp, route to
	// Unknown/Untagged, re-attempt next render.
	dir, ok, err := ResolveSessionDir(session, deps, runner)
	if err != nil || !ok {
		return "", false
	}

	// Ordering is load-bearing: the derived dir is the render value FIRST; the
	// stamp is a side-effect that only accelerates subsequent renders.
	//
	// Best-effort stamp. A write failure leaves @portal-dir empty so the next
	// render re-enters this lazy path and re-derives + re-stamps. Swallowed
	// silently: the session package has no log component and the closed
	// component vocabulary does not include one (see create.go CreateFromDir).
	_ = deps.SetSessionOption(session, PortalDirOption, dir)

	return dir, true
}
