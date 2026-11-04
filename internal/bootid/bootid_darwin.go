package bootid

import "golang.org/x/sys/unix"

func read() (string, error) { return unix.Sysctl("kern.bootsessionuuid") }
