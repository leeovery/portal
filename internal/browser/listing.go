// Package browser provides directory browsing utilities for the Portal file browser TUI.
package browser

import (
	"cmp"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// DirEntry represents a single directory entry in a listing.
type DirEntry struct {
	Name      string
	IsSymlink bool
}

// ListDirectories returns a sorted slice of directory entries at the given path.
// Files are excluded. Hidden directories (names starting with ".") are excluded
// unless showHidden is true. Returns an empty slice (not an error) when the
// directory cannot be read due to permission restrictions.
func ListDirectories(path string, showHidden bool) ([]DirEntry, error) {
	rawEntries, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return []DirEntry{}, nil
		}
		return nil, err
	}

	var result []DirEntry

	for _, entry := range rawEntries {
		name := entry.Name()

		if !showHidden && strings.HasPrefix(name, ".") {
			continue
		}

		isSymlink := entry.Type()&os.ModeSymlink != 0

		isDir := entry.IsDir()
		if isSymlink {
			target, err := os.Stat(filepath.Join(path, name))
			if err != nil {
				continue
			}
			isDir = target.IsDir()
		}

		if !isDir {
			continue
		}

		result = append(result, DirEntry{
			Name:      name,
			IsSymlink: isSymlink,
		})
	}

	slices.SortFunc(result, func(a, b DirEntry) int {
		return cmp.Compare(a.Name, b.Name)
	})

	if result == nil {
		result = []DirEntry{}
	}

	return result, nil
}