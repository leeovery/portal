package bootid

import (
	"bufio"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"
)

// btime reads the boot time, in whole seconds since the epoch, from the
// btime line of linux's /proc/stat.
func btime(stat []byte) (time.Time, error) {
	lines := bufio.NewScanner(bytes.NewReader(stat))
	for lines.Scan() {
		value, ok := strings.CutPrefix(lines.Text(), "btime ")
		if !ok {
			continue
		}
		secs, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(secs, 0), nil
	}
	if err := lines.Err(); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, errors.New("/proc/stat holds no btime line")
}
