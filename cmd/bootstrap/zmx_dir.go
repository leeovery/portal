package bootstrap

import (
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/state"
)

// OtherZmxDirMessage is the fatal of a Portal run against another zmx
// directory than the one its state directory belongs to: the state
// directory, the zmx directory it belongs to, and the one this Portal uses.
const OtherZmxDirMessage = "Portal's state in %[1]s belongs to the zmx sessions in %[2]s, but this Portal is using %[3]s. Run it with ZMX_DIR=%[2]s, or give it a state of its own with PORTAL_STATE_DIR."

// ZmxDirClaimer claims the state directory for the zmx directory Portal runs
// against. Its error is an *OtherZmxDirError for a state directory that
// belongs to another.
type ZmxDirClaimer interface {
	Claim() error
}

// OtherZmxDirError is a state directory that belongs to another zmx directory
// than the one this Portal runs against.
type OtherZmxDirError struct {
	StateDir string
	// Recorded is the zmx directory the state directory belongs to, as
	// recorded, canonical.
	Recorded string
	// Using is the zmx directory this Portal runs against, as it was given.
	Using string
}

func (e *OtherZmxDirError) Error() string {
	return fmt.Sprintf("the state directory %s belongs to the zmx directory %s, not %s", e.StateDir, e.Recorded, e.Using)
}

// ZmxDirOwner is a state directory and the zmx directory a Portal runs
// against. A state directory belongs to one zmx directory, which the first
// bootstrap that finds none records in it: run against another, the
// bootstrap would mark every saved session not there owed and restore it
// into that directory, duplicating it, and the first directory's saver would
// later replace those entries and forget what they kept.
type ZmxDirOwner struct {
	StateDir string
	// ZmxDir is the zmx directory this Portal runs against.
	ZmxDir string
	// Canonical is a directory as the backend compares them, two spellings of
	// one directory alike.
	Canonical func(dir string) string
}

// Check reports nil when the state directory belongs to ZmxDir or records no
// zmx directory, an *OtherZmxDirError when it belongs to another, and the
// read's error when its record can't be read. It writes nothing.
func (o ZmxDirOwner) Check() error {
	_, err := o.Recorded()
	return err
}

// Claim is Check, recording ZmxDir, canonical, when the state directory
// records no zmx directory.
func (o ZmxDirOwner) Claim() error {
	recorded, err := o.Recorded()
	if err != nil || recorded != "" {
		return err
	}
	return state.RecordZmxDir(o.StateDir, o.Canonical(o.ZmxDir))
}

// Recorded is the zmx directory the state directory records, empty when it
// records none, failing as Check does. It writes nothing.
func (o ZmxDirOwner) Recorded() (string, error) {
	recorded, err := state.ReadZmxDir(o.StateDir)
	if err != nil || recorded == "" {
		return "", err
	}
	if o.Canonical(recorded) != o.Canonical(o.ZmxDir) {
		return recorded, &OtherZmxDirError{StateDir: o.StateDir, Recorded: recorded, Using: o.ZmxDir}
	}
	return recorded, nil
}

// zmxDirFatal is the fatal of a state directory Portal can't claim.
func zmxDirFatal(err error) *FatalError {
	if other, ok := errors.AsType[*OtherZmxDirError](err); ok {
		return NewFatal(fmt.Sprintf(OtherZmxDirMessage, other.StateDir, other.Recorded, other.Using), err)
	}
	return NewFatal("Portal failed to record the zmx directory its state belongs to: "+err.Error(), err)
}
