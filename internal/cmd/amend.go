package cmd

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gids/internal/app"
	"gids/internal/config"
	"gids/internal/git"
)

func newAmendCmd(c *cli) *cobra.Command {
	var (
		n               int
		since, author   string
		addCo, removeCo []string
		pickCo, yes     bool
	)
	cmd := &cobra.Command{
		Use:   "amend",
		Short: "Re-author commits as the right profile (and fix co-authors)",
		Long: `Rewrites the author and committer of commits to a profile, keeping everything
else (code, messages, dates, merges) identical. Defaults: the last commit, as
the profile mapped to this repo. Nothing is pushed; a backup ref is kept.`,
		Example: `  gids amend                      # last commit, as this repo's profile
  gids amend --since main         # every commit on this branch
  gids amend -n 3 --author Personal
  gids amend --add-coauthor alice
  gids amend -c                   # pick co-authors`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			g, _, err := c.repo()
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("last") && n < 1 {
				return errors.New("-n must be a positive integer")
			}
			coFlags := len(addCo) > 0 || len(removeCo) > 0 || pickCo
			if coFlags && (n > 1 || since != "") {
				return errors.New("co-author changes work on one commit at a time (no -n/--since)")
			}
			plan, err := app.PlanAmend(g, n, since)
			if err != nil {
				return err
			}
			cfg, err := c.load()
			if err != nil {
				return err
			}
			who, err := amendAuthor(g, cfg, author)
			if err != nil {
				return err
			}
			rw := app.Rewrite{Name: who.GitName, Email: who.GitEmail}
			if coFlags {
				if rw.Message, err = c.coauthorEdit(cfg, plan.Commits[0].Message, addCo, removeCo, pickCo); err != nil {
					return err
				}
			}
			if !coFlags && allAuthoredBy(plan, who) {
				c.ui.Success("Nothing to do: %s already authored as %s.", plural(len(plan.Commits), "the commit is", "all %d commits are"), who.Ident())
				return nil
			}
			for _, cm := range plan.Commits {
				if len(cm.Dropped) > 0 {
					c.ui.Warn("%s has %s; rewriting removes it.", cm.SHA[:7], strings.Join(cm.Dropped, ", "))
				}
			}
			c.showPlan(plan, who)
			if !yes {
				ok, err := c.ui.Confirm("Proceed?", false)
				if err != nil {
					return err
				}
				if !ok {
					c.ui.Printf("Nothing changed.\n")
					return nil
				}
			}
			newHead, err := app.ApplyAmend(g, plan, rw)
			if err != nil {
				return err
			}
			c.ui.Success("Rewrote %d commit(s). HEAD is now %s.", len(plan.Commits), newHead[:7])
			c.ui.Hint("To undo: git reset --keep %s   (also saved as %s)", plan.OldHead[:7], app.BackupRef)
			if plan.Pushed > 0 {
				c.ui.Hint("These were already pushed: update the remote with git push --force-with-lease")
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.IntVarP(&n, "last", "n", 0, "rewrite the last N commits")
	f.StringVar(&since, "since", "", "rewrite every commit after <rev> (branch, tag or sha), e.g. --since main")
	f.StringVar(&author, "author", "", "profile to author as (default: the profile mapped to this repo)")
	f.StringSliceVar(&addCo, "add-coauthor", nil, "add a co-author (profile or \"Name <email>\")")
	f.StringSliceVar(&removeCo, "remove-coauthor", nil, "remove the co-author with this email")
	f.BoolVarP(&pickCo, "coauthor", "c", false, "pick co-authors interactively")
	f.BoolVarP(&yes, "yes", "y", false, "don't ask for confirmation")
	return cmd
}

// amendAuthor is --author's profile, else the one mapped to the repo.
func amendAuthor(g git.Client, cfg *config.Config, name string) (*config.Profile, error) {
	if name != "" {
		p := cfg.ProfileFold(name)
		if p == nil {
			return nil, fmt.Errorf("profile %q not found (see: gids profile list)", name)
		}
		return p, nil
	}
	mapped, _, err := g.ConfigGet("gids.profile")
	if err != nil {
		return nil, err
	}
	if p := cfg.Profile(mapped); p != nil {
		return p, nil
	}
	return nil, errors.New("no profile is mapped to this repo; pass --author <profile> (or map it: gids use <profile>)")
}

func allAuthoredBy(plan app.AmendPlan, p *config.Profile) bool {
	for _, c := range plan.Commits {
		if c.Author.Name != p.GitName || !strings.EqualFold(c.Author.Email, p.GitEmail) ||
			c.Committer.Name != p.GitName || !strings.EqualFold(c.Committer.Email, p.GitEmail) {
			return false
		}
	}
	return true
}

func (c *cli) showPlan(plan app.AmendPlan, p *config.Profile) {
	branch := strings.TrimPrefix(plan.Ref, "refs/heads/")
	if plan.Ref == "HEAD" {
		branch = "detached HEAD"
	}
	c.ui.Printf("Re-author %d commit(s) on %s as %s:\n", len(plan.Commits), branch, p.Ident())
	tw := tabwriter.NewWriter(c.ui.Out(), 0, 0, 2, ' ', 0)
	for _, cm := range slices.Backward(plan.Commits) { // newest first, like git log

		subject, _, _ := strings.Cut(cm.Message, "\n")
		if len([]rune(subject)) > 50 {
			subject = string([]rune(subject)[:49]) + "…"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", cm.SHA[:7], subject, cm.Author.Email)
	}
	tw.Flush()
	if plan.Pushed > 0 {
		c.ui.Warn("%d of these are already on a remote; you'll need to force-push.", plan.Pushed)
	}
}

var coauthorLine = regexp.MustCompile(`(?im)^co-authored-by:\s*(.+?)\s*$`)

// currentCoauthors reads Co-authored-by trailers from a message.
func currentCoauthors(msg string) []string {
	var out []string
	for _, m := range coauthorLine.FindAllStringSubmatch(msg, -1) {
		out = appendUnique(out, m[1])
	}
	return out
}

// coauthorEdit builds the message transform for co-author flags / picker.
func (c *cli) coauthorEdit(cfg *config.Config, msg string, add, remove []string, pick bool) (func(string) (string, error), error) {
	var toAdd []string
	for _, a := range add {
		r, err := resolveCoauthor(cfg, a)
		if err != nil {
			return nil, err
		}
		toAdd = appendUnique(toAdd, r)
	}
	toRemove := append([]string(nil), remove...)
	if pick {
		cur := currentCoauthors(msg)
		chosen, err := c.pickCoauthors(cfg, cur)
		if err != nil {
			return nil, err
		}
		for _, a := range chosen {
			if !containsFold(cur, a) {
				toAdd = appendUnique(toAdd, a)
			}
		}
		for _, a := range cur {
			if !containsFold(chosen, a) {
				if _, e, ok := parseCoauthor(a); ok {
					toRemove = append(toRemove, e)
				}
			}
		}
	}
	for _, e := range toRemove {
		if config.ValidateEmail(e) != nil {
			return nil, fmt.Errorf("--remove-coauthor takes an email, not %q", e)
		}
	}
	return func(m string) (string, error) { return editCoauthors(m, toAdd, toRemove) }, nil
}

// editCoauthors removes trailers whose email is in remove, then adds add.
func editCoauthors(msg string, add, remove []string) (string, error) {
	out := msg
	if len(remove) > 0 {
		lines := strings.SplitAfter(msg, "\n")
		kept := lines[:0]
		for _, l := range lines {
			if m := coauthorLine.FindStringSubmatch(strings.TrimRight(l, "\n")); m != nil {
				if _, e, ok := parseCoauthor(m[1]); ok && containsFold(remove, e) {
					continue
				}
			}
			kept = append(kept, l)
		}
		out = strings.TrimRight(strings.Join(kept, ""), "\n") + "\n"
	}
	if len(add) == 0 {
		return out, nil
	}
	args := []string{"interpret-trailers", "--if-exists", "addIfDifferent"}
	for _, a := range add {
		args = append(args, "--trailer", "Co-authored-by: "+a)
	}
	return git.Client{Env: []string{"GIDS_SKIP_HOOKS=1"}}.RunRaw(out, args...)
}
