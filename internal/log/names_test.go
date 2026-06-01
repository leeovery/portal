package log

import (
	"path/filepath"
	"testing"
)

func TestDayFile_JoinsDatedBasenameOntoStateDir(t *testing.T) {
	got := dayFile("/var/state", "2026-05-29")
	want := filepath.Join("/var/state", "portal.log.2026-05-29")
	if got != want {
		t.Errorf("dayFile = %q, want %q", got, want)
	}
}

func TestSymlinkPath_JoinsPortalLogOntoStateDir(t *testing.T) {
	got := symlinkPath("/var/state")
	want := filepath.Join("/var/state", "portal.log")
	if got != want {
		t.Errorf("symlinkPath = %q, want %q", got, want)
	}
}

func TestDaySegmentFile_AppendsDotNToDayFile(t *testing.T) {
	got := daySegmentFile("/var/state", "2026-05-29", 3)
	want := filepath.Join("/var/state", "portal.log.2026-05-29.3")
	if got != want {
		t.Errorf("daySegmentFile = %q, want %q", got, want)
	}
}
