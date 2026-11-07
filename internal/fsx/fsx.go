// Package fsx holds the few filesystem primitives gids relies on for crash
// safety: atomic replacement of a file's contents and symlink-aware targets.
package fsx

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gids/internal/testhook"
)

// TempPrefix marks gids' temporary files so leftovers from a killed process
// can be recognised and cleaned up.
const TempPrefix = ".gids-tmp-"

// Target resolves path to the file that a write should really change: if path
// is a symlink (e.g. a dotfile manager's ~/.gitconfig), the link's final
// target; otherwise path itself. A dangling symlink is an error, because
// writing through it would silently create a file somewhere unexpected.
func Target(path string) (string, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if fi.Mode()&fs.ModeSymlink == 0 {
		return path, nil
	}
	t, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("%s is a symlink that can't be resolved: %w", path, err)
	}
	return t, nil
}

// WriteAtomic replaces the contents of path with data so that readers (and a
// crash at any instant) observe either the old file or the new one, never a
// mix. Symlinks are written through, keeping the link. An existing file keeps
// its permissions; a new one gets perm. Parent directories are created. An
// existing target that isn't a regular file is an error.
func WriteAtomic(path string, data []byte, perm fs.FileMode) error {
	target, err := Target(path)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(target); err == nil {
		// Never replace a device, FIFO or directory with a regular file
		// (e.g. GIT_CONFIG_GLOBAL=/dev/null, run as root).
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("%s isn't a regular file; refusing to replace it", target)
		}
		perm = fi.Mode().Perm()
	}
	dir := filepath.Dir(target)
	testhook.Point("mkdir", dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	testhook.Point("create-temp", target)
	tmp, err := os.CreateTemp(dir, TempPrefix+filepath.Base(target)+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()
	testhook.Point("write-temp", target)
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	testhook.Point("rename", target)
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	ok = true
	syncDir(dir)
	return nil
}

// Remove deletes path if it exists (a missing file is not an error).
func Remove(path string) error {
	testhook.Point("remove", path)
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err == nil {
		syncDir(filepath.Dir(path))
	}
	return err
}

// Chmod sets path's permission bits.
func Chmod(path string, perm fs.FileMode) error {
	testhook.Point("chmod", path)
	return os.Chmod(path, perm)
}

// RemoveDirIfEmpty removes dir only if it is empty (best effort: anything
// else, including "not empty", is ignored).
func RemoveDirIfEmpty(dir string) {
	testhook.Point("rmdir", dir)
	if os.Remove(dir) == nil {
		syncDir(filepath.Dir(dir))
	}
}

// IsTemp reports whether name (a base name) is a leftover gids temp file.
func IsTemp(name string) bool { return strings.HasPrefix(name, TempPrefix) }

// syncDir makes a rename or removal durable. Best effort: some platforms
// (Windows) can't fsync directories, and a failure here doesn't undo the
// already-atomic rename.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
}
