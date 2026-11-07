package cmd_test

import (
	"strings"
	"testing"
)

// TestAcceptancePR8 mirrors briankhoi/discord-music-bot#8: 15 commits on a
// branch authored with the school identity, re-authored with --since main.
func TestAcceptancePR8(t *testing.T) {
	w := newWorld(t, true)
	repo := w.Repo("code/discord-music-bot", true)
	w.Commit(repo, "main work", "Brian Nguyen <nguyenbriankhoi@gmail.com>")
	base := w.Git(repo, "rev-parse", "main")
	w.Git(repo, "checkout", "-q", "-b", "dev-2026.11")
	subjects := []string{"Update dependencies", "Fix bugs", "Tell users when", "Add /remove", "Add end-to-end test",
		"Fix /play", "Add Now playing card\n\nWith buttons.\n\nCo-authored-by: Pal <pal@x>", "Add loop modes", "Add /previous",
		"Add /seek", "Add /swap", "Add /clear", "Give everyone a minute", "Fix ESLint", "Add a systemd service"}
	for _, s := range subjects {
		w.Commit(repo, s, "Brian Nguyen <bqn@andrew.cmu.edu>")
	}
	before := w.Git(repo, "log", "--format=%T %ad %cd %B", "--date=raw", "main..HEAD")
	mainBefore := w.Git(repo, "rev-parse", "main")

	r := w.Gids(repo, "", nil, "amend", "--since", "main", "--author", "Personal") // EOF at confirm → nothing
	want(t, r.Code == 1 && w.Git(repo, "rev-parse", "HEAD") != "" && strings.Contains(w.Git(repo, "log", "-1", "--format=%ae"), "andrew"), "unconfirmed amend changed things: %+v", r)
	r = w.Gids(repo, "n\n", nil, "amend", "--since", "main", "--author", "Personal")
	want(t, r.Code == 0 && strings.Contains(r.Out, "Nothing changed"), "%+v", r)

	r = w.Gids(repo, "y\n", nil, "amend", "--since", "main", "--author", "Personal")
	want(t, r.Code == 0, "%+v", r)
	want(t, strings.Contains(r.Out, "Re-author 15 commit(s) on dev-2026.11") && strings.Contains(r.Out, "To undo"), "%s", r.Out)
	ids := strings.Split(w.Git(repo, "log", "--format=%an <%ae>|%cn <%ce>", "main..HEAD"), "\n")
	want(t, len(ids) == 15, "%d commits", len(ids))
	for _, id := range ids {
		want(t, id == meID+"|"+meID, "not fixed: %q", id)
	}
	want(t, w.Git(repo, "log", "--format=%T %ad %cd %B", "--date=raw", "main..HEAD") == before, "content/dates changed")
	want(t, w.Git(repo, "rev-parse", "main") == mainBefore && w.Git(repo, "merge-base", "main", "HEAD") == base, "main touched or rebased")
	want(t, strings.Contains(w.Git(repo, "log", "--format=%B", "main..HEAD"), "Co-authored-by: Pal <pal@x>"), "existing co-author lost")
	r = w.Gids(repo, "", nil, "amend", "--since", "main", "--author", "Personal", "-y")
	want(t, r.Code == 0 && strings.Contains(r.Out, "Nothing to do"), "second run should be a no-op: %+v", r)
	w.check()
}

func TestAmendCLIErrorsAndCoauthors(t *testing.T) {
	w := newWorld(t, true)
	w.MustGids(w.Home, "profile", "add", "--name", "alice", "--git-name", "Alice Smith", "--email", "alice@x.com")
	repo := w.Repo("projects/a", true)
	w.Commit(repo, "feat\n\nCo-authored-by: Old Pal <old@x>", globalID)
	for _, tc := range []struct {
		args []string
		err  string
	}{
		{[]string{"amend", "-y"}, "no profile is mapped"},
		{[]string{"amend", "-y", "--author", "Ghost"}, "not found"},
		{[]string{"amend", "-y", "-n", "0", "--author", "Work"}, "positive"},
		{[]string{"amend", "-y", "-n", "2", "--since", "HEAD~1", "--author", "Work"}, "either -n or --since"},
		{[]string{"amend", "-y", "--since", "nope", "--author", "Work"}, "unknown revision"},
		{[]string{"amend", "-y", "-n", "2", "--add-coauthor", "alice", "--author", "Work"}, "one commit at a time"},
		{[]string{"amend", "-y", "--remove-coauthor", "Old Pal", "--author", "Work"}, "takes an email"},
	} {
		r := w.Gids(repo, "", nil, tc.args...)
		want(t, r.Code == 1 && strings.Contains(r.Err, tc.err), "%v: %+v", tc.args, r)
	}
	w.MustGids(repo, "amend", "-y", "--author", "Work", "--add-coauthor", "alice", "--remove-coauthor", "old@x")
	msg := w.Git(repo, "log", "-1", "--format=%B")
	want(t, strings.Contains(msg, "Co-authored-by: Alice Smith <alice@x.com>") && !strings.Contains(msg, "old@x"), "msg:\n%s", msg)
	want(t, w.Author(repo) == workID, "author %q", w.Author(repo))
	w.check()
}
