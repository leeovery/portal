//go:build darwin || linux

package bootid_test

import (
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootid"
)

func TestRead(t *testing.T) {
	first, err := bootid.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if first == "" {
		t.Fatal("Read = \"\", want this boot's id")
	}

	second, err := bootid.Read()
	if err != nil || second != first {
		t.Errorf("a second Read = %q, %v; want the same id, %q", second, err, first)
	}
}

func TestBootTime(t *testing.T) {
	at, err := bootid.BootTime()
	if err != nil {
		t.Fatalf("BootTime: %v", err)
	}

	if now := time.Now(); !at.Before(now) || at.Before(now.AddDate(-10, 0, 0)) {
		t.Errorf("BootTime = %v, want a time before now, %v, and within ten years of it", at, now)
	}
}
