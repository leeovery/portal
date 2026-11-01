package fileutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Write-phase sentinels let a caller errors.Is which step of AtomicWrite failed.
// Each sentinel's string is its error_class token.
var (
	// ErrWriteTempCreate covers creating the parent directory as well as the
	// temp file: the closed error_class space has no separate mkdir phase.
	ErrWriteTempCreate = errors.New("write-failed-temp-create")
	ErrWriteWrite      = errors.New("write-failed-write")
	// AtomicWrite has no explicit Sync, so Close is where deferred write errors
	// surface and maps here.
	ErrWriteFsync  = errors.New("write-failed-fsync")
	ErrWriteRename = errors.New("write-failed-rename")
)

// ClassifyWriteError maps a wrapped AtomicWrite error to its error_class token.
// An error matching no sentinel falls back to "write-failed-write" — a deliberate
// floor, so an unrecognised failure is attributed to a write rather than dropped.
func ClassifyWriteError(err error) string {
	switch {
	case errors.Is(err, ErrWriteTempCreate):
		return "write-failed-temp-create"
	case errors.Is(err, ErrWriteWrite):
		return "write-failed-write"
	case errors.Is(err, ErrWriteFsync):
		return "write-failed-fsync"
	case errors.Is(err, ErrWriteRename):
		return "write-failed-rename"
	default:
		return "write-failed-write"
	}
}

// IsWriteFailure reports whether err carries one of AtomicWrite's write-phase
// sentinels. Unlike ClassifyWriteError it has no floor: an error from anywhere
// else reads as false.
func IsWriteFailure(err error) bool {
	for _, phase := range []error{ErrWriteTempCreate, ErrWriteWrite, ErrWriteFsync, ErrWriteRename} {
		if errors.Is(err, phase) {
			return true
		}
	}
	return false
}

// AtomicWrite0600 writes data to path and chmods the result to 0600. The temp
// file is already created 0600; the extra chmod defends against a permissive
// umask leaking broader bits through.
func AtomicWrite0600(path string, data []byte) error {
	if err := AtomicWrite(path, data); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

// AtomicWrite writes data to path via temp-file-and-rename, creating the parent
// directory if absent and removing the temp file on any error.
//
// A path that is a symlink is written through: the rename lands on the file the
// link names, and the link stays. Renaming over the link itself would replace it
// with a regular file, silently detaching a config file kept in a dotfiles
// repository and linked into place on the first write Portal makes to it.
func AtomicWrite(path string, data []byte) error {
	target, err := resolveWriteTarget(path)
	if err != nil {
		return fmt.Errorf("%w: failed to resolve symlink: %w", ErrWriteTempCreate, err)
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w: failed to create directory: %w", ErrWriteTempCreate, err)
	}

	tmp, err := os.CreateTemp(dir, ".atomic-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: failed to create temp file: %w", ErrWriteTempCreate, err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to write temp file: %w", ErrWriteWrite, err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to close temp file: %w", ErrWriteFsync, err)
	}

	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to rename temp file: %w", ErrWriteRename, err)
	}

	return nil
}

// maxLinkHops bounds the walk as the kernel bounds its own, so a link cycle is
// an error rather than a hang.
const maxLinkHops = 40

// resolveWriteTarget follows path through any chain of symlinks to the name the
// write should replace. It stops at the first name that is not a link,
// including one that does not exist yet, so a dangling link is written through
// and creates the file it names. Directory links along the way need no
// resolving: the temp file is created beside the final name, so the rename stays
// within one directory whatever path reaches it.
func resolveWriteTarget(path string) (string, error) {
	target := path
	for range maxLinkHops {
		info, err := os.Lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			return target, nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink == 0 {
			return target, nil
		}

		dest, err := os.Readlink(target)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(dest) {
			// The kernel reads a relative link from the directory the link
			// physically sits in, which is not the lexical parent when that
			// parent is reached through a directory link.
			linkDir, err := filepath.EvalSymlinks(filepath.Dir(target))
			if err != nil {
				return "", err
			}
			dest = filepath.Join(linkDir, dest)
		}
		target = dest
	}
	return "", fmt.Errorf("%s: more than %d levels of symbolic links", path, maxLinkHops)
}
