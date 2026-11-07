package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gids/internal/app"
	"gids/internal/git"
	"gids/internal/testenv"
)

const school = "Brian Nguyen <bqn@andrew.cmu.edu>"

var fix = app.Rewrite{Name: "Brian Nguyen", Email: "nguyenbriankhoi@gmail.com"}

// repoWithBranch: main has 2 commits; feature branches off with k commits.
func repoWithBranch(t *testing.T, k int) (*testenv.Sandbox, string, git.Client) {
	s := testenv.New(t)
	dir := s.Repo("bot", true)
	s.Commit(dir, "main 2", "Main Dev <main@x>")
	s.Git(dir, "checkout", "-q", "-b", "feature")
	for i := range k {
		s.Commit(dir, "feature "+string(rune('a'+i))+"\n\nbody\n\nSigned-off-by: B <b@x>", school)
	}
	return s, dir, git.Client{Dir: dir}
}

// snapshot is everything amend must preserve, per commit, newest first:
// tree, author and committer dates, parent count, and the raw message.
// (Parent shas necessarily change; topology is checked via the count.)
func snapshot(s *testenv.Sandbox, dir, rng string) []string {
	out := strings.Split(s.Git(dir, "log", "--format=%T|%ad|%cd|%P|%B<END>", "--date=raw", rng), "<END>")
	for i, c := range out {
		f := strings.SplitN(c, "|", 5)
		if len(f) == 5 {
			out[i] = strings.Join([]string{f[0], f[1], f[2], string(rune('0' + len(strings.Fields(f[3])))), f[4]}, "|")
		}
	}
	return out
}

func TestPlanAmendValidation(t *testing.T) {
	s, dir, g := repoWithBranch(t, 3)
	cases := []struct {
		n     int
		since string
		err   string
	}{
		{2, "main", "either -n or --since"},
		{-1, "", "positive"},
		{99, "", "only 5 commit(s) exist"},
		{0, "nope", `unknown revision "nope"`},
		{0, "--output=x", "unknown revision"},
		{0, "HEAD", "nothing to amend"},
		{0, "feature", "nothing to amend"},
	}
	for _, tc := range cases {
		if _, err := app.PlanAmend(g, tc.n, tc.since); err == nil || !strings.Contains(err.Error(), tc.err) {
			t.Errorf("PlanAmend(%d, %q) = %v, want %q", tc.n, tc.since, err, tc.err)
		}
	}
	// Unrelated history.
	s.Git(dir, "checkout", "-q", "--orphan", "other")
	s.Commit(dir, "orphan", school)
	if _, err := app.PlanAmend(g, 0, "main"); err == nil || !strings.Contains(err.Error(), "shares no history") {
		t.Errorf("unrelated: %v", err)
	}
	s.Git(dir, "checkout", "-q", "feature")
	// In-progress operation.
	os.WriteFile(filepath.Join(dir, ".git", "MERGE_HEAD"), []byte(s.Git(dir, "rev-parse", "main")), 0o644)
	if _, err := app.PlanAmend(g, 1, ""); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("merge in progress: %v", err)
	}
	os.Remove(filepath.Join(dir, ".git", "MERGE_HEAD"))
	// Empty repo.
	empty := s.Repo("empty", false)
	if _, err := app.PlanAmend(git.Client{Dir: empty}, 1, ""); err == nil || !strings.Contains(err.Error(), "no commits") {
		t.Errorf("empty repo: %v", err)
	}
}

func TestAmendSinceRewritesExactlyTheBranch(t *testing.T) {
	s, dir, g := repoWithBranch(t, 15)
	mainBefore := s.Git(dir, "rev-parse", "main")
	before := snapshot(s, dir, "main..feature")
	// main moves on after branching: must not affect the rewrite.
	s.Git(dir, "checkout", "-q", "main")
	s.Commit(dir, "main 3", "Main Dev <main@x>")
	mainMoved := s.Git(dir, "rev-parse", "main")
	s.Git(dir, "checkout", "-q", "feature")
	oldHead := s.Git(dir, "rev-parse", "HEAD")

	plan, err := app.PlanAmend(g, 0, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commits) != 15 || plan.Ref != "refs/heads/feature" {
		t.Fatalf("plan: %d commits on %s", len(plan.Commits), plan.Ref)
	}
	if _, err := app.ApplyAmend(g, plan, fix); err != nil {
		t.Fatal(err)
	}
	authors := s.Git(dir, "log", "--format=%an <%ae>|%cn <%ce>", "main..feature")
	for line := range strings.SplitSeq(authors, "\n") {
		if line != "Brian Nguyen <nguyenbriankhoi@gmail.com>|Brian Nguyen <nguyenbriankhoi@gmail.com>" {
			t.Fatalf("not fixed: %q", line)
		}
	}
	if n := len(strings.Split(authors, "\n")); n != 15 {
		t.Fatalf("%d commits on branch after amend", n)
	}
	// Only the merge base changes nothing: trees, dates, messages identical.
	s.Git(dir, "branch", "-q", "base", mainBefore)
	after := snapshot(s, dir, "base..feature")
	if strings.Join(after, "") != strings.Join(before, "") {
		t.Fatalf("content changed:\nbefore %q\nafter  %q", before[0], after[0])
	}
	if s.Git(dir, "rev-parse", "main") != mainMoved {
		t.Fatal("main was touched")
	}
	if s.Git(dir, "merge-base", "main", "feature") != mainBefore {
		t.Fatal("branch was rebased onto the new main")
	}
	if s.Git(dir, "rev-parse", app.BackupRef) != oldHead {
		t.Fatal("backup ref missing")
	}
	if s.Git(dir, "status", "--porcelain") != "" {
		t.Fatal("worktree or index changed")
	}
}

func TestAmendLastNIncludingRootAndMerges(t *testing.T) {
	s := testenv.New(t)
	dir := s.Repo("r", true)
	g := git.Client{Dir: dir}
	s.Git(dir, "checkout", "-q", "-b", "side")
	s.Commit(dir, "side", school)
	s.Git(dir, "checkout", "-q", "main")
	os.WriteFile(filepath.Join(dir, "other.txt"), []byte("o"), 0o644)
	s.Git(dir, "add", "other.txt")
	s.Git(dir, "-c", "user.name=X", "-c", "user.email=x@x", "commit", "-q", "--no-verify", "-m", "main work")
	s.Git(dir, "-c", "user.name=X", "-c", "user.email=x@x", "merge", "-q", "--no-ff", "--no-verify", "-m", "merge side", "side")
	treeBefore := s.Git(dir, "rev-parse", "HEAD^{tree}")
	count := s.Git(dir, "rev-list", "--count", "HEAD")

	plan, err := app.PlanAmend(g, 4, "") // everything, root included
	if err != nil {
		t.Fatal(err)
	}
	if count != "4" || len(plan.Commits) != 4 {
		t.Fatalf("count %s plan %d", count, len(plan.Commits))
	}
	if _, err := app.ApplyAmend(g, plan, fix); err != nil {
		t.Fatal(err)
	}
	if got := s.Git(dir, "log", "--format=%ae|%ce", "HEAD"); strings.Count(got, "nguyenbriankhoi@gmail.com") != 8 {
		t.Fatalf("not every commit (incl. root and merge) was rewritten:\n%s", got)
	}
	if s.Git(dir, "rev-parse", "HEAD^{tree}") != treeBefore {
		t.Fatal("tree changed")
	}
	if p := s.Git(dir, "log", "-1", "--format=%P"); len(strings.Fields(p)) != 2 {
		t.Fatalf("merge lost a parent: %q", p)
	}
	if s.Git(dir, "rev-list", "--count", "HEAD") != "4" {
		t.Fatal("commit count changed")
	}
}

func TestAmendDetachedHead(t *testing.T) {
	s, dir, g := repoWithBranch(t, 2)
	s.Git(dir, "checkout", "-q", "--detach")
	branch := s.Git(dir, "rev-parse", "feature")
	plan, err := app.PlanAmend(g, 1, "")
	if err != nil || plan.Ref != "HEAD" {
		t.Fatalf("%v %s", err, plan.Ref)
	}
	if _, err := app.ApplyAmend(g, plan, fix); err != nil {
		t.Fatal(err)
	}
	if s.Git(dir, "rev-parse", "feature") != branch {
		t.Fatal("detached amend moved the branch")
	}
	if s.Author(dir) != "Brian Nguyen <nguyenbriankhoi@gmail.com>" {
		t.Fatal(s.Author(dir))
	}
}

func TestAmendRefusesIfBranchMovedMeanwhile(t *testing.T) {
	s, dir, g := repoWithBranch(t, 2)
	plan, err := app.PlanAmend(g, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	moved := s.Commit(dir, "sneaky", school) // someone commits between plan and apply
	if _, err := app.ApplyAmend(g, plan, fix); err == nil {
		t.Fatal("amend overwrote a concurrent commit")
	}
	if s.Git(dir, "rev-parse", "HEAD") != moved {
		t.Fatal("HEAD changed despite the refusal")
	}
}

func TestAmendMessageEditOnlyTouchesMessage(t *testing.T) {
	s, dir, g := repoWithBranch(t, 1)
	before := snapshot(s, dir, "-1")[0]
	plan, _ := app.PlanAmend(g, 1, "")
	rw := fix
	rw.Message = func(m string) (string, error) { return m + "\nCo-authored-by: A <a@x>\n", nil }
	if _, err := app.ApplyAmend(g, plan, rw); err != nil {
		t.Fatal(err)
	}
	after := snapshot(s, dir, "-1")[0]
	if !strings.HasPrefix(after, strings.TrimSuffix(before, "\n")) || !strings.Contains(after, "Co-authored-by: A <a@x>") {
		t.Fatalf("before %q\nafter %q", before, after)
	}
}
