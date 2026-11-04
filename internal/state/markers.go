package state

// RestoringMarkerName is set while bootstrap builds the restore skeleton: the
// daemon skips captures entirely, so half-built structure is never recorded.
const RestoringMarkerName = "@portal-restoring"

// BootstrappedMarkerName holds the version-stamped bootstrap latch. Its value is
// load-bearing rather than its presence: satisfaction is equality against the
// running binary version, so an upgraded binary re-bootstraps on first command.
const BootstrappedMarkerName = "@portal-bootstrapped"

type RestoringChecker interface {
	TryGetServerOption(name string) (string, bool, error)
}

// IsRestoringSet treats absent and empty alike as false, but propagates a tmux
// error so a real failure cannot masquerade as "not restoring".
func IsRestoringSet(c RestoringChecker) (bool, error) {
	val, found, err := c.TryGetServerOption(RestoringMarkerName)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	return val != "", nil
}

// RestoreWindowActive folds a read of @portal-restoring into the posture work
// that must not disturb an in-flight restore takes: a failed read counts as
// set, so such work stands down absent proof the window is clear. It composes
// with any read of the marker — RestoreWindowActive(IsRestoringSet(c)), or a
// caller's own seam — and returns the read error alongside, already folded
// into the bool, for a caller that reports why it stood down.
func RestoreWindowActive(restoring bool, err error) (bool, error) {
	return restoring || err != nil, err
}

// BootstrappedLatchSatisfied requires the latch's value to equal runningVersion
// exactly. Absence, mismatch and read failure all report false, so a cold,
// upgraded or unreachable server takes the full-bootstrap path.
//
// Swallowing the read error into a bare bool is deliberate — do not "fix" this
// into a (bool, error). runningVersion is a parameter rather than a read of
// cmd.version so internal/state stays a leaf.
func BootstrappedLatchSatisfied(c RestoringChecker, runningVersion string) bool {
	val, found, err := c.TryGetServerOption(BootstrappedMarkerName)
	if err != nil {
		return false
	}
	if !found {
		return false
	}
	return val == runningVersion
}
