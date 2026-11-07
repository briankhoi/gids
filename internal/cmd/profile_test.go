package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileCommands(t *testing.T) {
	w := newWorld(t, true)
	r := w.Gids(w.Home, "", nil, "profile", "add", "--name", "work", "--git-name", "X", "--email", "x@y")
	want(t, r.Code == 1, "case-insensitive duplicate accepted") // form gets EOF → Cancelled
	r = w.Gids(w.Home, "", nil, "profile", "add", "--name", "Bad", "--git-name", "X", "--email", "nope")
	want(t, r.Code == 1, "invalid email accepted")
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	r = w.Gids(w.Home, "", nil, "profile", "delete", "Work")
	want(t, r.Code == 1 && strings.Contains(r.Err, "--force"), "delete with rules: %+v", r)
	w.MustGids(w.Home, "profile", "edit", "Work", "--name", "Job", "--email", "job@co.com")
	c := w.cfg()
	want(t, c.Profile("Job") != nil && c.Rules["~/work/"] == "Job" && c.Profile("Job").GitName == "Work Me", "rename: %+v", c)
	_, err := os.Stat(filepath.Join(w.GidsDir(), "profiles", "Work.gitconfig"))
	want(t, os.IsNotExist(err), "old profile file not cleaned up")
	r = w.Gids(w.Home, "n\n", nil, "profile", "delete", "Personal")
	want(t, r.Code == 0 && w.cfg().Profile("Personal") != nil, "declined delete removed it")
	w.MustGids(w.Home, "profile", "delete", "Job", "--force")
	want(t, w.cfg().Profile("Job") == nil && len(w.cfg().Rules) == 0, "force delete")
	want(t, strings.Contains(w.MustGids(w.Home, "profile", "list"), "Personal"), "list")
	w.check()
}

func TestProfileAddFormFallback(t *testing.T) {
	w := newWorld(t, false)
	// Invalid answers are re-asked; optional fields can be skipped.
	r := w.Gids(w.Home, "bad name\nWork\nWork Me\nnot-an-email\nwork@co.com\n\nrelative/key\n~/.ssh/id_work\n\n", nil, "profile", "add")
	want(t, r.Code == 0, "%+v", r)
	p := w.cfg().Profile("Work")
	want(t, p != nil && p.SSHKey == "~/.ssh/id_work" && p.GitEmail == "work@co.com", "%+v", p)
}

func TestProfileImport(t *testing.T) {
	w := newWorld(t, true)
	cfgFile := w.Write(".ssh/config", "Host *\n  User x\nHost github-school\n  IdentityFile ~/.ssh/id_school\n  User git\nHost Work\n  IdentityFile ~/.ssh/w\nHost has space\n  IdentityFile relative\n")
	_ = cfgFile
	r := w.Gids(w.Home, "\nBrian Nguyen\nbqn@andrew.cmu.edu\n", nil, "profile", "import", "--file", "~/.ssh/config", "--host", "school")
	want(t, r.Code == 0, "%+v", r)
	p := w.cfg().Profile("github-school")
	want(t, p != nil && p.SSHKey == "~/.ssh/id_school" && p.Username == "git", "%+v", p)
	r = w.Gids(w.Home, "\n", nil, "profile", "import", "--file", "~/.ssh/config", "--host", "work")
	want(t, strings.Contains(r.Out, "already exists"), "existing profile should be skipped: %+v", r)
	r = w.Gids(w.Home, "", nil, "profile", "import", "--file", "~/.ssh/missing")
	want(t, r.Code == 1 && strings.Contains(r.Err, "cannot open SSH config"), "%+v", r)
}
