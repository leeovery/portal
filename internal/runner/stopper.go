package runner

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

var errStopped = errors.New("copy loop stopped")

// stopper stops a copy loop waiting for input, which a blocking read cannot
// be made to give up: the loop polls a pipe beside its source, and stopping
// closes the pipe's write end.
type stopper struct {
	r, w *os.File
	once sync.Once
}

func newStopper() (*stopper, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("make a stop pipe: %w", err)
	}
	return &stopper{r: r, w: w}, nil
}

func (s *stopper) stop() { s.once.Do(func() { _ = s.w.Close() }) }

func (s *stopper) close() {
	s.stop()
	_ = s.r.Close()
}

// read waits for src to have input and reads it, or returns errStopped once
// the stopper is stopped, even with input waiting.
func (s *stopper) read(src *os.File, buf []byte) (int, error) {
	fds := []unix.PollFd{
		{Fd: int32(src.Fd()), Events: unix.POLLIN},
		{Fd: int32(s.r.Fd()), Events: unix.POLLIN},
	}
	for {
		_, err := unix.Poll(fds, -1)
		switch {
		case errors.Is(err, unix.EINTR):
		case err != nil:
			return 0, err
		case fds[1].Revents != 0:
			return 0, errStopped
		case fds[0].Revents != 0:
			return src.Read(buf)
		}
	}
}
