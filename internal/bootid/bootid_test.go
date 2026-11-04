//go:build darwin || linux

package bootid_test

import (
	"testing"

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
