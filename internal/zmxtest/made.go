package zmxtest

import (
	"fmt"
	"sync"
)

// made is every zmx directory New made in this process. A helper that acts on
// a Backend's processes or sessions acts only in one of them: a Backend built
// on any other, such as one from an unpoisoned os.Environ() naming the
// developer's directory, would have it prove, signal and type into their own.
var made madeDirs

type madeDirs struct {
	mu   sync.Mutex
	dirs map[string]bool
}

func (m *madeDirs) add(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirs == nil {
		m.dirs = map[string]bool{}
	}
	m.dirs[dir] = true
}

// refuse is nil for a directory New made in this process, and an error naming
// any other.
func (m *madeDirs) refuse(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirs[dir] {
		return nil
	}
	return fmt.Errorf("zmxtest: %q is no zmx directory zmxtest made in this process, so nothing in it is acted on", dir)
}
