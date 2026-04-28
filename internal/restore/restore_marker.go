package restore

import "fmt"

// restoringMarker is the tmux server-option name used by bootstrap to signal
// that skeleton restoration is in progress. The save daemon honours this
// marker by skipping its tick loop while the marker is set, so the in-flight
// restore cascade of structural events does not trigger a partial-state save
// (see specification, Restore-Side Architecture → Marker Coordination).
const restoringMarker = "@portal-restoring"

// SetRestoring sets @portal-restoring=1 at server scope. A failure here is
// fatal to bootstrap because skeleton-restore would otherwise race the save
// daemon's tick (the Bootstrap Flow → Ordering Rationale section of the spec
// explains why the marker must precede every restore-side tmux call).
func (o *Orchestrator) SetRestoring() error {
	if err := o.Client.SetServerOption(restoringMarker, "1"); err != nil {
		return fmt.Errorf("set @portal-restoring: %w", err)
	}
	return nil
}

// ClearRestoring removes @portal-restoring at server scope. The marker is
// volatile — tmux server restart clears it implicitly — so a failure to clear
// is logged but never propagated; the next tmux server restart self-heals.
func (o *Orchestrator) ClearRestoring() error {
	if err := o.Client.UnsetServerOption(restoringMarker); err != nil {
		return fmt.Errorf("clear @portal-restoring: %w", err)
	}
	return nil
}

// RestoreWithMarker wraps Restore() with set/clear of @portal-restoring.
//
// Failure to SET the marker is fatal: Restore is not invoked and the wrapped
// error is returned to the bootstrap caller per the Observability section's
// "Fatal Bootstrap Errors" entry for `@portal-restoring set-option fails`.
//
// Failure to CLEAR the marker is logged via the orchestrator's Logger but
// never propagated. The marker is a server-scope option lost on next tmux
// server restart, so the failure is self-healing and must not mask the
// outcome of Restore() itself.
//
// The clear is registered via defer so it runs on every exit path — normal
// return, Restore returning an error, or Restore panicking. A panic from
// Restore propagates to the caller AFTER the deferred clear has fired.
func (o *Orchestrator) RestoreWithMarker() (retErr error) {
	if err := o.SetRestoring(); err != nil {
		return err
	}
	defer func() {
		if err := o.ClearRestoring(); err != nil && o.Logger != nil {
			o.Logger.Warn("restore", "ClearRestoring: %v", err)
		}
	}()
	return o.Restore()
}
