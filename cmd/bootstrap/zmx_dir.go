package bootstrap

import (
	"errors"
	"fmt"

	"github.com/leeovery/portal/internal/state"
)

// The fatals of a state directory Portal can't claim for its zmx directory.
const (
	// OtherZmxDirMessage is a Portal run against another zmx directory than
	// the one its state directory belongs to: the state directory, the zmx
	// directory it belongs to, and the one this Portal uses. It names no
	// deletion: a record of another directory means sessions live there.
	OtherZmxDirMessage = "Portal's state in %[1]s belongs to the zmx sessions in %[2]s, but this Portal is using %[3]s. Run it with ZMX_DIR=%[2]s, or give it a state of its own with PORTAL_STATE_DIR."
	// ZmxDirUnreadableMessage is a record Portal can't read: its path, and why.
	ZmxDirUnreadableMessage = "Portal can't read %[1]s, which records the zmx directory its state belongs to: %[2]v. Delete it, and Portal records the directory in use at its next start."
	// ZmxDirUnwritableMessage is a record Portal can't write: its path, and
	// why.
	ZmxDirUnwritableMessage = "Portal failed to record the zmx directory its state belongs to in %[1]s: %[2]v"
)

// ErrZmxDirUnrecorded is a state directory that records no zmx directory
// yet: the next bootstrap records the one in use.
var ErrZmxDirUnrecorded = errors.New("the state directory records no zmx directory")

// ZmxDirClaimer claims the state directory for the zmx directory Portal runs
// against. Its error is an *OtherZmxDirError for a state directory that
// belongs to another, and a *ZmxDirRecordError for a record it can't read or
// write.
type ZmxDirClaimer interface {
	Claim() error
}

// OtherZmxDirError is a state directory that belongs to another zmx directory
// than the one this Portal runs against.
type OtherZmxDirError struct {
	StateDir string
	// Record is the file that records it.
	Record string
	// Recorded is the zmx directory the state directory belongs to, as
	// recorded, canonical.
	Recorded string
	// Using is the zmx directory this Portal runs against, as it was given.
	Using string
}

func (e *OtherZmxDirError) Error() string {
	return fmt.Sprintf("the state directory %s belongs to the zmx directory %s, not %s", e.StateDir, e.Recorded, e.Using)
}

// ZmxDirRecordError is a zmx directory record Portal can't read, or, with
// Write, can't write.
type ZmxDirRecordError struct {
	Path  string
	Write bool
	Err   error
}

func (e *ZmxDirRecordError) Error() string {
	if e.Write {
		return fmt.Sprintf("write %s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("read %s: %v", e.Path, e.Err)
}

func (e *ZmxDirRecordError) Unwrap() error { return e.Err }

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
// zmx directory, an *OtherZmxDirError when it belongs to another, and a
// *ZmxDirRecordError when its record can't be read. It writes nothing.
func (o ZmxDirOwner) Check() error {
	_, err := o.Recorded()
	return err
}

// Claimed is Check, failing besides with ErrZmxDirUnrecorded when the state
// directory records no zmx directory: the gate's read, which owes the
// bootstrap that records one. It writes nothing.
func (o ZmxDirOwner) Claimed() error {
	recorded, err := o.Recorded()
	if err == nil && recorded == "" {
		return ErrZmxDirUnrecorded
	}
	return err
}

// Refusal is Check's failure as the fatal a bootstrap that can't claim the
// state directory ends on, nil when Check passes. It writes nothing.
func (o ZmxDirOwner) Refusal() error {
	if err := o.Check(); err != nil {
		return zmxDirFatal(err)
	}
	return nil
}

// Claim is Check, recording ZmxDir, canonical, when the state directory
// records no zmx directory.
func (o ZmxDirOwner) Claim() error {
	recorded, err := o.Recorded()
	if err != nil || recorded != "" {
		return err
	}
	if err := state.RecordZmxDir(o.StateDir, o.Canonical(o.ZmxDir)); err != nil {
		return &ZmxDirRecordError{Path: state.ZmxDirRecordPath(o.StateDir), Write: true, Err: err}
	}
	return nil
}

// Recorded is the zmx directory the state directory records, empty when it
// records none, failing as Check does. It writes nothing.
func (o ZmxDirOwner) Recorded() (string, error) {
	path := state.ZmxDirRecordPath(o.StateDir)
	recorded, err := state.ReadZmxDir(o.StateDir)
	switch {
	case err != nil:
		return "", &ZmxDirRecordError{Path: path, Err: err}
	case recorded == "":
		return "", nil
	case o.Canonical(recorded) != o.Canonical(o.ZmxDir):
		return recorded, &OtherZmxDirError{StateDir: o.StateDir, Record: path, Recorded: recorded, Using: o.ZmxDir}
	}
	return recorded, nil
}

// zmxDirFatal is the fatal of a state directory Portal can't claim. An error
// of neither kind ZmxDirClaimer names is worded by itself.
func zmxDirFatal(err error) *FatalError {
	other, isOther := errors.AsType[*OtherZmxDirError](err)
	record, isRecord := errors.AsType[*ZmxDirRecordError](err)
	switch {
	case isOther:
		return NewFatal(fmt.Sprintf(OtherZmxDirMessage, other.StateDir, other.Recorded, other.Using), err)
	case isRecord && record.Write:
		return NewFatal(fmt.Sprintf(ZmxDirUnwritableMessage, record.Path, record.Err), err)
	case isRecord:
		return NewFatal(fmt.Sprintf(ZmxDirUnreadableMessage, record.Path, record.Err), err)
	}
	return NewFatal(err.Error(), err)
}
