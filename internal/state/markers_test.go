package state_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// listerMock satisfies state.ServerOptionLister with a canned output/err.
type listerMock struct {
	out string
	err error
}

func (m *listerMock) ShowAllServerOptions() (string, error) {
	return m.out, m.err
}

// checkerMock satisfies state.RestoringChecker. The found flag and value are
// returned verbatim so individual tests can exercise the absent vs. set vs.
// empty-value paths independently. gotName records the last option name read
// so tests can assert exactly which server option was queried.
type checkerMock struct {
	val     string
	found   bool
	err     error
	gotName string
}

func (m *checkerMock) TryGetServerOption(name string) (string, bool, error) {
	m.gotName = name
	return m.val, m.found, m.err
}

// writerMock satisfies state.ServerOptionWriter. It records every Set/Unset
// call so tests can assert exact option names and values.
type writerMock struct {
	setCalls   []writerSetCall
	unsetCalls []string
	setErr     error
	unsetErr   error
}

type writerSetCall struct {
	name  string
	value string
}

func (m *writerMock) SetServerOption(name, value string) error {
	m.setCalls = append(m.setCalls, writerSetCall{name: name, value: value})
	return m.setErr
}

func (m *writerMock) UnsetServerOption(name string) error {
	m.unsetCalls = append(m.unsetCalls, name)
	return m.unsetErr
}

func TestListSkeletonMarkers(t *testing.T) {
	t.Run("returns an empty set for empty show-options output", func(t *testing.T) {
		got, err := state.ListSkeletonMarkers(&listerMock{out: ""})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got == nil {
			t.Fatal("got nil set; want non-nil empty set")
		}
		if len(got) != 0 {
			t.Errorf("got %d entries, want 0", len(got))
		}
	})

	t.Run("ignores unrelated @ options", func(t *testing.T) {
		out := "@portal-active-some-pane \"1\"\n@some-other-option \"value\""
		got, err := state.ListSkeletonMarkers(&listerMock{out: out})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d entries, want 0; entries = %v", len(got), got)
		}
	})

	t.Run("extracts paneKeys from skeleton marker entries", func(t *testing.T) {
		out := "@portal-skeleton-foo__0.0 \"1\""
		got, err := state.ListSkeletonMarkers(&listerMock{out: out})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d entries, want 1", len(got))
		}
		if _, ok := got["foo__0.0"]; !ok {
			t.Errorf("missing paneKey foo__0.0; got %v", got)
		}
	})

	t.Run("treats any non-empty marker value as present", func(t *testing.T) {
		out := "@portal-skeleton-a__0.0 \"1\"\n@portal-skeleton-b__0.0 \"yes\"\n@portal-skeleton-c__0.0 \"anything\""
		got, err := state.ListSkeletonMarkers(&listerMock{out: out})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for _, key := range []string{"a__0.0", "b__0.0", "c__0.0"} {
			if _, ok := got[key]; !ok {
				t.Errorf("missing paneKey %q; got %v", key, got)
			}
		}
	})

	t.Run("returns multiple paneKeys when multiple skeleton markers are set", func(t *testing.T) {
		out := "@portal-skeleton-foo__0.0 \"1\"\n@portal-skeleton-bar__1.2 \"1\"\n@portal-skeleton-baz__2.5 \"1\""
		got, err := state.ListSkeletonMarkers(&listerMock{out: out})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d entries, want 3; entries = %v", len(got), got)
		}
		for _, key := range []string{"foo__0.0", "bar__1.2", "baz__2.5"} {
			if _, ok := got[key]; !ok {
				t.Errorf("missing paneKey %q; got %v", key, got)
			}
		}
	})

	t.Run("parses lines with quoted values correctly", func(t *testing.T) {
		// Quoted form is the default tmux output. Verify the surrounding
		// double-quotes are stripped.
		out := "@portal-skeleton-foo__0.0 \"1\""
		got, err := state.ListSkeletonMarkers(&listerMock{out: out})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got["foo__0.0"]; !ok {
			t.Errorf("expected paneKey foo__0.0; got %v", got)
		}
	})

	t.Run("parses lines with unquoted values correctly", func(t *testing.T) {
		out := "@portal-skeleton-foo__0.0 1"
		got, err := state.ListSkeletonMarkers(&listerMock{out: out})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got["foo__0.0"]; !ok {
			t.Errorf("expected paneKey foo__0.0; got %v", got)
		}
	})

	t.Run("skips entries with empty values", func(t *testing.T) {
		out := "@portal-skeleton-foo__0.0 \"\"\n@portal-skeleton-bar__0.0 \"1\""
		got, err := state.ListSkeletonMarkers(&listerMock{out: out})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got["foo__0.0"]; ok {
			t.Errorf("foo__0.0 should be absent (empty value)")
		}
		if _, ok := got["bar__0.0"]; !ok {
			t.Errorf("bar__0.0 should be present")
		}
	})

	t.Run("propagates ShowAllServerOptions error", func(t *testing.T) {
		got, err := state.ListSkeletonMarkers(&listerMock{err: errors.New("tmux exploded")})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if got != nil {
			t.Errorf("expected nil set on error; got %v", got)
		}
	})
}

func TestIsRestoringSet(t *testing.T) {
	t.Run("returns false when @portal-restoring is absent", func(t *testing.T) {
		got, err := state.IsRestoringSet(&checkerMock{found: false})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Errorf("got true; want false (option not found)")
		}
	})

	t.Run("returns true when @portal-restoring is set to non-empty value", func(t *testing.T) {
		got, err := state.IsRestoringSet(&checkerMock{val: "1", found: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Errorf("got false; want true")
		}
	})

	t.Run("returns false when @portal-restoring has an empty value", func(t *testing.T) {
		got, err := state.IsRestoringSet(&checkerMock{val: "", found: true})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Errorf("got true; want false (empty value)")
		}
	})

	t.Run("propagates underlying tmux error", func(t *testing.T) {
		_, err := state.IsRestoringSet(&checkerMock{err: errors.New("tmux exploded")})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestBootstrappedLatchSatisfied(t *testing.T) {
	t.Run("it returns true when latch present and version matches", func(t *testing.T) {
		got := state.BootstrappedLatchSatisfied(&checkerMock{val: "1.2.3", found: true}, "1.2.3")
		if !got {
			t.Errorf("got false; want true (present + version match)")
		}
	})

	t.Run("it returns false when latch absent", func(t *testing.T) {
		got := state.BootstrappedLatchSatisfied(&checkerMock{found: false}, "1.2.3")
		if got {
			t.Errorf("got true; want false (latch absent)")
		}
	})

	t.Run("it returns false when stored version mismatches running version", func(t *testing.T) {
		got := state.BootstrappedLatchSatisfied(&checkerMock{val: "1.2.2", found: true}, "1.2.3")
		if got {
			t.Errorf("got true; want false (version mismatch)")
		}
	})

	t.Run("it returns false on read error / down server", func(t *testing.T) {
		got := state.BootstrappedLatchSatisfied(&checkerMock{err: errors.New("tmux exploded")}, "1.2.3")
		if got {
			t.Errorf("got true; want false (read error / down server)")
		}
	})

	t.Run("it returns false when stored value is empty and running version is non-empty", func(t *testing.T) {
		got := state.BootstrappedLatchSatisfied(&checkerMock{val: "", found: true}, "1.2.3")
		if got {
			t.Errorf("got true; want false (empty stored value)")
		}
	})

	t.Run("it reads exactly the @portal-bootstrapped option name", func(t *testing.T) {
		if state.BootstrappedMarkerName != "@portal-bootstrapped" {
			t.Errorf("BootstrappedMarkerName = %q, want %q", state.BootstrappedMarkerName, "@portal-bootstrapped")
		}
		c := &checkerMock{val: "1.2.3", found: true}
		state.BootstrappedLatchSatisfied(c, "1.2.3")
		if c.gotName != state.BootstrappedMarkerName {
			t.Errorf("read option name %q, want %q", c.gotName, state.BootstrappedMarkerName)
		}
	})
}

func TestSetSkeletonMarker(t *testing.T) {
	t.Run("sets server option named SkeletonMarkerPrefix+paneKey to \"1\"", func(t *testing.T) {
		w := &writerMock{}
		if err := state.SetSkeletonMarker(w, "foo__0.0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(w.setCalls) != 1 {
			t.Fatalf("got %d SetServerOption calls, want 1", len(w.setCalls))
		}
		wantName := state.SkeletonMarkerPrefix + "foo__0.0"
		if w.setCalls[0].name != wantName {
			t.Errorf("got option name %q, want %q", w.setCalls[0].name, wantName)
		}
		if w.setCalls[0].value != "1" {
			t.Errorf("got option value %q, want \"1\"", w.setCalls[0].value)
		}
	})

	t.Run("propagates SetServerOption error", func(t *testing.T) {
		w := &writerMock{setErr: errors.New("tmux exploded")}
		err := state.SetSkeletonMarker(w, "foo__0.0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("does not call UnsetServerOption", func(t *testing.T) {
		w := &writerMock{}
		_ = state.SetSkeletonMarker(w, "foo__0.0")
		if len(w.unsetCalls) != 0 {
			t.Errorf("got %d UnsetServerOption calls, want 0", len(w.unsetCalls))
		}
	})
}

func TestUnsetSkeletonMarker(t *testing.T) {
	t.Run("unsets server option named SkeletonMarkerPrefix+paneKey", func(t *testing.T) {
		w := &writerMock{}
		if err := state.UnsetSkeletonMarker(w, "foo__0.0"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(w.unsetCalls) != 1 {
			t.Fatalf("got %d UnsetServerOption calls, want 1", len(w.unsetCalls))
		}
		wantName := state.SkeletonMarkerPrefix + "foo__0.0"
		if w.unsetCalls[0] != wantName {
			t.Errorf("got option name %q, want %q", w.unsetCalls[0], wantName)
		}
	})

	t.Run("propagates UnsetServerOption error", func(t *testing.T) {
		w := &writerMock{unsetErr: errors.New("tmux exploded")}
		err := state.UnsetSkeletonMarker(w, "foo__0.0")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("does not call SetServerOption", func(t *testing.T) {
		w := &writerMock{}
		_ = state.UnsetSkeletonMarker(w, "foo__0.0")
		if len(w.setCalls) != 0 {
			t.Errorf("got %d SetServerOption calls, want 0", len(w.setCalls))
		}
	})
}

func TestUnsetSkeletonMarkerForFIFO(t *testing.T) {
	t.Run("derives paneKey from absolute FIFO path and unsets the matching option", func(t *testing.T) {
		w := &writerMock{}
		fifo := "/tmp/portal/hydrate-foo__0.0.fifo"
		if err := state.UnsetSkeletonMarkerForFIFO(w, fifo); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(w.unsetCalls) != 1 {
			t.Fatalf("got %d UnsetServerOption calls, want 1", len(w.unsetCalls))
		}
		wantName := state.SkeletonMarkerPrefix + "foo__0.0"
		if w.unsetCalls[0] != wantName {
			t.Errorf("got option name %q, want %q", w.unsetCalls[0], wantName)
		}
	})

	t.Run("derives paneKey from bare FIFO basename", func(t *testing.T) {
		w := &writerMock{}
		if err := state.UnsetSkeletonMarkerForFIFO(w, "hydrate-bar__1.2.fifo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantName := state.SkeletonMarkerPrefix + "bar__1.2"
		if len(w.unsetCalls) != 1 || w.unsetCalls[0] != wantName {
			t.Errorf("got unsetCalls %v, want [%q]", w.unsetCalls, wantName)
		}
	})

	t.Run("propagates UnsetServerOption error", func(t *testing.T) {
		w := &writerMock{unsetErr: errors.New("tmux exploded")}
		err := state.UnsetSkeletonMarkerForFIFO(w, "/tmp/portal/hydrate-foo__0.0.fifo")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("does not call SetServerOption", func(t *testing.T) {
		w := &writerMock{}
		_ = state.UnsetSkeletonMarkerForFIFO(w, "/tmp/portal/hydrate-foo__0.0.fifo")
		if len(w.setCalls) != 0 {
			t.Errorf("got %d SetServerOption calls, want 0", len(w.setCalls))
		}
	})
}
