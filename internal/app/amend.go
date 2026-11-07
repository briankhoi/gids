package app

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gids/internal/git"
)

// BackupRef holds the pre-amend HEAD, for undo.
const BackupRef = "refs/gids/amend-backup"

// AmendPlan is the exact set of commits an amend will rewrite.
type AmendPlan struct {
	Ref     string       // what gets updated: "refs/heads/<branch>", or "HEAD" if detached
	OldHead string       // sha HEAD had when planned (the compare-and-swap value)
	Commits []git.Commit // oldest first; parents always precede children
	Pushed  int          // how many are already on a remote-tracking branch
}

// PlanAmend validates the request and lists the commits to rewrite: HEAD only
// (n=0, since=""), the last n commits, or every commit in since..HEAD.
// Every error leaves the repository untouched.
func PlanAmend(g git.Client, n int, since string) (AmendPlan, error) {
	if n != 0 && since != "" {
		return AmendPlan{}, errors.New("use either -n or --since, not both")
	}
	if n < 0 {
		return AmendPlan{}, errors.New("-n must be a positive integer")
	}
	if _, err := g.Repo(); err != nil {
		return AmendPlan{}, err
	}
	head, err := g.Run("rev-parse", "--verify", "-q", "HEAD^{commit}")
	if err != nil {
		return AmendPlan{}, errors.New("this repository has no commits yet")
	}
	if err := checkNoOperation(g); err != nil {
		return AmendPlan{}, err
	}
	ref, err := g.Run("symbolic-ref", "-q", "HEAD")
	if err != nil {
		ref = "HEAD" // detached
	}

	var revs []string
	switch {
	case since != "":
		if strings.HasPrefix(since, "-") {
			return AmendPlan{}, fmt.Errorf("unknown revision %q", since)
		}
		base, err := g.Run("rev-parse", "--verify", "-q", "--end-of-options", since+"^{commit}")
		if err != nil {
			if ExitAmbiguous(err) {
				return AmendPlan{}, fmt.Errorf("%q is ambiguous; use a longer sha", since)
			}
			return AmendPlan{}, fmt.Errorf("unknown revision %q", since)
		}
		if _, err := g.Run("merge-base", base, head); err != nil {
			return AmendPlan{}, fmt.Errorf("%s shares no history with HEAD", since)
		}
		out, err := g.Run("rev-list", "--topo-order", "--reverse", base+".."+head)
		if err != nil {
			return AmendPlan{}, err
		}
		if out == "" {
			return AmendPlan{}, fmt.Errorf("no commits after %s on this branch; nothing to amend", since)
		}
		revs = strings.Split(out, "\n")
	default:
		if n == 0 {
			n = 1
		}
		total, err := g.Run("rev-list", "--count", head)
		if err != nil {
			return AmendPlan{}, err
		}
		if t, _ := strconv.Atoi(total); n > t {
			return AmendPlan{}, fmt.Errorf("only %d commit(s) exist; use -n %d", t, t)
		}
		// The first n in topological order (children before parents) are
		// closed under "child of a rewritten commit", so no commit outside
		// the set needs new parents.
		out, err := g.Run("rev-list", "--topo-order", "--max-count="+strconv.Itoa(n), head)
		if err != nil {
			return AmendPlan{}, err
		}
		revs = reverse(strings.Split(out, "\n"))
	}

	plan := AmendPlan{Ref: ref, OldHead: head}
	for _, r := range revs {
		c, err := g.ReadCommit(r)
		if err != nil {
			return AmendPlan{}, err
		}
		plan.Commits = append(plan.Commits, c)
	}
	unpushed, err := g.Run(append([]string{"rev-list", "--count"}, append(revs, "--not", "--remotes")...)...)
	if err == nil {
		u, _ := strconv.Atoi(unpushed)
		plan.Pushed = len(revs) - u
	}
	return plan, nil
}

// ExitAmbiguous reports whether rev-parse failed because a short sha is ambiguous.
func ExitAmbiguous(err error) bool {
	var ge *git.Error
	return errors.As(err, &ge) && strings.Contains(ge.Stderr, "ambiguous")
}

func reverse(s []string) []string {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
	return s
}

// checkNoOperation refuses while a rebase, merge, cherry-pick, revert or
// bisect is in progress: HEAD is in flux then.
func checkNoOperation(g git.Client) error {
	for _, p := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "BISECT_LOG"} {
		path, err := g.Run("rev-parse", "--path-format=absolute", "--git-path", p)
		if err != nil {
			return err
		}
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("a %s is in progress; finish or abort it first", strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(p, "_HEAD"), "_LOG")))
		}
	}
	return nil
}

// Rewrite says what changes in each commit.
type Rewrite struct {
	Name, Email string                           // new author and committer identity
	Message     func(msg string) (string, error) // nil keeps messages byte-identical
}

// ApplyAmend rewrites the planned commits and moves the ref, atomically.
//
// New commits are built with commit-tree: same tree, same parents (mapped
// to their rewritten versions), same author and committer dates, same
// message unless rw.Message changes it. Nothing is visible until the single
// compare-and-swap ref update at the end, so an interruption at any earlier
// point leaves the repository exactly as it was (SPEC §11 I6). Before the
// update, every new commit is checked against its original.
func ApplyAmend(g git.Client, plan AmendPlan, rw Rewrite) (newHead string, err error) {
	g.Env = append(g.Env, "GIDS_SKIP_HOOKS=1")
	mapped := map[string]string{}
	for _, c := range plan.Commits {
		parents := make([]string, len(c.Parents))
		for i, p := range c.Parents {
			parents[i] = p
			if np, ok := mapped[p]; ok {
				parents[i] = np
			}
		}
		msg := c.Message
		if rw.Message != nil {
			if msg, err = rw.Message(msg); err != nil {
				return "", err
			}
		}
		author := git.Ident{Name: rw.Name, Email: rw.Email, Date: c.Author.Date}
		committer := git.Ident{Name: rw.Name, Email: rw.Email, Date: c.Committer.Date}
		sha, err := g.CommitTree(c.Tree, parents, author, committer, c.Encoding, msg)
		if err != nil {
			return "", err
		}
		if err := verifyRewrite(g, c, sha, parents, author, committer, msg); err != nil {
			return "", fmt.Errorf("refusing to update %s: %w", plan.Ref, err)
		}
		mapped[c.SHA] = sha
	}
	newHead, ok := mapped[plan.OldHead]
	if !ok {
		return "", errors.New("internal error: HEAD was not rewritten")
	}
	if _, err := g.Run("update-ref", "-m", "gids amend: backup", BackupRef, plan.OldHead); err != nil {
		return "", err
	}
	args := []string{"update-ref", "-m", "gids amend"}
	if plan.Ref == "HEAD" {
		args = append(args, "--no-deref")
	}
	if _, err := g.Run(append(args, plan.Ref, newHead, plan.OldHead)...); err != nil {
		return "", fmt.Errorf("%s changed while amending; nothing was rewritten: %w", plan.Ref, err)
	}
	return newHead, nil
}

// verifyRewrite re-reads the new commit and checks that only what was asked
// changed.
func verifyRewrite(g git.Client, old git.Commit, sha string, parents []string, author, committer git.Ident, msg string) error {
	n, err := g.ReadCommit(sha)
	if err != nil {
		return err
	}
	switch {
	case n.Tree != old.Tree:
		return fmt.Errorf("tree of %s changed", short(old.SHA))
	case strings.Join(n.Parents, " ") != strings.Join(parents, " "):
		return fmt.Errorf("parents of %s changed", short(old.SHA))
	case n.Author != author || n.Committer != committer:
		return fmt.Errorf("identity or dates of %s not as expected (%v / %v)", short(old.SHA), n.Author, n.Committer)
	case n.Message != msg:
		return fmt.Errorf("message of %s changed", short(old.SHA))
	case normEncoding(n.Encoding) != normEncoding(old.Encoding):
		return fmt.Errorf("encoding of %s changed", short(old.SHA))
	}
	return nil
}

// normEncoding treats an explicit UTF-8 header like none (git omits it).
func normEncoding(e string) string {
	if strings.EqualFold(e, "utf-8") || strings.EqualFold(e, "utf8") {
		return ""
	}
	return e
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
