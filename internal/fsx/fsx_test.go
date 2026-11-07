package fsx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAtomicNewFileAndPerms(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "f")
	if err := WriteAtomic(p, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "one" {
		t.Fatalf("content %q", b)
	}
	os.Chmod(p, 0o640) // existing permissions are kept on rewrite
	if err := WriteAtomic(p, []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("perm %v, want 0640", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
}

func TestWriteAtomicThroughSymlinkKeepsLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "gitconfig")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte("old"), 0o644)
	link := filepath.Join(dir, ".gitconfig")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	if err := WriteAtomic(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced by a regular file")
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Fatalf("target content %q", b)
	}
}

func TestWriteAtomicRefusesDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "l")
	if err := os.Symlink(filepath.Join(dir, "missing", "x"), link); err != nil {
		t.Skip(err)
	}
	if err := WriteAtomic(link, []byte("x"), 0o644); err == nil {
		t.Fatal("wrote through a dangling symlink")
	}
}

func TestRemoveMissingIsFine(t *testing.T) {
	if err := Remove(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Fatal(err)
	}
}

func TestWriteAtomicRefusesNonRegularTargets(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a-dir")
	os.Mkdir(sub, 0o755)
	link := filepath.Join(dir, "link-to-dir")
	os.Symlink(sub, link)
	for _, p := range []string{sub, link, os.DevNull} {
		if err := WriteAtomic(p, []byte("x"), 0o644); err == nil {
			t.Errorf("WriteAtomic(%s) replaced a non-regular file", p)
		}
	}
	if fi, err := os.Stat(sub); err != nil || !fi.IsDir() {
		t.Fatalf("directory was replaced: %v %v", fi, err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if IsTemp(e.Name()) {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
