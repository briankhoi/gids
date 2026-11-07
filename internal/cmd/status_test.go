package cmd_test

import (
	"strings"
	"testing"
)

func TestStatusWarnings(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	repo := w.Repo("work/app", true)
	w.Git(repo, "config", "user.email", "x@override")
	w.Git(repo, "config", "core.hooksPath", ".husky")
	st := w.MustGids(repo, "status")
	want(t, strings.Contains(st, "overridden") && strings.Contains(st, "core.hooksPath"), "status:\n%s", st)
	un := w.Repo("projects/u", true)
	want(t, strings.Contains(w.MustGids(un, "status"), "no rule covers"), "unmapped status")
	want(t, strings.Contains(w.MustGids(w.Home, "status"), "Not in a git repository"), "outside repo")
	w.MustGids(w.Home, "uninstall")
	want(t, strings.Contains(w.MustGids(repo, "status"), "isn't installed"), "not installed warning")
	w.check()
}
