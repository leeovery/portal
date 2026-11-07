package runner

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// The termios requests that read a terminal's settings, and that set them
// once it has taken its output, dropping input not yet read.
const (
	getTermios        = unix.TCGETS
	setTermiosFlushed = unix.TCSETSF
)

// terminalPath is the path of the file open on fd.
func terminalPath(fd int) (string, error) {
	return os.Readlink("/proc/self/fd/" + strconv.Itoa(fd))
}
