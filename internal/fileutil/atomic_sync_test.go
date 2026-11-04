package fileutil

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

// syncScript answers each call to a sync with the next of its errors, nil once
// they run out, recording the order the syncs were made in.
type syncScript struct {
	calls []string
	errs  map[string][]error
}

func (s *syncScript) sync(name string) func(uintptr) error {
	return func(uintptr) error {
		s.calls = append(s.calls, name)
		queue := s.errs[name]
		if len(queue) == 0 {
			return nil
		}
		s.errs[name] = queue[1:]
		return queue[0]
	}
}

func TestOrderedSync(t *testing.T) {
	boom := errors.New("device gone")
	for _, tc := range []struct {
		name      string
		errs      map[string][]error
		wantCalls []string
		wantErr   error
	}{
		{"a sync that succeeds needs no fallback", nil, []string{"barrier"}, nil},
		{"a filesystem answering ENOTSUP gets the fallback", map[string][]error{"barrier": {syscall.ENOTSUP}}, []string{"barrier", "fsync"}, nil},
		{"a filesystem answering EINVAL gets the fallback", map[string][]error{"barrier": {syscall.EINVAL}}, []string{"barrier", "fsync"}, nil},
		{"any other failure is returned, with no fallback", map[string][]error{"barrier": {boom}}, []string{"barrier"}, boom},
		{"an interrupted sync is made again", map[string][]error{"barrier": {syscall.EINTR, syscall.EINTR}}, []string{"barrier", "barrier", "barrier"}, nil},
		{"an interrupted fallback is made again", map[string][]error{"barrier": {syscall.ENOTSUP}, "fsync": {syscall.EINTR}}, []string{"barrier", "fsync", "fsync"}, nil},
		{"a fallback that fails is returned", map[string][]error{"barrier": {syscall.ENOTSUP}, "fsync": {boom}}, []string{"barrier", "fsync"}, boom},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := &syncScript{errs: tc.errs}

			err := orderedSync(3, script.sync("barrier"), script.sync("fsync"))

			if !errors.Is(err, tc.wantErr) || (tc.wantErr == nil && err != nil) {
				t.Errorf("orderedSync = %v, want %v", err, tc.wantErr)
			}
			if !slices.Equal(script.calls, tc.wantCalls) {
				t.Errorf("syncs = %v, want %v", script.calls, tc.wantCalls)
			}
		})
	}
}

func TestSyncs_OnARealFilesystem(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	if err := syncFileData(f); err != nil {
		t.Errorf("syncFileData = %v, want the file synced", err)
	}
	if err := syncDirectory(dir); err != nil {
		t.Errorf("syncDirectory = %v, want the directory synced", err)
	}
	if err := syncDirectory(filepath.Join(dir, "missing")); err == nil {
		t.Error("syncDirectory of a missing directory = nil, want an error")
	}
}
