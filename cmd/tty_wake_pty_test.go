//go:build darwin

package cmd

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const ttyWakeBudget = 2 * time.Second

// ttyWakeHeld is how long an Await is watched for returning of its own: ten
// times the quiet that ends an arrival.
const ttyWakeHeld = 10 * resumeInputQuiet

type awaited struct {
	arrived bool
	err     error
}

// openWaker is a waker over a raw pty's slave, closed at the test's end.
func openWaker(t *testing.T) (w *ttyWaker, master func(string), fd int) {
	t.Helper()
	m, _, fd := rawPTY(t)
	w, err := newTTYWaker(fd)
	if err != nil {
		t.Fatalf("newTTYWaker() error = %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w, func(keys string) {
		if _, err := m.Write([]byte(keys)); err != nil {
			t.Fatalf("type into the pty: %v", err)
		}
	}, fd
}

// awaitOff runs one Await off the test's goroutine.
func awaitOff(w *ttyWaker) <-chan awaited {
	result := make(chan awaited, 1)
	go func() {
		arrived, err := w.Await()
		result <- awaited{arrived, err}
	}()
	return result
}

func answerWithin(t *testing.T, result <-chan awaited) awaited {
	t.Helper()
	select {
	case r := <-result:
		if r.err != nil {
			t.Fatalf("Await() error = %v", r.err)
		}
		return r
	case <-time.After(ttyWakeBudget):
		t.Fatal("Await never returned")
	}
	return awaited{}
}

// assertHeld asserts the Await behind result is still waiting, then wakes it
// so the test ends with none in flight.
func assertHeld(t *testing.T, w *ttyWaker, result <-chan awaited, why string) {
	t.Helper()
	select {
	case r := <-result:
		t.Fatalf("Await returned %+v %s", r, why)
	case <-time.After(ttyWakeHeld):
	}
	w.Wake()
	answerWithin(t, result)
}

func TestTTYWaker_RealPTY(t *testing.T) {
	t.Run("it waits with no timeout while nothing arrives and nothing wakes it", func(t *testing.T) {
		w, _, _ := openWaker(t)

		assertHeld(t, w, awaitOff(w), "with nothing arriving and no wake")
	})

	t.Run("it reports a byte that arrives, reading nothing", func(t *testing.T) {
		w, typeKeys, fd := openWaker(t)
		result := awaitOff(w)

		typeKeys("d")

		if r := answerWithin(t, result); !r.arrived {
			t.Fatal("Await() = false for a byte that arrived, want true")
		}
		if got := string(queuedOn(t, fd)); got != "d" {
			t.Errorf("the terminal held %q after the Await, want the byte unread", got)
		}
	})

	t.Run("it reports the terminal's end as input, for the read to find", func(t *testing.T) {
		m, _, fd := rawPTY(t)
		w, err := newTTYWaker(fd)
		if err != nil {
			t.Fatalf("newTTYWaker() error = %v", err)
		}
		t.Cleanup(func() { _ = w.Close() })
		result := awaitOff(w)

		_ = m.Close()

		if r := answerWithin(t, result); !r.arrived {
			t.Fatal("Await() = false on a hung-up terminal, want true")
		}
	})

	t.Run("a wake ends the Await in flight, reporting no input", func(t *testing.T) {
		w, _, _ := openWaker(t)
		result := awaitOff(w)

		w.Wake()

		if r := answerWithin(t, result); r.arrived {
			t.Fatal("Await() = true for a wake, want false")
		}
	})

	t.Run("a wake taken before an Await ends that one, and it takes the wake", func(t *testing.T) {
		w, _, _ := openWaker(t)

		w.Wake()
		if r := answerWithin(t, awaitOff(w)); r.arrived {
			t.Fatal("Await() = true after a wake, want false")
		}

		assertHeld(t, w, awaitOff(w), "for a wake an earlier Await already took")
	})

	t.Run("however many wakes are taken, none blocks, and one Await takes them all", func(t *testing.T) {
		w, _, _ := openWaker(t)

		woken := make(chan struct{})
		go func() {
			defer close(woken)
			// Past any pipe's buffer.
			for range 1 << 17 {
				w.Wake()
			}
		}()
		select {
		case <-woken:
		case <-time.After(ttyWakeBudget):
			t.Fatal("Wake blocked on a full pipe")
		}
		if r := answerWithin(t, awaitOff(w)); r.arrived {
			t.Fatal("Await() = true after the wakes, want false")
		}

		assertHeld(t, w, awaitOff(w), "for wakes an earlier Await already took")
	})

	t.Run("a byte and a wake together report the byte, and the wake is taken with it", func(t *testing.T) {
		w, typeKeys, fd := openWaker(t)
		typeKeys("d")
		if arrived, err := awaitTTYInput(fd, ttyWakeBudget); err != nil || !arrived {
			t.Fatalf("the byte typed never reached the terminal: %v", err)
		}
		w.Wake()

		if r := answerWithin(t, awaitOff(w)); !r.arrived {
			t.Fatal("Await() = false with a byte arrived as it was woken, want true")
		}
		if got := string(queuedOn(t, fd)); got != "d" {
			t.Fatalf("the terminal held %q after the Await, want the byte unread", got)
		}

		assertHeld(t, w, awaitOff(w), "for a wake the Await that reported the byte took")
	})

	t.Run("its pipe is closed on exec", func(t *testing.T) {
		w, _, _ := openWaker(t)

		for _, end := range w.pipe {
			flags, err := unix.FcntlInt(uintptr(end), unix.F_GETFD, 0)
			if err != nil {
				t.Fatalf("read the pipe's descriptor flags: %v", err)
			}
			if flags&unix.FD_CLOEXEC == 0 {
				t.Errorf("pipe descriptor %d survives an exec, want it closed on exec", end)
			}
		}
	})
}

func TestTTYWaker_RefusesADescriptorSelectCannotHold(t *testing.T) {
	for _, fd := range []int{-1, unix.FD_SETSIZE} {
		if w, err := newTTYWaker(fd); !errors.Is(err, errUnselectableTTY) {
			if w != nil {
				_ = w.Close()
			}
			t.Errorf("newTTYWaker(%d) error = %v, want %v", fd, err, errUnselectableTTY)
		}
	}
}
