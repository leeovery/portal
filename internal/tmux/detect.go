package tmux

import (
	"os"
	"strconv"
	"strings"
)

func InsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// ServerPIDFromEnv returns the pid of the tmux server named by a TMUX value,
// which tmux sets to "<socket>,<server pid>,<session>" for every pane process
// and every run-shell job it starts. The pid is read from the second field
// from the end, since a socket path may carry a comma.
func ServerPIDFromEnv(value string) (int, bool) {
	fields := strings.Split(value, ",")
	if len(fields) < 3 {
		return 0, false
	}
	pid, err := strconv.Atoi(fields[len(fields)-2])
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}
