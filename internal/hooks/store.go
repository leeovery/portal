package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/storelog"
)

// The op verb is both the slog message and a required "op" attr, so the
// `hooks: <verb>` grep idiom and `grep op=set` filtering both work.
var logger = log.For("hooks")

type Hook struct {
	Key     string
	Event   string
	Command string
	Resume  resumemode.Mode
}

// Snapshot is the on-disk shape: map[hook_key]map[event]registration. The clean
// also names its pre-enumeration read of the file by this type — the older view
// a deletion may be narrowed by.
type Snapshot map[string]map[string]Registration

type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// Load reads hooks from the JSON file, returning an empty map when the file is
// missing or holds malformed JSON. via names the calling surface for the
// degradation breadcrumb.
func (s *Store) Load(via Via) (Snapshot, error) {
	return s.loadShared(via)
}

// LoadStrict reads hooks as Load does, but reports a file that does not parse
// as ErrMalformed rather than as empty: a diagnosis that read a corrupt file as
// holding no hooks would call it healthy.
func (s *Store) LoadStrict(via Via) (Snapshot, error) {
	return s.loadSharedBounded(via, lockTimeout, s.load)
}

// loadSnapshot is the clean's advisory pre-read. It reads at the derived
// pre-read bound, so a clean — which takes the sidecar twice, shared here and
// exclusive in deleteStale — never spends two full lockTimeouts waiting on a
// wedged writer. It refuses a file that does not parse, as the clean's load
// under its hold does: read as empty, a corrupt file would pass for one with
// nothing to sweep.
func (s *Store) loadSnapshot() (Snapshot, error) {
	return s.loadSharedBounded(ViaInternal, snapshotLockBound(), s.load)
}

// loadShared reads under the shared hold every ordinary read takes.
func (s *Store) loadShared(via Via) (Snapshot, error) {
	return s.loadSharedBounded(via, lockTimeout, func() (Snapshot, error) { return s.loadDegrading(via) })
}

// loadSharedBounded runs read under a shared hold acquired at bound, releasing
// it before returning so no read ever hands a lock to its caller. Any
// acquisition failure — an absent sidecar, an absent directory, an unreadable
// file, or the bound elapsing — falls through to the read with no hold after
// one DEBUG record: correctness never depends on the shared lock (AtomicWrite
// renames, so a reader sees the pre-state or the post-state, never a torn one),
// and failing a read would forfeit a hook for nothing. The bound is a parameter
// and is never derived from via, which is a log attr.
func (s *Store) loadSharedBounded(via Via, bound time.Duration, read func() (Snapshot, error)) (Snapshot, error) {
	f, err := s.acquireSharedLock(bound)
	if err != nil {
		logger.Debug("load-unlocked", "op", "load-unlocked", "via", via.String(), "error", err)
		return read()
	}
	defer func() { _ = f.Close() }()

	return read()
}

// loadDegrading is the read doors' view of the file: a malformed hooks.json
// reads as empty after one DEBUG record, so a lookup, a listing or a diagnosis
// never fails over content a hand edit can still repair. Only the read doors
// degrade — a mutation takes load directly and refuses what it cannot parse.
func (s *Store) loadDegrading(via Via) (Snapshot, error) {
	h, err := s.load()
	if errors.Is(err, ErrMalformed) {
		logger.Debug("load-malformed", "op", "load-malformed", "via", via.String(), "error", err)
		return Snapshot{}, nil
	}
	return h, err
}

// load is the non-locking read the mutations use from inside their own hold: a
// second acquisition from the same process is not re-entrant and would block
// against that hold until the bound. A file that does not parse is reported as
// ErrMalformed rather than as empty, so no mutation writes a one-entry map over
// the registrations it could not read.
func (s *Store) load() (Snapshot, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, nil
		}
		return nil, err
	}

	var h Snapshot
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}

	return h, nil
}

// save atomically writes hooks to the JSON file, creating the parent directory
// if it does not exist. Non-locking, like load: it is called from inside a hold.
func (s *Store) save(h Snapshot) error {
	data, err := encode(h)
	if err != nil {
		return err
	}
	return s.write(data)
}

// encode is the file save writes for h.
func encode(h Snapshot) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(h); err != nil {
		return nil, fmt.Errorf("failed to marshal hooks: %w", err)
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

func (s *Store) write(data []byte) error {
	return fileutil.AtomicWrite(s.path, data, fileutil.ReportUnsynced(warnUnsynced))
}

// warnUnsynced logs a write that landed though the directory sync after its
// rename failed: the mutation stands, and logs as written.
func warnUnsynced(err error) {
	logger.Warn("unsynced", "op", "unsynced", "error", err, "error_class", fileutil.ClassifyWriteError(err))
}

// acquireMutationLockFor takes the mutation hold for a mutation of keys and,
// when it cannot, writes that mutation's WARN under op for each key. The record
// carries no error_class — nothing reached the write phases it classifies — and
// no value, which names what a write carried.
func (s *Store) acquireMutationLockFor(op string, via Via, keys ...string) (*os.File, error) {
	lock, err := s.acquireMutationLock()
	if err != nil {
		warnEach(op, via, keys, "error", err)
		return nil, err
	}
	return lock, nil
}

// loadForMutation is a mutation's load from inside its hold. A load it cannot
// complete — an unreadable file or one that does not parse — is reported as
// ErrStoreRead and written as the mutation's WARN under op for each of keys,
// with no error_class and no value for the same reasons as a refused acquire.
func (s *Store) loadForMutation(op string, via Via, keys ...string) (Snapshot, error) {
	h, err := s.load()
	if err != nil {
		err = fmt.Errorf("%w: %w", ErrStoreRead, err)
		warnEach(op, via, keys, "error", err)
		return nil, err
	}
	return h, nil
}

// warnEach writes op's WARN for each of keys, carrying attrs.
func warnEach(op string, via Via, keys []string, attrs ...any) {
	for _, key := range keys {
		logger.Warn(op, append([]any{"op", op, "hook_key", key, "via", via.String()}, attrs...)...)
	}
}

// Set adds or overwrites the hook for key and event with the whole of
// registration: nothing the replaced value held survives a rewrite it was not
// given. Writing a registration the stored one already matches on command and
// mode is a no-op: the file is left untouched. via records the mutation origin
// for the audit breadcrumb.
func (s *Store) Set(key string, event Event, registration Registration, via Via) error {
	// Under the method's own op, not classifySet's verdict: that verdict reads
	// the loaded file, and a refused acquire or load is what prevented reading it.
	lock, err := s.acquireMutationLockFor("set", via, key)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()

	h, err := s.loadForMutation("set", via, key)
	if err != nil {
		return err
	}

	op := classifySet(h, key, event, registration)
	if op == "set-noop" {
		logger.Debug("set-noop", "op", "set-noop", "hook_key", key, "via", via.String())
		return nil
	}

	if h[key] == nil {
		h[key] = make(map[string]Registration)
	}
	// Storing the handed value instead reinstates the bytes it was decoded from,
	// so a caller that loaded, adjusted and handed it back writes what it read.
	h[key][event.String()] = Registration{Command: registration.Command, Resume: registration.Resume}

	if err := s.save(h); err != nil {
		logger.Warn(op, "op", op, "hook_key", key, "value", registration.Command, "via", via.String(),
			"error", err, "error_class", fileutil.ClassifyWriteError(err))
		return err
	}

	logger.Info(op, "op", op, "hook_key", key, "value", registration.Command, "via", via.String())
	return nil
}

func classifySet(h Snapshot, key string, event Event, registration Registration) string {
	events, ok := h[key]
	if !ok {
		return "set"
	}
	existing, ok := events[event.String()]
	if !ok {
		return "set"
	}
	if sameRegistration(existing, registration) {
		return "set-noop"
	}
	return "modify"
}

// Remove deletes the hook for key and event, dropping the outer key when its last
// event goes, and reports whether it removed anything. A call that removes
// nothing — an empty key, an absent key, an absent event, an absent file —
// writes no file and emits no breadcrumb, and a failed save reports no removal.
// The answer comes from the map this call loaded and mutated, never from a
// separate read.
func (s *Store) Remove(key string, event Event, via Via) (bool, error) {
	removed, err := s.removeEntries([]string{key}, event, via, removal{op: "rm"})
	return len(removed) == 1, err
}

// Discard is the removal route for a caller that judged a command before
// removing it: the resume panel, and forgetting a dropped session. It removes
// the entry for key and event only when its command is exactly shown, the
// command the panel put in front of the user or the forget read; an entry
// holding any other command, whatever its resume mode, is nothing to remove
// and answers as an absent one does. Otherwise it
// keeps Remove's contract, but its breadcrumb is filed under op=discard and
// carries the removed command as value, so the destroyed registration can be
// copied back out of the log.
func (s *Store) Discard(key string, event Event, shown string, via Via) (bool, error) {
	removed, err := s.DiscardEach(event, map[string]string{key: shown}, via)
	return len(removed) == 1, err
}

// DiscardEach is Discard for every key of shown, each removed only while its
// command is exactly the one shown names for it, under one hold of the lock:
// a caller holding another lock as it removes them waits on this one once,
// however many there are. It reports the keys it removed, in order. A refused
// acquire, a load or a save that fails removes none, and writes each key's
// WARN.
func (s *Store) DiscardEach(event Event, shown map[string]string, via Via) ([]string, error) {
	return s.removeEntries(slices.Sorted(maps.Keys(shown)), event, via, removal{
		op:         "discard",
		carryValue: true,
		matches:    func(key string, r Registration) bool { return r.Command == shown[key] },
	})
}

// removal is how one route removes an entry: the op its breadcrumbs carry,
// whether they carry the removed command, and which stored registrations it may
// remove at all — any, when matches is nil.
type removal struct {
	op         string
	carryValue bool
	matches    func(key string, r Registration) bool
}

// removeEntries removes the entry for event of each of keys that r allows,
// under one hold, and reports the keys it removed.
func (s *Store) removeEntries(keys []string, event Event, via Via, r removal) ([]string, error) {
	keys = slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return key == "" })
	if len(keys) == 0 {
		return nil, nil
	}

	// A refused acquire or load is a failed operation, not the silent no-removal
	// below: that one changed nothing because there was nothing to change, and
	// this one could not look.
	lock, err := s.acquireMutationLockFor(r.op, via, keys...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = lock.Close() }()

	h, err := s.loadForMutation(r.op, via, keys...)
	if err != nil {
		return nil, err
	}

	var removed []string
	commands := map[string]string{}
	for _, key := range keys {
		events := h[key]
		registration, ok := events[event.String()]
		if !ok || (r.matches != nil && !r.matches(key, registration)) {
			continue
		}
		delete(events, event.String())
		if len(events) == 0 {
			delete(h, key)
		}
		removed = append(removed, key)
		commands[key] = registration.Command
	}
	if len(removed) == 0 {
		return nil, nil
	}

	if err := s.save(h); err != nil {
		warnEach(r.op, via, removed, "error", err, "error_class", fileutil.ClassifyWriteError(err))
		return nil, err
	}

	for _, key := range removed {
		attrs := []any{"op", r.op, "hook_key", key, "via", via.String()}
		if r.carryValue {
			attrs = append(attrs, "value", commands[key])
		}
		logger.Info(r.op, attrs...)
	}
	return removed, nil
}

// List returns the hooks sorted by key then event.
func (s *Store) List(via Via) ([]Hook, error) {
	h, err := s.loadShared(via)
	if err != nil {
		return nil, err
	}

	var list []Hook
	for key, events := range h {
		for event, registration := range events {
			list = append(list, Hook{
				Key:     key,
				Event:   event,
				Command: registration.Command,
				Resume:  registration.Resume,
			})
		}
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Key != list[j].Key {
			return list[i].Key < list[j].Key
		}
		return list[i].Event < list[j].Event
	})

	return list, nil
}

// StaleKeys applies the staleness rule; every reader of staleness must route
// through here rather than restate it. A persisted key is stale iff it names
// neither a session in listed, the user sessions the backend lists (answering
// or not), nor an entry in indexed, the names sessions.json holds. Every key
// is judged, whatever its shape. The stale keys come back sorted.
//
// It carries no mass-deletion guard: an empty listing beside an empty index
// makes every persisted key stale, and deferring on a read that may not be
// one is the caller's repair-safety policy.
func StaleKeys(persisted Snapshot, listed, indexed []string) []string {
	protected := make(map[string]struct{}, len(listed)+len(indexed))
	for _, name := range slices.Concat(listed, indexed) {
		protected[name] = struct{}{}
	}
	var stale []string
	for key := range persisted {
		if _, ok := protected[key]; !ok {
			stale = append(stale, key)
		}
	}
	slices.Sort(stale)
	return stale
}

// narrowToSnapshot drops every candidate the snapshot does not hold.
func narrowToSnapshot(candidates []string, snapshot Snapshot) []string {
	var narrowed []string
	for _, key := range candidates {
		if _, ok := snapshot[key]; ok {
			narrowed = append(narrowed, key)
		}
	}
	return narrowed
}

// ErrStoreRead reports a read of hooks.json that a mutation or a clean could
// not complete, so nothing was judged or written on its strength. A failed
// write never carries it.
var ErrStoreRead = errors.New("failed to read hooks store")

// ErrMalformed reports a hooks.json that exists but does not parse. A read
// degrades it to an empty map — a resume must not forfeit a hook over a file
// that can still be repaired by hand — while a mutation, a clean and LoadStrict
// refuse it: a map loaded from nothing and written back is every other entry
// gone, and one judged as empty passes for healthy.
var ErrMalformed = errors.New("hooks.json holds malformed JSON")

// CleanStale removes and returns the hook entries the staleness rule judges
// stale against listSessions' and readIndex's answers, and that the snapshot
// this call read holds. A clean that removes nothing writes no file and emits
// no summary.
//
// The reads a safe deletion needs are sequenced here rather than by the
// caller: the snapshot first, then listSessions with no lock held, then
// readIndex under the exclusive hold. Taking the snapshot first is what keeps
// a registration for a new key out of the delete set: it is the older view,
// and deleteStale narrows to it. Running listSessions under the hold would
// park every writer behind it. A session name recurs, so a registration can
// land under a key the snapshot already holds as stale; `hook set` claims its
// session's index entry before it writes, so the index read under the hold
// protects every registration the file it judges holds.
//
// listSessions is handed that snapshot and returns the names the backend
// lists; readIndex returns the names sessions.json holds. An error either one
// returns aborts the clean with the file untouched and is returned unwrapped,
// so a caller can carry its own reasons through. A snapshot read that fails,
// or finds a file that does not parse, returns ErrStoreRead and calls neither.
func (s *Store) CleanStale(listSessions func(Snapshot) ([]string, error), readIndex func() ([]string, error)) ([]string, error) {
	snapshot, err := s.loadSnapshot()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStoreRead, err)
	}

	listed, err := listSessions(snapshot)
	if err != nil {
		return nil, err
	}

	return s.deleteStale(listed, readIndex, snapshot)
}

// deleteStale is the clean's mutation: it derives the delete set from the file
// it loads under its own exclusive hold, so a deletion is decided on the file as
// it stands rather than on the older view the snapshot describes. The snapshot
// may only narrow that set, never widen it: a key it does not hold was written
// after the listing and so was never offered to it for protection.
func (s *Store) deleteStale(listed []string, readIndex func() ([]string, error), snapshot Snapshot) ([]string, error) {
	start := time.Now()

	lock, err := s.acquireMutationLock()
	if err != nil {
		return nil, err
	}
	defer func() { _ = lock.Close() }()

	h, err := s.load()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrStoreRead, err)
	}

	indexed, err := readIndex()
	if err != nil {
		return nil, err
	}

	removed := narrowToSnapshot(StaleKeys(h, listed, indexed), snapshot)

	if len(removed) == 0 {
		return removed, nil
	}

	kept := maps.Clone(h)
	for _, key := range removed {
		delete(kept, key)
	}

	if err := s.save(kept); err != nil {
		storelog.EmitCleanStaleSummary(logger, len(removed), start, err)
		return nil, fmt.Errorf("failed to save after cleaning stale hooks: %w", err)
	}

	// After the write, so a line exists only for a key the file no longer holds.
	// The commands come from h, the pre-delete map.
	for _, key := range removed {
		logger.Info("clean-stale", "op", "clean-stale", "hook_key", key,
			"value", removedValue(h[key]), "via", ViaInternal.String())
	}

	storelog.EmitCleanStaleSummary(logger, len(removed), start, nil)

	return removed, nil
}

// removedValue renders a reaped key's value attr from the events that key
// actually held, so the breadcrumb reports the commands the deletion destroyed
// whatever they were filed under. A key holding one event renders its command
// alone — the recoverable form an operator copies back out of the log — and a
// key holding several renders every event=command pair in event order, since
// naming one of them would misreport the rest.
func removedValue(events map[string]Registration) string {
	if len(events) == 1 {
		for _, registration := range events {
			return registration.Command
		}
	}

	names := make([]string, 0, len(events))
	for event := range events {
		names = append(names, event)
	}
	sort.Strings(names)

	pairs := make([]string, 0, len(names))
	for _, event := range names {
		pairs = append(pairs, event+"="+events[event].Command)
	}
	return strings.Join(pairs, "; ")
}
