package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuleCommands(t *testing.T) {
	w := newWorld(t, true)
	dir := w.Path("code")
	os.MkdirAll(dir, 0o755)
	w.MustGids(dir, "rule", "add", ".", "Work")
	w.MustGids(dir, "rule", "add", "../code/", "Personal") // same dir → update
	want(t, len(w.cfg().Rules) == 1 && w.cfg().Rules["~/code/"] == "Personal", "%v", w.cfg().Rules)
	list := w.MustGids(w.Home, "rule", "list")
	want(t, strings.Contains(list, "~/code/") && strings.Contains(list, "Personal"), "%s", list)
	for _, bad := range [][]string{{"rule", "add", "~/x", "Ghost"}, {"rule", "add", "~bob/x", "Work"}, {"rule", "remove", "~/nope"}} {
		r := w.Gids(w.Home, "", nil, bad...)
		want(t, r.Code == 1, "%v should fail", bad)
	}
	w.MustGids(w.Home, "rule", "remove", "~/code")
	want(t, len(w.cfg().Rules) == 0, "remove: %v", w.cfg().Rules)
	w.check()
}

func TestRuleAppliesEverywhereBeneathIt(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	repo := w.Repo("work/deep/api", true)
	sub := filepath.Join(repo, "src", "pkg")
	os.MkdirAll(sub, 0o755)
	out, code := w.commit(sub, "")
	want(t, code == 0 && !strings.Contains(out, "gids"), "mapped commit should be silent: %d %q", code, out)
	want(t, w.Author(repo) == workID, "author %q", w.Author(repo))
	// Linked worktree outside ~/work is still covered (rule is on the main repo).
	wt := w.Path("elsewhere", "wt")
	w.Git(repo, "worktree", "add", "-q", wt)
	_, code = w.commit(wt, "")
	want(t, code == 0 && w.Author(wt) == workID, "worktree author %q", w.Author(wt))
	w.check()
}

func TestMostSpecificRuleWins(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	w.MustGids(w.Home, "rule", "add", "~/work/oss", "Personal")
	a, b := w.Repo("work/app", true), w.Repo("work/oss/lib", true)
	w.commit(a, "")
	w.commit(b, "")
	want(t, w.Author(a) == workID && w.Author(b) == meID, "%q %q", w.Author(a), w.Author(b))
}
