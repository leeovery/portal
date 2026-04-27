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
// empty-value paths independently.
type checkerMock struct {
	val   string
	found bool
	err   error
}

func (m *checkerMock) TryGetServerOption(name string) (string, bool, error) {
	return m.val, m.found, m.err
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
