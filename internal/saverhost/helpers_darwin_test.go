package saverhost_test

import (
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenFDs(t *testing.T) {
	t.Run("it counts a descriptor opened and no longer once it is closed", func(t *testing.T) {
		before := openFDs(t)
		f, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatal(err)
		}

		if got := openFDs(t); got != before+1 {
			t.Errorf("openFDs with a file open = %d, want %d", got, before+1)
		}
		_ = f.Close()
		if got := openFDs(t); got != before {
			t.Errorf("openFDs once it is closed = %d, want %d", got, before)
		}
	})

	t.Run("it counts a kqueue, whose /dev/fd entry can't be lstat'd", func(t *testing.T) {
		before := openFDs(t)
		kq, err := unix.Kqueue()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = unix.Close(kq) }()

		if got := openFDs(t); got != before+1 {
			t.Errorf("openFDs with a kqueue open = %d, want %d", got, before+1)
		}
	})
}
