package fileutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Write-phase sentinels let a caller errors.Is which step of AtomicWrite failed.
// Each sentinel's string is its error_class token.
var (
	// ErrWriteTempCreate covers creating the parent directory as well as the
	// temp file: the closed error_class space has no separate mkdir phase.
	ErrWriteTempCreate = errors.New("write-failed-temp-create")
	ErrWriteWrite      = errors.New("write-failed-write")
	// ErrWriteFsync covers the temp file's sync and close before the rename,
	// which fail the write, and the directory's sync after it, which only
	// ReportUnsynced hears of: the file is replaced by then.
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

// AtomicWrite names each temp file it makes tempPrefix, a random string of
// decimal digits, then tempSuffix: os.CreateTemp's random part for a pattern
// with one "*".
const (
	tempPrefix = ".atomic-"
	tempSuffix = ".tmp"
)

// IsAtomicTemp reports whether name is the base name of a temp file
// AtomicWrite makes. A write cut short before its rename, by a crash or a kill,
// leaves its temp file beside the target.
func IsAtomicTemp(name string) bool {
	random, ok := strings.CutPrefix(name, tempPrefix)
	if !ok {
		return false
	}
	random, ok = strings.CutSuffix(random, tempSuffix)
	return ok && random != "" && strings.Trim(random, "0123456789") == ""
}

// AtomicWrite0600 writes data to path and chmods the result to 0600. The temp
// file is already created 0600; the extra chmod defends against a permissive
// umask leaking broader bits through.
func AtomicWrite0600(path string, data []byte, opts ...WriteOption) error {
	if err := AtomicWrite(path, data, opts...); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

// WriteOption adjusts one AtomicWrite.
type WriteOption func(*writeOptions)

type writeOptions struct {
	unsynced func(error)
}

// ReportUnsynced hands report the failure of the directory sync after the
// rename, wrapping ErrWriteFsync, for a caller that logs it. The write has
// landed all the same; what is lost is only its order against later writes.
func ReportUnsynced(report func(err error)) WriteOption {
	return func(o *writeOptions) { o.unsynced = report }
}

// AtomicWrite writes data to path via temp-file-and-rename, creating the parent
// directory if absent and removing the temp file on any error.
//
// Its writes are ordered: the temp file is synced before the rename, so the
// rename never persists ahead of the bytes it names, and the directory after
// it, so the rename persists ahead of any later write. After a crash or power
// loss each file is its old version or its new one, never empty. On darwin
// both syncs are F_BARRIERFSYNC, which orders without flushing the drive's
// cache, so the newest writes may be lost; on linux they are fsync. A
// filesystem that makes no such sync is written to without it.
//
// Once the rename lands the write has landed, and AtomicWrite returns nil: a
// directory sync that fails after it goes to the ReportUnsynced option, if
// given, rather than failing a write whose new content is already in place.
//
// A path that is a symlink is written through: the rename lands on the file the
// link names, and the link stays. Renaming over the link itself would replace it
// with a regular file, silently detaching a config file kept in a dotfiles
// repository and linked into place on the first write Portal makes to it.
func AtomicWrite(path string, data []byte, opts ...WriteOption) error {
	var o writeOptions
	for _, opt := range opts {
		opt(&o)
	}

	target, err := resolveWriteTarget(path)
	if err != nil {
		return fmt.Errorf("%w: failed to resolve symlink: %w", ErrWriteTempCreate, err)
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("%w: failed to create directory: %w", ErrWriteTempCreate, err)
	}

	tmp, err := os.CreateTemp(dir, tempPrefix+"*"+tempSuffix)
	if err != nil {
		return fmt.Errorf("%w: failed to create temp file: %w", ErrWriteTempCreate, err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to write temp file: %w", ErrWriteWrite, err)
	}

	if err := syncFile(tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to sync temp file: %w", ErrWriteFsync, err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to close temp file: %w", ErrWriteFsync, err)
	}

	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: failed to rename temp file: %w", ErrWriteRename, err)
	}

	if err := syncDir(dir); err != nil && o.unsynced != nil {
		o.unsynced(fmt.Errorf("%w: failed to sync directory after renaming over %s: %w", ErrWriteFsync, target, err))
	}

	return nil
}

// SyncDir orders the renames just made in the directory dir ahead of any
// later write, as AtomicWrite orders its own rename: a rename of a directory,
// which no AtomicWrite makes, persists ahead of what follows it. A filesystem
// that makes no such sync is passed over, as AtomicWrite passes it over.
func SyncDir(dir string) error { return syncDir(dir) }

var (
	syncFile = syncFileData
	syncDir  = syncDirectory
)

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
