package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUseMapsRepoRootFromSubdirAndRemovesOverrides(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/x", true)
	sub := filepath.Join(repo, "a", "b")
	os.MkdirAll(sub, 0o755)
	w.Git(repo, "config", "user.email", "v1@leftover")
	r := w.Gids(sub, "y\n", nil, "use", "personal") // case-insensitive
	want(t, r.Code == 0, "%+v", r)
	want(t, w.cfg().Rules["~/projects/x/"] == "Personal", "rule must be the repo root: %v", w.cfg().Rules)
	_, err := w.TryGit(repo, "config", "--local", "user.email")
	want(t, err != nil, "override not removed")
	st := w.MustGids(sub, "status")
	want(t, strings.Contains(st, meID) && strings.Contains(st, "rule ~/projects/x/"), "status:\n%s", st)
	w.check()
}

func TestUsePickerFallback(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("projects/x", true)
	r := w.Gids(repo, "1\n", nil, "use")
	want(t, r.Code == 0 && w.cfg().Rules["~/projects/x/"] == "Work", "%+v %v", r, w.cfg().Rules)
	r = w.Gids(repo, "", nil, "use") // Ctrl-D at the picker
	want(t, r.Code == 1 && strings.Contains(r.Err, "Cancelled"), "%+v", r)
	r = w.Gids(w.Home, "", nil, "use", "Work")
	want(t, r.Code == 1 && strings.Contains(r.Err, "not inside a git repository"), "%+v", r)
}
