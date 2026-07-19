package spawn

import "strings"

// serverOptionWriter is the write half of the spawn ack channel's tmux
// server-option seam: it sets and unsets named server options. It is a
// spawn-local interface (mirroring the Phase 1 clientLister idiom) so
// internal/spawn need not import internal/state to reach these operations;
// *tmux.Client satisfies it implicitly via SetServerOption / UnsetServerOption.
type serverOptionWriter interface {
	SetServerOption(name, value string) error
	UnsetServerOption(name string) error
}

// serverOptionLister is the read half of the seam: a single call dumps every
// server option so Collect / Clean enumerate the whole @portal-spawn- namespace
// in one tmux invocation. *tmux.Client satisfies it implicitly via
// ShowAllServerOptions.
type serverOptionLister interface {
	ShowAllServerOptions() (string, error)
}

// AckCollector is the read-side consumer seam: it yields the confirmed token
// set for a batch. The burst orchestrator polls it to decide which windows came
// up.
type AckCollector interface {
	Collect(batch string) (map[string]struct{}, error)
}

// AckCleaner is the cleanup-side consumer seam: it unsets a batch's markers.
// Callers treat it as best-effort — bounded, harmless leaks self-expire with
// the tmux server.
type AckCleaner interface {
	Clean(batch string) error
}

// AckWriter is the write-side consumer seam: a spawned window writes its own
// token marker just before it execs into tmux (see the open command's hidden
// --ack flag / writeAckMarker in cmd/open.go).
type AckWriter interface {
	Write(batch, token string) error
}

// AckChannelFull is the combined Collect+Clean seam the burst orchestrators
// depend on (OpenBurstDeps.Ack and tui.Deps.AckChannel both reference it). It is
// deliberately narrower than the full ServerOptionAckChannel — the burster
// never writes markers itself (the spawned windows do).
type AckChannelFull interface {
	AckCollector
	AckCleaner
}

// ServerOptionAckChannel implements the @portal-spawn- token-ack contract over
// tmux server options: Write sets a window's marker, Collect enumerates the
// markers of a batch, and Clean unsets them. Markers are presence-only signals
// (the value is opaque "1"); a distinct @portal-spawn- prefix keeps this
// namespace mutually invisible to state.ListSkeletonMarkers (the only other
// all-server-options enumerator), so sweeps in either direction cannot collide.
type ServerOptionAckChannel struct {
	w serverOptionWriter
	l serverOptionLister
}

// NewServerOptionAckChannel wires an ack channel to a writer and a lister. In
// production both are the same *tmux.Client.
func NewServerOptionAckChannel(w serverOptionWriter, l serverOptionLister) *ServerOptionAckChannel {
	return &ServerOptionAckChannel{w: w, l: l}
}

// Compile-time guards: *ServerOptionAckChannel satisfies every consumer seam.
var (
	_ AckWriter      = (*ServerOptionAckChannel)(nil)
	_ AckCollector   = (*ServerOptionAckChannel)(nil)
	_ AckCleaner     = (*ServerOptionAckChannel)(nil)
	_ AckChannelFull = (*ServerOptionAckChannel)(nil)
)

// Write sets the @portal-spawn-<batch>-<token> server option to "1". The value
// is opaque — presence is the signal — so a stale re-write is harmless.
func (c *ServerOptionAckChannel) Write(batch, token string) error {
	return c.w.SetServerOption(SpawnMarkerName(batch, token), "1")
}

// Collect enumerates all server options once and returns the set of tokens
// whose @portal-spawn-<batch>-<token> marker belongs to batch. Foreign-batch
// spawn markers and every @portal-skeleton- (or other) option are silently
// skipped (they fail ParseSpawnMarkerName).
//
// On a ShowAllServerOptions failure it returns (nil, err) — never a partial or
// empty-as-success set, which would silently mis-classify every window as
// failed. On success it always returns a non-nil map (empty when the batch has
// no markers).
func (c *ServerOptionAckChannel) Collect(batch string) (map[string]struct{}, error) {
	out, err := c.l.ShowAllServerOptions()
	if err != nil {
		return nil, err
	}
	tokens := map[string]struct{}{}
	forEachBatchMarker(out, batch, func(_, token string) {
		tokens[token] = struct{}{}
	})
	return tokens, nil
}

// Clean enumerates all server options once and unsets every marker belonging to
// batch. It is idempotent by construction (only present markers are
// enumerated) and robust to a concurrent unset because tmux set-option -su on
// an absent user-option exits 0. Per-marker unset errors do not abort the
// sweep: it attempts every marker and returns the first non-nil unset error (or
// the enumeration error). A Clean on a batch with zero markers returns nil.
func (c *ServerOptionAckChannel) Clean(batch string) error {
	out, err := c.l.ShowAllServerOptions()
	if err != nil {
		return err
	}
	var firstErr error
	forEachBatchMarker(out, batch, func(name, _ string) {
		if uerr := c.w.UnsetServerOption(name); uerr != nil && firstErr == nil {
			firstErr = uerr
		}
	})
	return firstErr
}

// forEachBatchMarker invokes fn(name, token) for each @portal-spawn- marker in a
// ShowAllServerOptions dump whose parsed batch equals batch. It is the single
// parse chokepoint shared by Collect and Clean.
func forEachBatchMarker(out, batch string, fn func(name, token string)) {
	for _, name := range optionNames(out) {
		b, token, ok := ParseSpawnMarkerName(name)
		if !ok || b != batch {
			continue
		}
		fn(name, token)
	}
}

// optionNames extracts each server-option name from a ShowAllServerOptions
// dump, mirroring state.ListSkeletonMarkers' parse shape: one option per
// non-empty line, the name being the text before the first space or tab. Lines
// with no value separator are skipped.
func optionNames(out string) []string {
	if out == "" {
		return nil
	}
	var names []string
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			continue
		}
		names = append(names, line[:idx])
	}
	return names
}
