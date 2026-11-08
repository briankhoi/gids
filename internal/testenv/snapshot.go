package testenv

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// CopyTree copies the directory src into dst (created if needed), keeping
// permissions and symlinks. It's how tests snapshot and restore a whole
// sandbox HOME (gids' files, ~/.gitconfig and repos), which is valid because
// nothing gids writes there contains the absolute home path (rules, the
// include and hooksPath all use "~/"; hooks embed only the binary's path).
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		info, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			l, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(l, to)
		case d.IsDir():
			return os.MkdirAll(to, info.Mode().Perm()|0o700)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}

// Step is one testhook.Point a gids process passed: an operation and its
// argument (a path, or a git command line).
type Step struct{ Op, Arg string }

// ReadTrace parses a GIDS_TEST_TRACE file.
func ReadTrace(path string) []Step {
	b, _ := os.ReadFile(path)
	var steps []Step
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if line == "" {
			continue
		}
		op, arg, _ := strings.Cut(line, " ")
		steps = append(steps, Step{op, arg})
	}
	return steps
}
