package state_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

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

func TestRestoreWindowActive(t *testing.T) {
	t.Run("reports active when @portal-restoring is set", func(t *testing.T) {
		active, err := state.RestoreWindowActive(state.IsRestoringSet(&checkerMock{val: "1", found: true}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !active {
			t.Errorf("got false; want true (marker set)")
		}
	})

	t.Run("reports inactive when @portal-restoring is absent", func(t *testing.T) {
		active, err := state.RestoreWindowActive(state.IsRestoringSet(&checkerMock{found: false}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if active {
			t.Errorf("got true; want false (marker absent)")
		}
	})

	t.Run("presumes active on a failed read and returns the error alongside", func(t *testing.T) {
		sentinel := errors.New("tmux exploded")
		active, err := state.RestoreWindowActive(state.IsRestoringSet(&checkerMock{err: sentinel}))
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v; want the read error", err)
		}
		if !active {
			t.Errorf("got false; want true (a failed read counts as set)")
		}
	})
}
