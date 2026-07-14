package spawn

import (
	"log/slog"
	"os"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/tmux"
)

// spawnLogger is the spawn-component-bound package logger. Binding it once at
// package init via log.For introduces the new `spawn` component into Portal's
// closed logging taxonomy (a spec-governed amendment, not a call-site
// invention) and routes every record through the shared handler indirection so
// it observes later log.Init / SetTestHandler swaps.
//
// It is deliberately NOT named a bare `logger`: `logger` reads as a
// function-parameter name and a package var of that name invites shadowing.
// internal/spawn may import internal/log (internal/log never imports
// internal/spawn), so this binding is cycle-free.
var spawnLogger = log.For("spawn")

// spawn detection-outcome message strings — the Phase-1 slice of the `spawn`
// closed event catalog (detection outcome: identity / NULL-bundle / transient).
// The handler renders each under the `spawn:` component prefix.
const (
	msgDetectionResolved   = "detection resolved host terminal"
	msgDetectionNullBundle = "detection resolved no host-local terminal"
	msgDetectionTransient  = "detection transient failure"
)

// Detection-route detail strings — the opaque `detail` attr value describing
// which resolution path produced the outcome. The value is opaque to consumers
// (its presence is the contract, not its exact text).
const (
	routeInsideTmux  = "inside-tmux client walk"
	routeOutsideTmux = "outside-tmux env/self walk"
)

// Detector orchestrates host-terminal detection: it branches on whether Portal
// is running inside tmux, drives the appropriate resolution path (client-walk
// inside, env fast-path + self-walk outside), folds a transient failure into
// the same NULL outcome as a clean no-host-local result, and emits the
// spawn-component detection-outcome breadcrumb.
//
// Every field is an injectable seam so Detect is unit-testable with fabricated
// ancestry, client sets, and a capture logger — no real tmux, ps, or defaults.
// NewDetector wires the production seams.
type Detector struct {
	insideTmux     func() bool
	getenv         func(string) string
	selfPID        int
	walker         ProcessWalker
	reader         BundleReader
	lister         clientLister
	currentSession func() (string, error)
	logger         *slog.Logger
}

// NewDetector builds the production Detector, wiring the real seams: tmux.
// InsideTmux for the branch, os.Getenv / os.Getpid for the outside path, the
// real `ps`/`defaults`-backed walker and reader, the tmux client-list adapter
// and current-session read for the inside path, and the spawn-component logger.
func NewDetector(client *tmux.Client) *Detector {
	return &Detector{
		insideTmux:     tmux.InsideTmux,
		getenv:         os.Getenv,
		selfPID:        os.Getpid(),
		walker:         realProcessWalker{},
		reader:         realBundleReader{},
		lister:         tmuxClientLister{c: client},
		currentSession: client.CurrentSessionName,
		logger:         spawnLogger,
	}
}

// Detect resolves the host-terminal Identity and emits exactly one
// spawn-component detection-outcome record:
//
//   - transient failure (a flaky ps/list-clients, or an unreadable current
//     session): a WARN carrying the opaque underlying-error detail, folded to
//     the NULL identity — the same unsupported/no-op path as a clean NULL.
//   - resolved host-local terminal: an INFO carrying terminal + bundle_id and
//     the opaque route detail.
//   - clean NULL (remote/mosh, or no local client): an INFO NULL-bundle outcome
//     with neither terminal nor bundle_id, and no WARN.
func (d *Detector) Detect() Identity {
	id, route, err := d.resolve()

	switch {
	case err != nil:
		d.logger.Warn(msgDetectionTransient, "detail", err.Error())
		return Identity{}
	case !id.IsNull():
		d.logger.Info(msgDetectionResolved, "terminal", id.Name, "bundle_id", id.BundleID, "detail", route)
		return id
	default:
		d.logger.Info(msgDetectionNullBundle, "detail", route)
		return Identity{}
	}
}

// resolve runs the branch-appropriate detection path and returns its identity,
// the route detail describing that path, and any error. It normalises the
// current-session read failure into an ErrDetectTransient-wrapped error so
// Detect treats it uniformly with a mid-walk transient failure; every non-nil
// error returned here therefore satisfies errors.Is(err, ErrDetectTransient).
func (d *Detector) resolve() (Identity, string, error) {
	if d.insideTmux() {
		session, err := d.currentSession()
		if err != nil {
			return Identity{}, routeInsideTmux, transient("resolve current tmux session", err)
		}
		id, derr := detectInsideTmux(session, d.lister, d.walker, d.reader)
		return id, routeInsideTmux, derr
	}

	id, derr := detectOutsideTmux(d.getenv, d.selfPID, d.walker, d.reader)
	return id, routeOutsideTmux, derr
}
