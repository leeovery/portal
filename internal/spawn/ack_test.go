package spawn

import (
	"errors"
	"slices"
	"sort"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// optionDump is a crafted ShowAllServerOptions output mixing two @portal-spawn-
// markers of batch b1, one of batch b2, two @portal-skeleton- markers, and a
// pair of ordinary (non-marker) server options as noise. It is the single
// fixture driving the two-way prefix-isolation proof: the spawn Collect/Clean
// path must see only its batch, and state.ListSkeletonMarkers must see only the
// skeleton paneKeys — over the SAME dump.
const optionDump = "@portal-spawn-b1-t1 1\n" +
	"@portal-spawn-b1-t2 1\n" +
	"@portal-spawn-b2-t9 1\n" +
	"@portal-skeleton-foo 1\n" +
	"@portal-skeleton-bar 1\n" +
	"default-terminal \"tmux-256color\"\n" +
	"\n" +
	"escape-time 10"

// fakeOptionLister is a crafted-string serverOptionLister. It structurally
// satisfies both spawn.serverOptionLister and state.ServerOptionLister, so the
// one fixture drives both enumerators in the isolation proof.
type fakeOptionLister struct {
	out string
	err error
}

func (f fakeOptionLister) ShowAllServerOptions() (string, error) { return f.out, f.err }

// setCall records one SetServerOption invocation.
type setCall struct {
	name  string
	value string
}

// fakeOptionWriter records SetServerOption / UnsetServerOption calls and can
// script per-marker unset failures via unsetErr.
type fakeOptionWriter struct {
	sets     []setCall
	unsets   []string
	unsetErr func(name string) error
}

func (f *fakeOptionWriter) SetServerOption(name, value string) error {
	f.sets = append(f.sets, setCall{name: name, value: value})
	return nil
}

func (f *fakeOptionWriter) UnsetServerOption(name string) error {
	f.unsets = append(f.unsets, name)
	if f.unsetErr != nil {
		return f.unsetErr(name)
	}
	return nil
}

// sortedKeys returns the keys of a set in sorted order for stable comparison.
func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestServerOptionAckChannel_CollectIgnoresForeignBatchesAndSkeletonMarkers(t *testing.T) {
	// "it collects only the target batch's tokens and ignores foreign batches and skeleton markers"
	ch := NewServerOptionAckChannel(&fakeOptionWriter{}, fakeOptionLister{out: optionDump})

	got, err := ch.Collect("b1")
	if err != nil {
		t.Fatalf("Collect(b1) error = %v, want nil", err)
	}
	if want := []string{"t1", "t2"}; !slices.Equal(sortedKeys(got), want) {
		t.Errorf("Collect(b1) tokens = %v, want %v (foreign-batch and skeleton markers must be excluded)", sortedKeys(got), want)
	}

	// A batch with no present markers yields a non-nil empty set (never nil on
	// success) — callers distinguish "no tokens" from "enumeration failed".
	none, err := ch.Collect("nope")
	if err != nil {
		t.Fatalf("Collect(nope) error = %v, want nil", err)
	}
	if none == nil {
		t.Errorf("Collect(nope) = nil map, want non-nil empty set")
	}
	if len(none) != 0 {
		t.Errorf("Collect(nope) = %v, want empty", sortedKeys(none))
	}
}

func TestListSkeletonMarkers_IgnoresSpawnMarkersOnSameDump(t *testing.T) {
	// "it proves ListSkeletonMarkers ignores @portal-spawn markers on the same option dump"
	got, err := state.ListSkeletonMarkers(fakeOptionLister{out: optionDump})
	if err != nil {
		t.Fatalf("ListSkeletonMarkers error = %v, want nil", err)
	}
	if want := []string{"bar", "foo"}; !slices.Equal(sortedKeys(got), want) {
		t.Errorf("ListSkeletonMarkers paneKeys = %v, want %v (must be blind to @portal-spawn- markers)", sortedKeys(got), want)
	}
	// Defensive: none of the spawn tokens/names may leak in as a skeleton paneKey.
	for _, spawnLeak := range []string{"t1", "t2", "t9", "b1-t1", "b1-t2", "b2-t9"} {
		if _, ok := got[spawnLeak]; ok {
			t.Errorf("ListSkeletonMarkers leaked spawn-derived key %q", spawnLeak)
		}
	}
}

func TestServerOptionAckChannel_WriteSetsMarkerToOne(t *testing.T) {
	// "it writes the @portal-spawn-<batch>-<token> marker set to 1"
	w := &fakeOptionWriter{}
	ch := NewServerOptionAckChannel(w, fakeOptionLister{})

	if err := ch.Write("b1", "t1"); err != nil {
		t.Fatalf("Write(b1, t1) error = %v, want nil", err)
	}
	want := []setCall{{name: "@portal-spawn-b1-t1", value: "1"}}
	if !slices.Equal(w.sets, want) {
		t.Errorf("Write recorded sets = %v, want %v", w.sets, want)
	}
}

func TestServerOptionAckChannel_CleanUnsetsOnlyBatchMarkersIdempotently(t *testing.T) {
	// "it cleans every batch marker idempotently and leaves other markers intact"
	w := &fakeOptionWriter{}
	ch := NewServerOptionAckChannel(w, fakeOptionLister{out: optionDump})

	if err := ch.Clean("b1"); err != nil {
		t.Fatalf("Clean(b1) error = %v, want nil", err)
	}
	want := []string{"@portal-spawn-b1-t1", "@portal-spawn-b1-t2"}
	if !slices.Equal(w.unsets, want) {
		t.Errorf("Clean(b1) unset = %v, want %v (must not touch b2 or skeleton markers)", w.unsets, want)
	}

	// A Clean on a batch with zero present markers is a nil-return no-op that
	// unsets nothing (idempotent — already-absent is not an error).
	w2 := &fakeOptionWriter{}
	ch2 := NewServerOptionAckChannel(w2, fakeOptionLister{out: optionDump})
	if err := ch2.Clean("absent"); err != nil {
		t.Fatalf("Clean(absent) error = %v, want nil", err)
	}
	if len(w2.unsets) != 0 {
		t.Errorf("Clean(absent) unset = %v, want none (zero-marker batch is a no-op)", w2.unsets)
	}
}

func TestServerOptionAckChannel_CleanContinuesAfterUnsetErrorReturnsFirst(t *testing.T) {
	// Clean collects per-marker unset errors but continues, returning the FIRST.
	boom := errors.New("unset boom")
	w := &fakeOptionWriter{
		unsetErr: func(name string) error {
			if name == "@portal-spawn-b1-t1" {
				return boom
			}
			return nil
		},
	}
	ch := NewServerOptionAckChannel(w, fakeOptionLister{out: optionDump})

	err := ch.Clean("b1")
	if !errors.Is(err, boom) {
		t.Fatalf("Clean(b1) error = %v, want it to be %v (first unset error)", err, boom)
	}
	// Both markers were still attempted — the loop continued past the failure.
	want := []string{"@portal-spawn-b1-t1", "@portal-spawn-b1-t2"}
	if !slices.Equal(w.unsets, want) {
		t.Errorf("Clean(b1) unset = %v, want %v (must continue past an unset error)", w.unsets, want)
	}
}

func TestServerOptionAckChannel_CollectReturnsErrorNotFalseEmpty(t *testing.T) {
	// "it returns an error (not a false-empty set) when enumeration fails"
	boom := errors.New("show-options boom")
	ch := NewServerOptionAckChannel(&fakeOptionWriter{}, fakeOptionLister{err: boom})

	got, err := ch.Collect("b1")
	if !errors.Is(err, boom) {
		t.Fatalf("Collect error = %v, want it to be %v", err, boom)
	}
	if got != nil {
		t.Errorf("Collect on enumeration failure = %v, want nil (never a false-empty success set)", got)
	}
}
