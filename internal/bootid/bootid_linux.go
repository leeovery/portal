package bootid

import (
	"os"
	"strings"
	"time"
)

func read() (string, error) {
	id, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(id)), err
}

func bootTime() (time.Time, error) {
	stat, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}, err
	}
	return btime(stat)
}
