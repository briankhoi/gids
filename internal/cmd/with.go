package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gids/internal/config"
	"gids/internal/git"
	"gids/internal/ui"
)

// parseCoauthor parses "Name <email>".
func parseCoauthor(s string) (name, email string, ok bool) {
	s = strings.TrimSpace(s)
	lt := strings.IndexByte(s, '<')
	if lt < 1 || !strings.HasSuffix(s, ">") {
		return "", "", false
	}
	name, email = strings.TrimSpace(s[:lt]), s[lt+1:len(s)-1]
	if config.ValidateGitName(name) != nil || config.ValidateEmail(email) != nil {
		return "", "", false
	}
	return name, email, true
}

func validCoauthor(s string) bool { _, _, ok := parseCoauthor(s); return ok }

// resolveCoauthor turns a profile name or "Name <email>" into "Name <email>".
func resolveCoauthor(cfg *config.Config, target string) (string, error) {
	if p := cfg.ProfileFold(target); p != nil {
		return p.Ident(), nil
	}
	if n, e, ok := parseCoauthor(target); ok {
		return n + " <" + e + ">", nil
	}
	return "", fmt.Errorf("profile %q not found. Use: gids with \"Alice Smith <alice@example.com>\", or add them with: gids profile add", target)
}

func clearSession(g git.Client) error {
	if err := g.ConfigUnsetLocal("gids.coauthor"); err != nil {
		return err
	}
	return g.ConfigUnsetLocal("gids.coauthorOnce")
}

// setSession replaces the repo's co-authors with authors.
func setSession(g git.Client, authors []string, once bool) error {
	if err := clearSession(g); err != nil {
		return err
	}
	for _, a := range authors {
		if _, err := g.Run("config", "--local", "--add", "gids.coauthor", a); err != nil {
			return err
		}
	}
	if once && len(authors) > 0 {
		_, err := g.Run("config", "--local", "gids.coauthorOnce", "true")
		return err
	}
	return nil
}

func newWithCmd(c *cli) *cobra.Command {
	var once, clear bool
	cmd := &cobra.Command{
		Use:   "with [<profile | \"Name <email>\">...]",
		Short: "Pair: add Co-authored-by trailers to commits in this repo",
		Long: `Sets this repo's co-authors. Every commit gets a Co-authored-by trailer for each
one until you run 'gids with --clear' (or, with --once, just the next commit).
With no arguments: pick co-authors (in a terminal) or show the current ones.`,
		Example: "  gids with alice\n  gids with alice \"Bob Lee <bob@example.com>\"\n  gids with --once alice\n  gids with --clear",
		RunE: func(cmd *cobra.Command, args []string) error {
			g, _, err := c.repo()
			if err != nil {
				return err
			}
			if clear {
				if len(args) > 0 || once {
					return errors.New("--clear takes no other arguments")
				}
				if err := clearSession(g); err != nil {
					return err
				}
				c.ui.Success("No co-authors for this repo now.")
				return nil
			}
			cfg, err := c.load()
			if err != nil {
				return err
			}
			current, err := g.ConfigGetAll("gids.coauthor", "--local")
			if err != nil {
				return err
			}
			var authors []string
			switch {
			case len(args) > 0:
				for _, a := range args {
					r, err := resolveCoauthor(cfg, a)
					if err != nil {
						return err
					}
					authors = appendUnique(authors, r)
				}
			case c.ui.Interactive():
				if authors, err = c.pickCoauthors(cfg, current); err != nil {
					return err
				}
			default:
				if len(current) == 0 {
					c.ui.Printf("No co-authors for this repo. Set some: gids with <profile | \"Name <email>\">...\n")
				} else {
					c.ui.Printf("Co-authors: %s\n", strings.Join(current, ", "))
				}
				return nil
			}
			if err := setSession(g, authors, once); err != nil {
				return err
			}
			switch {
			case len(authors) == 0:
				c.ui.Success("No co-authors for this repo now.")
			case once:
				c.ui.Success("Next commit only: %s", strings.Join(authors, ", "))
			default:
				c.ui.Success("Co-authors until 'gids with --clear': %s", strings.Join(authors, ", "))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&once, "once", false, "only for the next commit")
	cmd.Flags().BoolVar(&clear, "clear", false, "remove this repo's co-authors")
	return cmd
}

func appendUnique(list []string, s string) []string {
	if containsFold(list, s) {
		return list
	}
	return append(list, s)
}

// pickCoauthors shows a checklist of profiles plus anyone already listed,
// with a last row to add someone new.
func (c *cli) pickCoauthors(cfg *config.Config, current []string) ([]string, error) {
	var idents []string
	for _, p := range cfg.Profiles {
		idents = appendUnique(idents, p.Ident())
	}
	for _, a := range current {
		idents = appendUnique(idents, a)
	}
	items := make([]ui.Item, 0, len(idents)+1)
	checked := make([]bool, 0, len(idents)+1)
	for _, id := range idents {
		label := id
		for _, p := range cfg.Profiles {
			if p.Ident() == id {
				label = p.Name
			}
		}
		detail := ""
		if label != id {
			detail = id
		}
		items = append(items, ui.Item{Label: label, Detail: detail})
		checked = append(checked, containsFold(current, id))
	}
	items = append(items, ui.Item{Label: "+ Someone not listed…"})
	checked = append(checked, false)
	sel, err := c.ui.MultiSelect("Co-authors for this repo", items, checked)
	if err != nil {
		return nil, err
	}
	var out []string
	for i, id := range idents {
		if sel[i] {
			out = append(out, id)
		}
	}
	if sel[len(idents)] {
		s, err := c.ui.Input(`Co-author as "Name <email>"`, "", func(s string) error {
			if !validCoauthor(s) {
				return errors.New(`use the form: Alice Smith <alice@example.com>`)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		n, e, _ := parseCoauthor(s)
		out = appendUnique(out, n+" <"+e+">")
	}
	return out, nil
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
