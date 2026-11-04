package bootid

import (
	"os"
	"strings"
)

func read() (string, error) {
	id, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(id)), err
}
