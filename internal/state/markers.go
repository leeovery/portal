package state

import (
	"strings"

	"github.com/leeovery/portal/internal/tmuxout"
)

// SkeletonMarkerPrefix marks a pane as skeleton-restored and awaiting hydration.
// While set, the save loop skips the pane so its pre-boot scrollback file stays
// authoritative.
const SkeletonMarkerPrefix = "@portal-skeleton-"

// RestoringMarkerName is set while bootstrap builds the restore skeleton: the
// daemon skips captures entirely, so half-built structure is never recorded.
const RestoringMarkerName = "@portal-restoring"

// BootstrappedMarkerName holds the version-stamped bootstrap latch. Its value is
// load-bearing rather than its presence: satisfaction is equality against the
// running binary version, so an upgraded binary re-bootstraps on first command.
const BootstrappedMarkerName = "@portal-bootstrapped"

// PortalPaneIDOption names the tmux pane user-option holding a pane's durable
// identity token. Every format string, option argument and stamp must compose
// the name from here rather than restate the literal, so no two sites can drift.
const PortalPaneIDOption = "@portal-pane-id"

// ResumePendingOption names the tmux pane user-option marking a pane as waiting
// on a resume decision. Every format string and option argument must compose the
// name from here rather than restate the literal, so no two sites can drift.
const ResumePendingOption = "@portal-resume-pending"

// ServerOptionLister is declared here so internal/state need not import
// internal/tmux, which imports it back and would cycle.
type ServerOptionLister interface {
	ShowAllServerOptions() (string, error)
}

type RestoringChecker interface {
	TryGetServerOption(name string) (string, bool, error)
}

type ServerOptionWriter interface {
	SetServerOption(name, value string) error
	UnsetServerOption(name string) error
}

// PaneOptionWriter is declared here so internal/state need not import
// internal/tmux, which imports it back and would cycle. The target type is a
// parameter for the same reason: the tmux client takes its own named target
// type, and hardcoding string here would put this seam out of its reach.
type PaneOptionWriter[T ~string] interface {
	SetPaneOption(target T, name, value string) error
	UnsetPaneOption(target T, name string) error
}

// ListSkeletonMarkers returns (nil, err) on a read failure, never a partial set.
// A marker with no value, or an empty value, counts as absent.
func ListSkeletonMarkers(c ServerOptionLister) (map[string]struct{}, error) {
	out, err := c.ShowAllServerOptions()
	if err != nil {
		return nil, err
	}
	set := map[string]struct{}{}
	if out == "" {
		return set, nil
	}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			continue
		}
		name := line[:idx]
		if !strings.HasPrefix(name, SkeletonMarkerPrefix) {
			continue
		}
		value := tmuxout.StripMatchedOuterQuotes(strings.TrimSpace(line[idx+1:]))
		if value == "" {
			continue
		}
		paneKey := strings.TrimPrefix(name, SkeletonMarkerPrefix)
		set[paneKey] = struct{}{}
	}
	return set, nil
}

func SetSkeletonMarker(w ServerOptionWriter, paneKey string) error {
	return w.SetServerOption(SkeletonMarkerPrefix+paneKey, "1")
}

func UnsetSkeletonMarker(w ServerOptionWriter, paneKey string) error {
	return w.UnsetServerOption(SkeletonMarkerPrefix + paneKey)
}

func UnsetSkeletonMarkerForFIFO(w ServerOptionWriter, fifoPath string) error {
	return UnsetSkeletonMarker(w, PaneKeyFromFIFOPath(fifoPath))
}

// SetResumePendingMarker marks the pane as waiting on a resume decision. The
// marker lives on the pane itself, so no tmux rearrangement can detach it.
func SetResumePendingMarker[T ~string](w PaneOptionWriter[T], target T) error {
	return w.SetPaneOption(target, ResumePendingOption, "1")
}

// UnsetResumePendingMarker clears the marker. Clearing one that is already
// absent succeeds, so no caller has to read before it clears.
func UnsetResumePendingMarker[T ~string](w PaneOptionWriter[T], target T) error {
	return w.UnsetPaneOption(target, ResumePendingOption)
}

// ResumePendingSet states the presence rule: tmux reports an unset pane option
// and one set to an empty value identically, so both read as absent.
func ResumePendingSet(optionValue string) bool {
	return optionValue != ""
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
