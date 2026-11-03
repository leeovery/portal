package spawn

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type AckCollector interface {
	Collect(batch string) (map[string]struct{}, error)
}

type AckCleaner interface {
	Clean(batch string) error
}

// AckWriter writes a spawned window's own ack once its attach has begun, or
// just before it execs the attach in its place.
type AckWriter interface {
	Write(batch, token string) error
}

// AckChannelFull omits Write deliberately: the burster never writes acks,
// the spawned windows do.
type AckChannelFull interface {
	AckCollector
	AckCleaner
}

// ackDir is where the acks live in the state directory.
const ackDir = "spawn-acks"

// staleAckAge is how old an ack is before Clean takes it for one a burst
// left behind. A burst cleans its own within the ack timeout of its last
// window, so no live burst's ack is ever this old.
const staleAckAge = time.Hour

// FileAckChannel is the token-ack channel as files in the state directory's
// spawn-acks/, one empty file named <batch>-<token> per window acked, only
// its owner able to read it. Presence is the signal.
type FileAckChannel struct {
	dir string
}

func NewFileAckChannel(stateDir string) *FileAckChannel {
	return &FileAckChannel{dir: filepath.Join(stateDir, ackDir)}
}

var (
	_ AckWriter      = (*FileAckChannel)(nil)
	_ AckChannelFull = (*FileAckChannel)(nil)
)

// Dir is the directory holding the acks.
func (c *FileAckChannel) Dir() string { return c.dir }

// Write is idempotent. It refuses a batch or token no burst could have
// minted before either reaches a path.
func (c *FileAckChannel) Write(batch, token string) error {
	name, err := ackFileName(batch, token)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0o700); err != nil {
		return fmt.Errorf("spawn: write ack: %w", err)
	}
	// MkdirAll leaves a directory that already exists as it is.
	if err := os.Chmod(c.dir, 0o700); err != nil {
		return fmt.Errorf("spawn: write ack: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(c.dir, name), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("spawn: write ack: %w", err)
	}
	return f.Close()
}

// Collect returns the tokens acked for batch. A failed read returns (nil, err)
// rather than an empty-as-success set, which would mis-classify every window
// as failed; on success the map is non-nil, and a directory never made holds
// no acks.
func (c *FileAckChannel) Collect(batch string) (map[string]struct{}, error) {
	acks, err := c.list()
	if err != nil {
		return nil, err
	}
	tokens := map[string]struct{}{}
	for _, a := range acks {
		if a.batch == batch {
			tokens[a.token] = struct{}{}
		}
	}
	return tokens, nil
}

// Clean removes batch's acks, and any other burst's old enough to be stale,
// as one that ended before cleaning leaves them. It is idempotent and
// tolerates a concurrent removal. A failed removal does not stop the rest;
// the first error is returned.
func (c *FileAckChannel) Clean(batch string) error {
	acks, err := c.list()
	if err != nil {
		return err
	}
	var firstErr error
	staleBefore := time.Now().Add(-staleAckAge)
	for _, a := range acks {
		stale := a.batch != batch && a.modified.Before(staleBefore)
		if a.batch != batch && !stale {
			continue
		}
		path := filepath.Join(c.dir, a.name)
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			firstErr = cmpErr(firstErr, err)
			continue
		}
		if stale {
			spawnLogger.Debug("stale spawn ack removed", "path", path)
		}
	}
	return firstErr
}

// ack is one ack file.
type ack struct {
	name, batch, token string
	modified           time.Time
}

// list reads the acks in the directory, skipping any other file and one
// removed as it is read.
func (c *FileAckChannel) list() ([]ack, error) {
	entries, err := os.ReadDir(c.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("spawn: read acks: %w", err)
	}
	var acks []ack
	for _, e := range entries {
		batch, token, ok := parseAckFileName(e.Name())
		if !ok || !e.Type().IsRegular() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		acks = append(acks, ack{name: e.Name(), batch: batch, token: token, modified: info.ModTime()})
	}
	return acks, nil
}

func cmpErr(first, next error) error {
	if first != nil {
		return first
	}
	return next
}
