package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithAddsTrailersAndRespectsEmptyMessages(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	w.MustGids(w.Home, "profile", "add", "--name", "alice", "--git-name", "Alice Smith", "--email", "alice@x.com")
	repo := w.Repo("work/app", true)
	w.MustGids(repo, "with", "alice", "Bob Lee <bob@y.com>")
	want(t, strings.Contains(w.MustGids(repo, "status"), "Alice Smith <alice@x.com>, Bob Lee <bob@y.com>"), "status")
	w.commit(repo, "")
	msg := w.Git(repo, "log", "-1", "--format=%B")
	want(t, strings.Contains(msg, "Co-authored-by: Alice Smith <alice@x.com>") && strings.Contains(msg, "Co-authored-by: Bob Lee <bob@y.com>"), "trailers:\n%s", msg)
	// Empty editor message must still abort (PROGRESS D1).
	head := w.Git(repo, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(repo, "f.txt"), []byte("again\n"), 0o644)
	w.Git(repo, "add", "f.txt")
	_, err := w.TryGit(repo, "-c", "core.editor=true", "commit")
	want(t, err != nil && w.Git(repo, "rev-parse", "HEAD") == head, "empty message created a commit")
	// Amend doesn't duplicate trailers.
	w.Git(repo, "commit", "--amend", "--no-edit")
	msg = w.Git(repo, "log", "-1", "--format=%B")
	want(t, strings.Count(msg, "alice@x.com") == 1, "duplicated trailer:\n%s", msg)
	w.MustGids(repo, "with", "--clear")
	w.commit(repo, "")
	want(t, !strings.Contains(w.Git(repo, "log", "-1", "--format=%B"), "Co-authored-by"), "cleared session still adds")
}

func TestWithOnceSurvivesAbortedCommit(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "rule", "add", "~/work", "Work")
	repo := w.Repo("work/app", true)
	w.MustGids(repo, "with", "--once", "Bob Lee <bob@y.com>")
	os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\nexit 1\n"), 0o755)
	w.commit(repo, "")
	want(t, strings.Contains(w.MustGids(repo, "with"), "Bob Lee"), "aborted commit consumed --once")
	os.Remove(filepath.Join(repo, ".git", "hooks", "pre-commit"))
	w.commit(repo, "")
	want(t, strings.Contains(w.Git(repo, "log", "-1", "--format=%B"), "Bob Lee"), "trailer missing")
	want(t, strings.Contains(w.MustGids(repo, "with"), "No co-authors"), "--once not cleared")
	w.commit(repo, "")
	want(t, !strings.Contains(w.Git(repo, "log", "-1", "--format=%B"), "Bob Lee"), "--once applied twice")
}

func TestWithErrors(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("p", true)
	for _, args := range [][]string{{"with", "ghost"}, {"with", "Bad <nope>"}, {"with", "--clear", "x"}} {
		r := w.Gids(repo, "", nil, args...)
		want(t, r.Code == 1 && r.Err != "", "%v should fail: %+v", args, r)
	}
	r := w.Gids(w.Home, "", nil, "with", "alice")
	want(t, r.Code == 1 && strings.Contains(r.Err, "not inside a git repository"), "%+v", r)
}
