package runner

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The termios requests that read a terminal's settings, and that set them
// once it has taken its output, dropping input not yet read.
const (
	getTermios        = unix.TIOCGETA
	setTermiosFlushed = unix.TIOCSETAF
)

// pathBuf takes F_GETPATH's answer. fcntl's wrapper is handed the buffer's
// address as an integer, which the garbage collector neither keeps alive nor
// updates when it moves a stack, so the buffer is a package variable, which
// never moves.
var (
	pathMu  sync.Mutex
	pathBuf [unix.PathMax]byte
)

// terminalPath is the path of the file open on fd.
func terminalPath(fd int) (string, error) {
	pathMu.Lock()
	defer pathMu.Unlock()
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_GETPATH, int(uintptr(unsafe.Pointer(&pathBuf[0])))); err != nil {
		return "", err
	}
	return unix.ByteSliceToString(pathBuf[:]), nil
}
